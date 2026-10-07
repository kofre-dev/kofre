package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"kofre/pkg/corporativo"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/storage"
	"kofre/pkg/vault"
)

func TestNuvemContaPersisteBaseParaPrimeiraEdicaoAposReinicio(t *testing.T) {
	ctx := context.Background()
	v := vault.NewManaged()
	defer v.Close()
	key, salt, err := mycrypto.DeriveKeyBytes([]byte("senha-fixture-123"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(key)
	base, err := v.Pack(key, salt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.AddEntry(vault.SecretEntry{Title: "Credencial criada após login", Fields: []vault.Field{{Name: "Senha", Value: "somente-fixture", Protected: true}}}); err != nil {
		t.Fatal(err)
	}
	edicao, err := v.Pack(key, salt)
	if err != nil {
		t.Fatal(err)
	}
	for _, existiaRemoto := range []bool{false, true} {
		t.Run(fmt.Sprint("remoto_anterior_", existiaRemoto), func(t *testing.T) {
			var mu sync.Mutex
			var remoto []byte
			if existiaRemoto {
				remoto = bytes.Clone(base)
			}
			revisao := func(data []byte) string { return fmt.Sprintf(`"%x"`, sha256.Sum256(data)) }
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if r.Method == http.MethodGet {
					if remoto == nil {
						http.NotFound(w, r)
						return
					}
					w.Header().Set("ETag", revisao(remoto))
					_, _ = w.Write(remoto)
					return
				}
				if r.Method != http.MethodPut {
					http.Error(w, "inesperado", 500)
					return
				}
				if remoto == nil && r.Header.Get("If-None-Match") != "*" || remoto != nil && r.Header.Get("If-Match") != revisao(remoto) {
					http.Error(w, "conflito", 409)
					return
				}
				remoto, _ = io.ReadAll(r.Body)
				w.Header().Set("ETag", revisao(remoto))
			}))
			defer srv.Close()
			t.Setenv("KOFRE_CLOUD_ENDPOINT", srv.URL)
			path := filepath.Join(t.TempDir(), "vault.enc")
			local, err := storage.NewLocalStorage(path)
			if err != nil {
				t.Fatal(err)
			}
			i := &corporativo.Identidade{ID: "conta-ficticia", Token: "token-ficticio"}
			cloud, err := nuvemContaComRevisao(path, i)
			if err != nil {
				t.Fatal(err)
			}
			carregado, err := cloud.Load(ctx)
			if err != nil && err != storage.ErrNotFound {
				t.Fatal(err)
			}
			if err := local.Save(ctx, base); err != nil {
				t.Fatal(err)
			}
			if carregado != nil {
				if err := cloud.ConfirmarLeitura(carregado); err != nil {
					t.Fatal(err)
				}
			}
			if err := cloud.Save(ctx, base); err != nil {
				t.Fatal(err)
			}
			// Simula novo processo: objetos de sincronização são novos e só podem
			// recuperar a base pelos arquivos persistidos do primeiro login.
			syncer := storage.NewSyncStorage(local, storage.NewKofreCloudStorage(srv.URL, i.Token, i.ID))
			defer syncer.Close()
			candidate, err := syncer.Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			candidateSalt, payload, err := vault.UnpackHeader(candidate)
			if err != nil {
				t.Fatal(err)
			}
			opened, err := vault.DecryptAndLoad(payload, key, candidateSalt)
			if err != nil {
				t.Fatal(err)
			}
			opened.Close()
			if err := storage.ConfirmarCarga(ctx, syncer, candidate); err != nil {
				t.Fatal(err)
			}
			if err := syncer.Save(ctx, edicao); err != nil {
				t.Fatal(err)
			}
			if err := syncer.Flush(5 * time.Second); err != nil {
				t.Fatal("primeira edição encontrou conflito falso:", err)
			}
			mu.Lock()
			igual := bytes.Equal(remoto, edicao)
			mu.Unlock()
			if !igual {
				t.Fatal("primeira edição não chegou à nuvem")
			}
		})
	}
}
