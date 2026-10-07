package tui

import (
	"bytes"
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/vault"
)

func TestCriacaoExigeSeisCaracteresComComplemento(t *testing.T) {
	for _, caso := range []struct {
		nome, senha string
		aceita      bool
	}{
		{"numeros curtos", "12345", false},
		{"letras curtas", "abcde", false},
		{"bytes nao sao caracteres", "ééééé", false},
		{"espacos externos nao contam", " 12345 ", false},
		{"somente numeros", "123456", false},
		{"numeros unicode", "１２３４５６", false},
		{"simbolo interno", "12@456", true},
		{"simbolo inicial", "@12345", true},
		{"letra final", "12345a", true},
		{"espaco interno", "12 456", true},
		{"espaco externo nao complementa", " 123456 ", false},
		{"controle nao complementa", "12\t456", false},
		{"invisivel nao complementa", "12\u200b456", false},
		{"somente letras", "abcdef", true},
		{"seis letras com acentos", "éééééé", true},
	} {
		t.Run(caso.nome, func(t *testing.T) {
			store := &memoryStore{}
			m := NewModel(store)
			defer func() { m.Close() }()
			m.passInput.SetValue(caso.senha)
			runes := m.passInput.secret.runes
			next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(Model)
			for _, r := range runes {
				if r != 0 {
					t.Fatal("entrada da senha não foi apagada")
				}
			}
			if !caso.aceita {
				if m.err == nil || m.state != ViewUnlock || len(store.data) != 0 || m.sessionKey != nil || m.vault != nil {
					t.Fatalf("senha curta criou ou abriu cofre: estado=%d erro=%v", m.state, m.err)
				}
				return
			}
			if m.err != nil || m.state != ViewList || len(store.data) == 0 {
				t.Fatalf("senha válida recusada: estado=%d erro=%v", m.state, m.err)
			}
			m.lock()
			m.passInput.SetValue(caso.senha)
			next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(Model)
			if m.err != nil || m.state != ViewList {
				t.Fatalf("senha aceita não reabre o arquivo: %v", m.err)
			}
		})
	}
}

func TestTrocaSenhaCurtaNaoAlteraArquivoNemChave(t *testing.T) {
	m := fixtureModel(t)
	defer func() { m.Close() }()
	packed, err := m.pack()
	if err != nil {
		t.Fatal(err)
	}
	store := m.storage.(*memoryStore)
	if err := store.Save(context.Background(), packed); err != nil {
		t.Fatal(err)
	}
	chaveAnterior := m.sessionKey
	for _, senha := range []string{"", "12345", "ééééé", " 12345 ", "123456", "1234567890"} {
		if err := m.changePassword([]byte(senha)); err == nil {
			t.Fatal("troca direta aceitou menos de seis caracteres")
		}
		if !bytes.Equal(store.data, packed) || m.sessionKey != chaveAnterior {
			t.Fatal("senha recusada alterou arquivo ou chave da sessão")
		}
	}
	// Exercita também a tela de troca, sem passar pela chamada direta.
	m.initChangePasswordForm()
	m.state = ViewChangePassword
	m.changePassFocus = 1
	m.newPassInput.SetValue("ééééé")
	m.confirmPassInput.SetValue("ééééé")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.err == nil || m.state != ViewChangePassword || !bytes.Equal(store.data, packed) || m.sessionKey != chaveAnterior {
		t.Fatal("tela de troca não preservou o cofre ao recusar senha curta")
	}
}

func TestTrocaSenhaSegueNormalizacaoDaAbertura(t *testing.T) {
	m := fixtureModel(t)
	defer func() { m.Close() }()
	m.initChangePasswordForm()
	m.state = ViewChangePassword
	m.changePassFocus = 1
	m.newPassInput.SetValue(" 65@321 ")
	m.confirmPassInput.SetValue("65@321")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.err != nil || m.state != ViewList {
		t.Fatalf("troca com seis caracteres recusada: %v", m.err)
	}
	m.lock()
	m.passInput.SetValue("65@321")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.err != nil || m.state != ViewList {
		t.Fatalf("nova senha não reabre o arquivo: %v", m.err)
	}
}

func TestMinimoNovoNaoBloqueiaSenhaDeCofreExistente(t *testing.T) {
	key, salt, err := mycrypto.DeriveKeyBytes([]byte("123"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(key)
	v := vault.NewManaged()
	defer v.Close()
	packed, err := v.Pack(key, salt)
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(&memoryStore{data: packed})
	defer func() { m.Close() }()
	m.passInput.SetValue("123")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.err != nil || m.state != ViewList {
		t.Fatalf("regra de criação bloqueou abertura de arquivo existente: %v", m.err)
	}
}
