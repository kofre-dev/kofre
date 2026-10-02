package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/vault"
	"testing"
)

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

	// 9. Confirma que os dados permaneceram intactos
	if m.vault.Count() != 1 {
		t.Fatalf("esperava 1 entrada no cofre, obteve %d", m.vault.Count())
	}
	items := m.vault.Search("Banco Secreto", "")
	if len(items) == 0 || items[0].Title != "Banco Secreto" {
		t.Fatal("dados corrompidos ou perdidos apos recriptografia")
	}
}
