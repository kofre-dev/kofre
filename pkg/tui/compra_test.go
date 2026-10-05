package tui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"kofre/pkg/config"
	"kofre/pkg/storage"
)

func TestCompraNaTUIPreservaArquivoLocalEIsolaSandbox(t *testing.T) {
	for _, sandbox := range []bool{false, true} {
		name := "production"
		if sandbox {
			name = "sandbox"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("APPDATA", dir)
			t.Setenv("XDG_CONFIG_HOME", dir)
			original := []byte("arquivo cifrado ficticio")
			vaultPath := filepath.Join(dir, "vault.enc")
			if err := os.WriteFile(vaultPath, original, 0600); err != nil {
				t.Fatal(err)
			}
			cfg := config.DefaultConfig()
			cfg.VaultPath = vaultPath
			if err := config.SaveConfig(cfg); err != nil {
				t.Fatal(err)
			}
			configDir, _ := config.GetDefaultDir()
			before, _ := os.ReadFile(filepath.Join(configDir, "config.json"))
			uploads := make(chan []byte, 1)
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/v1/billing/activations":
					json.NewEncoder(w).Encode(map[string]any{"session_id": strings.Repeat("a", 32), "browser_url": "https://kofre.dev/comprar?ativacao=" + strings.Repeat("a", 32) + "#vinculo=" + strings.Repeat("b", 64), "callback_code": strings.Repeat("c", 64), "expires_at": time.Now().Add(time.Hour)})
				case strings.HasSuffix(r.URL.Path, "/complete"):
					w.Write([]byte(`{"completed":true}`))
				case strings.HasPrefix(r.URL.Path, "/v1/billing/activations/"):
					status := "active"
					token := "kfr_pro_" + strings.Repeat("d", 64)
					if sandbox {
						status = "sandbox_confirmed"
						token = ""
					}
					json.NewEncoder(w).Encode(map[string]any{"status": status, "token": token, "sandbox": sandbox})
				case r.URL.Path == "/v1/auth/verify":
					w.Write([]byte(`{}`))
				case r.URL.Path == "/v1/vault" && r.Method == "PUT":
					data, _ := io.ReadAll(r.Body)
					uploads <- data
					w.Write([]byte(`{}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer gateway.Close()
			t.Setenv("KOFRE_CLOUD_ENDPOINT", gateway.URL)
			oldOpen := abrirNavegadorCompra
			abrirNavegadorCompra = func(string) error { return nil }
			defer func() { abrirNavegadorCompra = oldOpen }()
			local, err := storage.NewLocalStorage(vaultPath)
			if err != nil {
				t.Fatal(err)
			}
			m := NewModel(local)
			defer func() { m.Close() }()
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
			m = next.(Model)
			if m.state != ViewPro {
				t.Fatal("Ctrl+P não abriu Pro")
			}
			next, start := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(Model)
			if start == nil || !m.compraEmAndamento {
				t.Fatal("Enter não iniciou compra")
			}
			next, duplicate := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(Model)
			if duplicate != nil {
				t.Fatal("Enter duplicou pedido")
			}
			next, wait := m.Update(start())
			m = next.(Model)
			if wait == nil || m.compraSessao == nil {
				t.Fatal("sessão não vinculada")
			}
			next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
			m = next.(Model)
			if !m.compraEmAndamento || m.state != ViewUnlock {
				t.Fatal("Esc cancelou compra em andamento")
			}
			next, _ = m.Update(wait())
			m = next.(Model)
			if m.err != nil || m.compraEmAndamento {
				t.Fatalf("ativação não concluiu: %v", m.err)
			}
			after, _ := os.ReadFile(filepath.Join(configDir, "config.json"))
			if sandbox {
				if !m.compraTesteConfirmada {
					t.Fatal("confirmação Sandbox não permaneceu no aplicativo")
				}
				next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
				m = next.(Model)
				if !strings.Contains(m.View(), "Pagamento de teste confirmado") {
					t.Fatal("tela Pro perdeu a confirmação do simulador")
				}
				next, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				m = next.(Model)
				if command != nil || m.compraEmAndamento {
					t.Fatal("Enter abriu nova compra após confirmação Sandbox")
				}
				if !bytes.Equal(before, after) || m.storage != local {
					t.Fatal("Sandbox alterou conta ou storage")
				}
			} else {
				cfg, err = config.LoadConfig()
				if err != nil || !cfg.CloudEnabled || cfg.KofreToken == "" || cfg.VaultPath != vaultPath {
					t.Fatal("compra não salvou licença")
				}
				syncer, ok := m.storage.(*storage.SyncStorage)
				if !ok {
					t.Fatal("sincronização não aplicada")
				}
				if err = syncer.Flush(time.Second); err != nil {
					t.Fatal(err)
				}
				select {
				case data := <-uploads:
					if !bytes.Equal(data, original) {
						t.Fatal("arquivo cifrado modificado")
					}
				case <-time.After(time.Second):
					t.Fatal("primeiro envio não executado")
				}
			}
			data, _ := os.ReadFile(vaultPath)
			if !bytes.Equal(data, original) {
				t.Fatal("cofre local sobrescrito")
			}
		})
	}
}
