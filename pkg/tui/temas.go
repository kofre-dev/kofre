package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"kofre/pkg/config"
)

type paletaTema struct {
	ID, Nome                                                       string
	Destaque, Brilho, Botao, Selecao, Texto, Suave, Sucesso, Borda lipgloss.Color
}

var temas = []paletaTema{
	{"azul", "Azul", "#009DFF", "#36D9FF", "#006CFF", "#0053A6", "#E0E0F0", "#A5BEDF", "#36E6BA", "#24445E"},
	{"violeta", "Violeta", "#B394FF", "#DBCAFF", "#6D42C8", "#4E3188", "#EEE8FF", "#BEB1D6", "#88DBBD", "#473657"},
	{"esmeralda", "Esmeralda", "#36D7A0", "#97F0D0", "#087553", "#125940", "#E2F5ED", "#A0C8B8", "#6EE7B7", "#245344"},
	{"ambar", "Âmbar", "#F5BC55", "#FFE0A0", "#86520A", "#62420F", "#FFF0D6", "#D5BC91", "#BADD87", "#554328"},
	{"grafite", "Grafite", "#BCC7D8", "#F1F5F9", "#46556B", "#344257", "#E8EDF4", "#A2ADBC", "#BCC7D8", "#3E4857"},
}

func indiceTema(id string) int {
	for i, tema := range temas {
		if tema.ID == id {
			return i
		}
	}
	return 0
}

func (m Model) cores() paletaTema {
	if m.mostrarTemas {
		return temas[m.temaCursor]
	}
	return temas[indiceTema(m.temaID)]
}

// A paleta pertence ao modelo; pré-visualizar não modifica estilos globais,
// a licença, o cofre ou outra instância da interface.
type estilosTema struct {
	titleStyle, headerBoxStyle, statusBadgeStyle, searchStyle, selectedItemStyle,
	normalItemStyle, dimStyle, dangerStyle, successStyle, boxStyle, helpStyle lipgloss.Style
}

func (m Model) estilos() estilosTema {
	p := m.cores()
	return estilosTema{
		titleStyle.Foreground(lipgloss.Color("#FFFFFF")).Background(p.Botao),
		headerBoxStyle.BorderForeground(p.Destaque),
		statusBadgeStyle.Background(p.Botao),
		searchStyle.BorderForeground(p.Borda).Foreground(p.Texto),
		selectedItemStyle.Background(p.Selecao),
		normalItemStyle.Foreground(p.Texto),
		dimStyle.Foreground(p.Suave),
		dangerStyle,
		successStyle.Foreground(p.Sucesso),
		boxStyle.BorderForeground(p.Borda).Foreground(p.Texto),
		helpStyle.Foreground(p.Suave),
	}
}

func (m Model) atualizarTemas(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp, tea.KeyLeft:
		m.temaCursor = (m.temaCursor + len(temas) - 1) % len(temas)
	case tea.KeyDown, tea.KeyRight:
		m.temaCursor = (m.temaCursor + 1) % len(temas)
	case tea.KeyEsc, tea.KeyF2:
		m.mostrarTemas = false
	case tea.KeyEnter:
		cfg, err := config.LoadConfig()
		if err == nil {
			cfg.Tema = temas[m.temaCursor].ID
			err = config.SaveConfig(cfg)
		}
		if err != nil {
			m.err = err
			return m, nil
		}
		m.temaID = cfg.Tema
		m.mostrarTemas = false
		m.err = nil
		m.adjustScroll()
	}
	return m, nil
}

func (m Model) viewTemas() string {
	w := min(72, m.larguraAcesso()-4)
	p := m.cores()
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(p.Destaque).Render("Temas do Kofre") + "\n\n")
	for i, tema := range temas {
		rotulo := "  " + tema.Nome
		estilo := lipgloss.NewStyle().Width(w - 6).Foreground(p.Suave)
		if i == m.temaCursor {
			rotulo = "› " + tema.Nome
			estilo = estilo.Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(p.Botao)
		}
		b.WriteString(estilo.Render(rotulo) + "\n")
	}
	b.WriteString("\n" + lipgloss.NewStyle().Foreground(p.Sucesso).Render("✓ Prévia das cores selecionadas") + "\n\n")
	b.WriteString(wrapHelp([]string{"↑↓ Escolher", "Enter Salvar", "Esc Cancelar"}, w-6))
	if m.err != nil {
		b.WriteString("\n" + dangerStyle.Render(m.err.Error()))
	}
	return m.cabecalhoAcesso() + "\n" + lipgloss.NewStyle().Width(w).Padding(1, 3).Border(lipgloss.RoundedBorder()).BorderForeground(p.Destaque).Foreground(p.Texto).Render(b.String())
}
