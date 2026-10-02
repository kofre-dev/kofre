package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"kofre/pkg/vault"
	"strings"
	"testing"
)

func TestResponsiveFooterAndListSpace(t *testing.T) {
	m := Model{height: 30}
	for _, width := range []int{20, 40, 80, 120} {
		m.width = width
		for _, line := range strings.Split(m.listFooter(), "\n") {
			if lipgloss.Width(line) > width {
				t.Fatalf("rodape excedeu %d colunas", width)
			}
		}
		if !strings.Contains(m.listFooter(), "F1 Ajuda") {
			t.Fatal("atalho da ajuda sumiu")
		}
	}
	m.width = 120
	wide := m.visibleListHeight()
	m.width = 20
	if m.visibleListHeight() >= wide {
		t.Fatal("rodape maior nao reservou espaco")
	}
}

func TestHelpNavigationAndResizePreserveSearch(t *testing.T) {
	m := fixtureModel(t)
	m.width = 40
	m.height = 10
	m.searchInput.SetValue("fixture")
	m.searchFocused = true
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyF1})
	m = next.(Model)
	if !m.showHelp {
		t.Fatal("F1 nao abriu ajuda durante busca")
	}
	for i := 0; i < 40; i++ {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(Model)
	}
	if m.helpOffset != m.helpMaxOffset() {
		t.Fatal("rolagem nao chegou ao fim")
	}
	for _, width := range []int{20, 40, 100} {
		next, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 10})
		m = next.(Model)
		view := m.View()
		if lipgloss.Height(view) > 10 || lipgloss.Width(view) > width {
			t.Fatal("ajuda excedeu janela")
		}
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.showHelp || m.searchInput.Value() != "fixture" || !m.searchFocused {
		t.Fatal("fechar ajuda alterou busca")
	}
}

func TestEmptySearchMessage(t *testing.T) {
	m := fixtureModel(t)
	if !strings.Contains(m.viewList(), "Cofre vazio") {
		t.Fatal("faltou orientacao de primeiro cadastro")
	}
	if _, err := m.vault.AddEntry(vault.SecretEntry{Title: "Fixture"}); err != nil {
		t.Fatal(err)
	}
	m.searchInput.SetValue("inexistente")
	m.refreshList()
	view := m.viewList()
	if !strings.Contains(view, "corresponde à busca") || strings.Contains(view, "primeira credencial") {
		t.Fatal("busca vazia confundida com cofre vazio")
	}
}
