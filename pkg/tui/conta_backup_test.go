package tui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"kofre/pkg/corporativo"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/vault"
)

func TestSenhaDaContaPreservaFalhasEReenviaBackupCifrado(t *testing.T) {
	for _, caso := range []string{"preflight", "disco", "envio", "conflito", "ok"} {
		t.Run(caso, func(t *testing.T) {
			m := fixtureModel(t)
			defer m.Close()
			i, err := corporativo.PrepararCadastro("Pessoa fictícia")
			if err != nil {
				t.Fatal(err)
			}
			defer i.Fechar()
			i.ID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			i.Email = "pessoa@example.test"
			data, err := i.Serializar()
			if err != nil {
				t.Fatal(err)
			}
			if err = m.vault.DefinirConta(data); err != nil {
				t.Fatal(err)
			}
			mycrypto.ZeroBytes(data)
			before, err := m.pack()
			if err != nil {
				t.Fatal(err)
			}
			store := m.storage.(*memoryStore)
			store.data = bytes.Clone(before)
			store.fail = caso == "disco"
			var attempts atomic.Int32
			remote := make(chan []byte, 2)
			var failSend atomic.Bool
			failSend.Store(caso == "envio" || caso == "conflito")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					if caso == "preflight" {
						w.WriteHeader(503)
						return
					}
					json.NewEncoder(w).Encode(map[string]string{"id": i.ID, "backup_revisao": "revisao-inicial"})
					return
				}
				attempts.Add(1)
				var in struct {
					Cifra   []byte `json:"identidade_cifrada"`
					Revisao string `json:"revisao"`
				}
				if json.NewDecoder(r.Body).Decode(&in) != nil || in.Revisao != "revisao-inicial" {
					t.Error("revisão original perdida")
					w.WriteHeader(400)
					return
				}
				if failSend.Load() {
					if caso == "conflito" {
						w.WriteHeader(409)
					} else {
						w.WriteHeader(503)
					}
					return
				}
				remote <- bytes.Clone(in.Cifra)
				w.WriteHeader(200)
			}))
			defer server.Close()
			t.Setenv("KOFRE_CLOUD_ENDPOINT", server.URL)
			err = m.changePassword([]byte("nova-senha-ficticia"))
			if caso == "preflight" || caso == "disco" {
				if err == nil || !bytes.Equal(before, store.data) || len(m.vault.BackupContaPendente()) != 0 || attempts.Load() != 0 {
					t.Fatal("falha inicial alterou persistência ou publicou backup")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if caso == "envio" || caso == "conflito" {
				pending := m.vault.BackupContaPendente()
				if len(pending) == 0 {
					t.Fatal("falha remota perdeu fila")
				}
				salt, payload, err := vault.UnpackHeader(store.data)
				if err != nil {
					t.Fatal(err)
				}
				key, _, err := mycrypto.DeriveKeyBytes([]byte("nova-senha-ficticia"), salt)
				if err != nil {
					t.Fatal(err)
				}
				reopened, err := vault.DecryptAndLoad(payload, key, salt)
				mycrypto.ZeroBytes(key)
				if err != nil {
					t.Fatal("nova senha não persistida", err)
				}
				if !bytes.Equal(pending, reopened.BackupContaPendente()) {
					t.Fatal("fila não está no cofre cifrado")
				}
				reopened.Close()
				if err = m.changePassword([]byte("outra-senha-ficticia")); err == nil {
					t.Fatal("segunda troca atropelou fila")
				}
				failSend.Store(false)
				if err = m.enviarBackupContaPendente(); err != nil {
					t.Fatal(err)
				}
			}
			if len(m.vault.BackupContaPendente()) != 0 {
				t.Fatal("fila concluída não foi removida")
			}
			cifra := <-remote
			restored, err := corporativo.AbrirIdentidadeBytes(cifra, []byte("nova-senha-ficticia"))
			if err != nil {
				t.Fatal("backup remoto não usa nova senha", err)
			}
			restored.Fechar()
			if wrong, err := corporativo.AbrirIdentidadeBytes(cifra, []byte("antiga-ficticia")); err == nil {
				wrong.Fechar()
				t.Fatal("backup aceitou senha antiga")
			}
		})
	}
}
