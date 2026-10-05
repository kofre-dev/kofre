package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"kofre/pkg/vault"
)

func TestListaAlinhaColunasEPreservaRolagem(t *testing.T) {
	m := fixtureModel(t)
	for i := 0; i < 80; i++ {
		_, err := m.vault.AddEntry(vault.SecretEntry{
			Title: strings.Repeat("Título extenso ", 12), Category: vault.CategoryPassword,
			Fields: []vault.Field{{Value: strings.Repeat("usuario\n", 20)}, {Value: "nao-exibir-senha", Protected: true}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	m.refreshList()
	for _, tamanho := range [][2]int{{40, 30}, {80, 30}, {120, 40}} {
		m.width, m.height = tamanho[0], tamanho[1]
		m.cursor = len(m.filteredItems) - 1
		m.adjustScroll()
		view := ansi.Strip(m.View())
		if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
			t.Fatalf("lista excedeu %dx%d: %dx%d", m.width, m.height, lipgloss.Width(view), lipgloss.Height(view))
		}
		if strings.Contains(view, "nao-exibir-senha") || !strings.Contains(view, "Item 80 de 80") {
			t.Fatal("senha exposta ou item selecionado fora da página")
		}
	}
}
