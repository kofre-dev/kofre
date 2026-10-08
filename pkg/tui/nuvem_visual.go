package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) indicadorNuvem() string {
	texto := "☁̸  Offline · ative a nuvem gratuita em Ctrl+E"
	cor := m.cores().Suave
	if m.vault != nil && m.vault.TemConta() {
		texto = "☁̸  Conta criada · cofre não sincronizado · Ctrl+E para sincronizar"
		cor = lipgloss.Color("#F5C76B")
	}
	if syncer, ok := m.storage.(interface{ StatusSincronizacao() (bool, error) }); ok {
		pendente, err := syncer.StatusSincronizacao()
		switch {
		case err != nil:
			texto, cor = "☁̸  Sincronização pendente · cópia local preservada", lipgloss.Color("#F5C76B")
		case pendente:
			texto, cor = "☁̸  Sincronizando · há alterações pendentes", m.cores().Suave
		default:
			texto, cor = "☁  Nuvem conectada · sem envios pendentes", m.cores().Sucesso
		}
	}
	return lipgloss.NewStyle().Foreground(cor).Bold(true).Render(ansi.Truncate(texto, max(16, m.larguraAcesso()-4), "…"))
}
