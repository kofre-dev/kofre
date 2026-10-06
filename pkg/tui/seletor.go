package tui

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"kofre/pkg/config"
	"strings"
	"time"
	"unicode"
)

type seletor struct {
	titulo                           string
	opcoes                           []string
	cursor, escolha, largura, altura int
	tema                             paletaTema
	atividade                        time.Time
}
type seletorTick time.Time

func tickSeletor() tea.Cmd {
	return tea.Tick(time.Minute, func(t time.Time) tea.Msg { return seletorTick(t) })
}
func (s seletor) Init() tea.Cmd { return tickSeletor() }
func (s seletor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		s.largura, s.altura = m.Width, m.Height
	case seletorTick:
		if time.Since(s.atividade) > 5*time.Minute {
			return s, tea.Quit
		}
		return s, tickSeletor()
	case tea.KeyMsg:
		s.atividade = time.Now()
		switch m.String() {
		case "esc", "ctrl+c":
			return s, tea.Quit
		case "up", "k":
			s.cursor = max(0, s.cursor-1)
		case "down", "j":
			s.cursor = min(len(s.opcoes)-1, s.cursor+1)
		case "enter":
			if len(s.opcoes) > 0 {
				s.escolha = s.cursor
			}
			return s, tea.Quit
		}
	}
	return s, nil
}
func textoSeguroMenu(v string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, v)
}
func (s seletor) View() string {
	w := max(18, min(76, s.largura-8))
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(s.tema.Destaque).Render(ansi.Truncate(textoSeguroMenu(s.titulo), w, "…")) + "\n\n")
	visiveis := max(1, s.altura-11)
	inicio := max(0, s.cursor-visiveis+1)
	for i := inicio; i < min(len(s.opcoes), inicio+visiveis); i++ {
		line := "  " + textoSeguroMenu(s.opcoes[i])
		style := lipgloss.NewStyle().Width(w).Foreground(s.tema.Texto)
		if i == s.cursor {
			line = "› " + textoSeguroMenu(s.opcoes[i])
			style = style.Background(s.tema.Selecao).Bold(true)
		}
		b.WriteString(style.Render(ansi.Truncate(line, w, "…")) + "\n")
	}
	b.WriteString("\n" + lipgloss.NewStyle().Foreground(s.tema.Suave).Render("↑↓ Escolher  ·  Enter Abrir  ·  Esc Voltar"))
	return "\n" + lipgloss.NewStyle().Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(s.tema.Destaque).Render(b.String()) + "\n"
}

// Menus auxiliares usam a mesma paleta do cofre. O chamador mantém dados e
// credenciais fora dos rótulos; o menu fecha após inatividade.
func EscolherOpcao(titulo string, opcoes []string) (int, error) {
	tema := temas[0]
	if cfg, err := config.LoadConfig(); err == nil {
		tema = temas[indiceTema(cfg.Tema)]
	}
	m, err := tea.NewProgram(seletor{titulo: titulo, opcoes: opcoes, escolha: -1, largura: 80, altura: 24, tema: tema, atividade: time.Now()}, tea.WithAltScreen()).Run()
	if err != nil {
		return -1, err
	}
	s, ok := m.(seletor)
	if !ok {
		return -1, errors.New("menu encerrado")
	}
	return s.escolha, nil
}
