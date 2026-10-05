package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestAtivarProCliqueComAvisos(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, aviso := range []string{"", "erro", "notificacao"} {
		m := Model{state: ViewUnlock, passInput: newInput(true)}
		m.passInput.SetValue("segredo-nao-publicar")
		if aviso == "erro" {
			m.err = errors.New("senha incorreta")
		} else if aviso == "notificacao" {
			m.notification = "Pronto"
		}
		view := ansi.Strip(m.View())
		if strings.Contains(view, "segredo-nao-publicar") || strings.Contains(view, "[Ctrl+T]") {
			t.Fatal("Free expôs senha ou sugeriu Telegram sem licença")
		}
		x, y := -1, -1
		for linha, texto := range strings.Split(view, "\n") {
			if pos := strings.Index(texto, botaoAtivarPro); pos >= 0 {
				x, y = lipgloss.Width(texto[:pos]), linha
			}
		}
		if x < 0 {
			t.Fatal("ação Pro não está visível")
		}
		for _, evento := range []tea.MouseMsg{
			{X: x - 1, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress},
			{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease},
			{X: x, Y: y, Button: tea.MouseButtonRight, Action: tea.MouseActionPress},
		} {
			next, _ := m.Update(evento)
			if next.(Model).state != ViewUnlock {
				t.Fatal("evento fora da ação abriu Pro")
			}
		}
		next, cmd := m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		pro := next.(Model)
		if pro.state != ViewPro || pro.previousState != ViewUnlock || pro.compraEmAndamento || cmd != nil {
			t.Fatal("clique deve abrir Pro sem iniciar compra")
		}
		if pro.passInput.Value() != m.passInput.Value() {
			t.Fatal("clique alterou a senha digitada")
		}
		m.passInput.Reset()
	}
}

func TestAcessoCabeNaJanela(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, tamanho := range [][2]int{{40, 24}, {60, 24}, {80, 24}, {80, 40}, {120, 40}} {
		m := Model{state: ViewUnlock, passInput: newInput(true), width: tamanho[0], height: tamanho[1]}
		m.passInput.SetValue(strings.Repeat("senha-privada", 20))
		view := ansi.Strip(m.View())
		if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
			t.Fatalf("layout excedeu janela %dx%d: %dx%d", m.width, m.height, lipgloss.Width(view), lipgloss.Height(view))
		}
		if strings.Contains(view, "senha-privada") {
			t.Fatal("senha exposta na renderização")
		}
		x, y := -1, -1
		for linha, texto := range strings.Split(view, "\n") {
			if pos := strings.Index(texto, botaoAtivarPro); pos >= 0 {
				x, y = lipgloss.Width(texto[:pos]), linha
			}
		}
		if x < 0 || !m.clicouAtivarPro(x, y) {
			t.Fatal("ação Pro inacessível após centralização")
		}
		m.passInput.Reset()
	}
}
