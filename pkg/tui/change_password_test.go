package tui

import (
	"bytes"
	tea "github.com/charmbracelet/bubbletea"
	"kofre/pkg/config"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/vault"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrocaSenhaFalhaTelegramMantemNovaSenhaEInformaRecuperacao(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "falha fictícia", 503) }))
	defer endpoint.Close()
	t.Setenv("KOFRE_CLOUD_ENDPOINT", endpoint.URL)
	cfg := config.DefaultConfig()
	cfg.CloudEnabled = true
	cfg.Mode = "kofre_cloud"
	cfg.KofreToken = "kfr_licenca_ficticia"
	cfg.CloudEndpoint = endpoint.URL
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	envelopePath := filepath.Join(dir, "Kofre", "telegram_envelope.json")
	previous := []byte(`{"device_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	if err := os.WriteFile(envelopePath, previous, 0600); err != nil {
		t.Fatal(err)
	}
	m := fixtureModel(t)
	defer m.Close()
	m.initChangePasswordForm()
	m.state = ViewChangePassword
	m.changePassFocus = 1
	m.newPassInput.SetValue("nova-senha-ficticia")
	m.confirmPassInput.SetValue("nova-senha-ficticia")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.err != nil || m.state != ViewList || !strings.Contains(m.notification, "Telegram pendente") {
		t.Fatalf("troca confirmada não informou recuperação: estado=%d erro=%v aviso=%q", m.state, m.err, m.notification)
	}
	got, err := os.ReadFile(envelopePath)
	if err != nil || !bytes.Equal(got, previous) {
		t.Fatal("falha remota destruiu envelope anterior")
	}
	m.lock()
	m.passInput.SetValue("nova-senha-ficticia")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.state != ViewList || m.err != nil {
		t.Fatal("nova senha não abre o cofre após falha do Telegram")
	}
}

func TestChangeMasterPasswordFullCycle(t *testing.T) {
	// 1. Inicializa o cofre com a senha inicial
	initialPass := "senhaAntiga123"
	salt := make([]byte, mycrypto.SaltLength)
	key, _, err := mycrypto.DeriveKeyBytes([]byte(initialPass), salt)
	if err != nil {
		t.Fatal(err)
	}

	v := vault.NewManaged()
	_, _ = v.AddEntry(vault.SecretEntry{
		Title: "Banco Secreto",
		Fields: []vault.Field{
			{Name: "Senha", Value: "123456", Protected: true},
		},
	})

	packed, err := v.Pack(key, salt)
	if err != nil {
		t.Fatal(err)
	}
	mycrypto.ZeroBytes(key)
	v.Close()

	store := &memoryStore{data: packed}
	m := NewModel(store)

	// 2. Abre o cofre no TUI
	m.passInput.SetValue(initialPass)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.state != ViewList || m.vault == nil {
		t.Fatalf("falha ao abrir cofre: %v", m.err)
	}
	defer m.Close()

	if err := m.vault.DefinirConta([]byte(`{"token":"conta-ficticia-troca"}`)); err != nil {
		t.Fatal(err)
	}
	// 3. Pressiona 'm' para abrir tela de mudar senha
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = next.(Model)
	if m.state != ViewChangePassword {
		t.Fatalf("esperava ViewChangePassword, obteve %d", m.state)
	}

	// 4. Digita nova senha e confirmação com senhas divergentes
	m.newPassInput.SetValue("novaSenhaForte99")
	m.confirmPassInput.SetValue("senhaDiferente")
	m.changePassFocus = 1 // focado no confirmar
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.err == nil || m.state != ViewChangePassword {
		t.Fatal("esperava erro de divergencia de senhas")
	}

	// 5. Digita a confirmação correta
	m.confirmPassInput.SetValue("novaSenhaForte99")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.err != nil || m.state != ViewList {
		t.Fatalf("falha ao salvar nova senha: %v", m.err)
	}

	// 6. Bloqueia o cofre
	m.lock()
	if m.state != ViewUnlock {
		t.Fatalf("esperava ViewUnlock, obteve %d", m.state)
	}

	// 7. Tenta abrir com a senha antiga -> DEVE FALHAR!
	m.passInput.SetValue(initialPass)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.state == ViewList || m.err == nil {
		t.Fatal("ERRO GRAVE: cofre abriu com a senha antiga apos alteracao!")
	}

	// 8. Tenta abrir com a nova senha -> DEVE TER SUCESSO!
	m.passInput.Reset()
	m.passInput.SetValue("novaSenhaForte99")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.err != nil || m.state != ViewList {
		t.Fatalf("falha ao reabrir cofre com nova senha: %v", m.err)
	}

	if !m.vault.TemConta() {
		t.Fatal("troca de senha perdeu a identidade da conta")
	}
	// 9. Confirma que os dados permaneceram intactos
	if m.vault.Count() != 1 {
		t.Fatalf("esperava 1 entrada no cofre, obteve %d", m.vault.Count())
	}
	items := m.vault.Search("Banco Secreto", "")
	if len(items) == 0 || items[0].Title != "Banco Secreto" {
		t.Fatal("dados corrompidos ou perdidos apos recriptografia")
	}
}
