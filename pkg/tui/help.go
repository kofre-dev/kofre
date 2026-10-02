package tui

import (
	"github.com/charmbracelet/lipgloss"
	"strings"
)

func (m Model) helpWidth() int {
	if m.width <= 0 {
		return 80
	}
	return m.width
}

// Mantém cada dica inteira quando cabe; em larguras menores quebra entre palavras.
func wrapHelp(items []string, width int) string {
	var lines []string
	line := ""
	for _, item := range items {
		candidate := item
		if line != "" {
			candidate = line + " · " + item
		}
		if lipgloss.Width(candidate) <= width {
			line = candidate
			continue
		}
		if line != "" {
			lines = append(lines, line)
			line = ""
		}
		for _, word := range strings.Fields(item) {
			candidate = word
			if line != "" {
				candidate = line + " " + word
			}
			if line != "" && lipgloss.Width(candidate) > width {
				lines = append(lines, line)
				line = word
			} else {
				line = candidate
			}
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m Model) listFooter() string {
	return helpStyle.Render(wrapHelp([]string{
		"↑↓ Navegar", "Enter Abrir", "c Copiar", "/ Buscar",
		"n Novo", "m Mudar Senha", "Ctrl+L Bloquear", "F1 Ajuda", "q Sair",
	}, m.helpWidth()))
}

func (m Model) helpLines() []string {
	items := []string{
		"↑/↓ ou j/k: navegar na lista",
		"Scroll: navegar com o mouse",
		"PgUp/PgDn: avançar ou voltar uma página",
		"Home/g: primeira credencial",
		"End/G: última credencial",
		"Enter: abrir detalhes",
		"c: copiar o segredo principal",
		"/: buscar; Esc: sair da busca",
		"n: nova credencial",
		"d: excluir credencial",
		"m: alterar senha mestre do cofre (recriptografia)",
		"Tab/Shift+Tab: mudar categoria",
		"p: plano e status Pro",
		"Ctrl+L: bloquear o cofre",
		"q ou Ctrl+C: sair do Kofre",
	}
	var lines []string
	for _, item := range items {
		lines = append(lines, strings.Split(wrapHelp([]string{item}, m.helpWidth()), "\n")...)
	}
	return lines
}

func (m Model) helpNavigation() string {
	return wrapHelp([]string{"↑↓ Rolar", "F1/Esc Voltar"}, m.helpWidth())
}

func (m Model) helpPageHeight() int {
	if m.height <= 0 {
		return 20
	}
	return max(1, m.height-3-lipgloss.Height(m.helpNavigation()))
}

func (m Model) helpMaxOffset() int { return max(0, len(m.helpLines())-m.helpPageHeight()) }

func (m Model) viewHelp() string {
	lines := m.helpLines()
	start := min(m.helpOffset, m.helpMaxOffset())
	end := min(len(lines), start+m.helpPageHeight())
	return "Atalhos da lista\n\n" + strings.Join(lines[start:end], "\n") + "\n\n" + dimStyle.Render(m.helpNavigation())
}
