package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"kofre/pkg/vault"
)

func TestExclusaoExigeConfirmacaoNaListaENoDetalhe(t *testing.T) {
	for _, estado := range []ViewState{ViewList, ViewDetail} {
		for _, tecla := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune{'d'}}, {Type: tea.KeyDelete}} {
			t.Run(fmt.Sprintf("%d/%s", estado, tecla.String()), func(t *testing.T) {
				m := fixtureModel(t)
				entrada, err := m.vault.AddEntry(vault.SecretEntry{Title: "Credencial de teste"})
				if err != nil {
					t.Fatal(err)
				}
				m.refreshList()
				m.selectedEntry = entrada
				m.width = 100
				m.height = 40
				m.state = estado
				if !strings.Contains(ansi.Strip(m.View()), "d/Del Excluir") {
					t.Fatal("atalho de excluir invisível")
				}
				next, _ := m.Update(tecla)
				m = next.(Model)
				if m.state != ViewConfirmDelete {
					t.Fatal("não abriu confirmação")
				}
				if _, err := m.vault.GetEntry(entrada.ID); err != nil {
					t.Fatal("excluiu antes de confirmar")
				}
				if len(m.storage.(*memoryStore).data) != 0 {
					t.Fatal("salvou antes de confirmar")
				}
				next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
				m = next.(Model)
				if _, err := m.vault.GetEntry(entrada.ID); err != nil {
					t.Fatal("cancelamento perdeu a credencial")
				}
				next, _ = m.Update(tecla)
				m = next.(Model)
				next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
				m = next.(Model)
				if m.err != nil {
					t.Fatal(m.err)
				}
				if _, err := m.vault.GetEntry(entrada.ID); !errors.Is(err, vault.ErrNotFound) {
					t.Fatal("credencial permaneceu após confirmação")
				}
				if len(m.storage.(*memoryStore).data) == 0 {
					t.Fatal("exclusão não foi salva")
				}
			})
		}
	}
}
