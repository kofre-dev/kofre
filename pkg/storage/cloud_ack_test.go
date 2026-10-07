package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestCloudRetomaACKAposFalhaNoDiscoSemAdotarOutraEdicao(t *testing.T) {
	for _, remotoMudou := range []bool{false, true} {
		t.Run(map[bool]string{false: "mesmos_bytes", true: "outra_edicao"}[remotoMudou], func(t *testing.T) {
			ctx := context.Background()
			base := []byte("cifra-ficticia-inicial")
			novo := []byte("cifra-ficticia-editada")
			remoto := bytes.Clone(base)
			var mu sync.Mutex
			puts := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if r.Method == http.MethodGet {
					w.Header().Set("ETag", revisaoArquivoCloud(remoto))
					_, _ = w.Write(remoto)
					return
				}
				puts++
				if r.Header.Get("If-Match") != revisaoArquivoCloud(remoto) {
					http.Error(w, "conflito", 409)
					return
				}
				remoto, _ = io.ReadAll(r.Body)
				w.Header().Set("ETag", revisaoArquivoCloud(remoto))
			}))
			defer srv.Close()
			path := filepath.Join(t.TempDir(), "revisao.json")
			cloud := NewKofreCloudStorage(srv.URL, "token-ficticio", "conta-ficticia")
			if err := cloud.DefinirArquivoRevisao(path); err != nil {
				t.Fatal(err)
			}
			carregado, err := cloud.Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := cloud.ConfirmarLeitura(carregado); err != nil {
				t.Fatal(err)
			}
			revisaoAnterior, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// Um diretório no destino provoca falha real na gravação atômica do
			// ACK depois de o servidor já ter persistido o PUT da fixture.
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			if err := cloud.Save(ctx, novo); err == nil {
				t.Fatal("falha no ACK foi ignorada")
			}
			mu.Lock()
			aplicado := bytes.Equal(remoto, novo)
			mu.Unlock()
			if !aplicado {
				t.Fatal("fixture não reproduziu PUT aplicado antes da falha")
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, revisaoAnterior, 0600); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			if remotoMudou {
				remoto = []byte("cifra-ficticia-de-outro-PC")
			}
			esperado := bytes.Clone(remoto)
			mu.Unlock()
			err = cloud.Save(ctx, novo)
			if remotoMudou {
				if !errors.Is(err, ErrConflito) {
					t.Fatal("adotou edição remota diferente", err)
				}
				if !cloud.coincideComBase(base) {
					t.Fatal("conflito adotou outra revisão")
				}
			} else {
				if err != nil {
					t.Fatal("ACK idempotente não recuperou", err)
				}
				reaberto := NewKofreCloudStorage(srv.URL, "outra-sessao-ficticia", "conta-ficticia")
				if err := reaberto.DefinirArquivoRevisao(path); err != nil || !reaberto.coincideComBase(novo) {
					t.Fatal("ACK não persistiu base confirmada", err)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if puts != 2 || !bytes.Equal(remoto, esperado) {
				t.Fatal("reconciliação escreveu ou perdeu dados remotos")
			}
		})
	}
}
