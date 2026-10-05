package tui

import (
	"context"
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"testing"
)

type unavailableStore struct {
	memoryStore
	saves int
}

func (*unavailableStore) Exists(context.Context) (bool, error) {
	return false, errors.New("perfil indisponível")
}
func (*unavailableStore) Load(context.Context) ([]byte, error) {
	return nil, errors.New("perfil indisponível")
}
func (s *unavailableStore) Save(context.Context, []byte) error { s.saves++; return nil }
func TestFalhaDeExistenciaNaoCriaOuSobrescreveCofre(t *testing.T) {
	store := &unavailableStore{}
	m := NewModel(store)
	defer m.Close()
	if m.isNewVault || m.err == nil {
		t.Fatal("falha remota foi tratada como cofre novo")
	}
	m.passInput.SetValue("senha-ficticia")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if store.saves != 0 || m.state != ViewUnlock || m.err == nil {
		t.Fatal("tentativa de abrir criou cofre apesar de erro no storage")
	}
}
