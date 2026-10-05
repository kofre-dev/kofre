package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) painelPro() string {
	largura := min(72, m.larguraAcesso()-4)
	conteudo := largura - 6
	centro := lipgloss.NewStyle().Width(conteudo).Align(lipgloss.Center)
	texto := lipgloss.NewStyle().Width(conteudo).Foreground(textoAcesso)
	botao := centro.Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#006CFF"))
	var b strings.Builder
	b.WriteString(centro.Bold(true).Foreground(azulAcesso).Render("Kofre Cloud Pro") + "\n")
	if isProPlan() {
		b.WriteString(centro.Foreground(verdeAcesso).Render("Licença configurada") + "\n\n")
		b.WriteString(texto.Render("Seu cofre sincronizado, com criptografia E2E.") + "\n\n")
		b.WriteString(texto.Render("Acesso à nuvem conforme a validade da licença.") + "\n")
		b.WriteString(texto.Render("O cofre local continua disponível.") + "\n\n")
		b.WriteString(botao.Render("Voltar ao cofre  [Enter]  →"))
	} else if m.compraEmAndamento {
		b.WriteString(centro.Foreground(verdeAcesso).Render("Aguardando confirmação") + "\n\n")
		b.WriteString(texto.Render("Continue a compra no navegador.") + "\n")
		b.WriteString(texto.Render("Mantenha o Kofre aberto para receber o resultado.") + "\n\n")
		b.WriteString(botao.Render("Reabrir navegador  [r]  →") + "\n\n")
		b.WriteString(texto.Render("[x] Interromper espera"))
	} else {
		b.WriteString(centro.Foreground(textoAcesso).Render("Seu cofre, conectado entre PCs.") + "\n\n")
		for _, beneficio := range []string{
			"✓ Sincronização com criptografia E2E",
			"✓ Acesso nos seus computadores",
			"✓ Desbloqueio e recuperação pelo Telegram",
			"✓ Bloqueio de pânico pelo Telegram",
		} {
			b.WriteString(texto.Render(beneficio) + "\n")
		}
		b.WriteString("\n" + botao.Render("Conhecer planos e comprar  [Enter]  →") + "\n\n")
		b.WriteString(texto.Render("Compra no navegador, vinculada a este Kofre.") + "\n")
		b.WriteString(texto.Render("Após pagar, mantenha o app aberto para ativar.") + "\n")
		b.WriteString(texto.Render("Mantenha também seus backups locais."))
	}
	b.WriteString("\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#24445E")).Render(strings.Repeat("─", conteudo)))
	voltar := "[Esc] Voltar"
	if m.compraEmAndamento && !isProPlan() {
		voltar = "[Esc] Usar o cofre enquanto aguarda"
	}
	b.WriteString("\n" + texto.Render(voltar))
	return lipgloss.NewStyle().Width(largura).Padding(1, 3).Border(lipgloss.RoundedBorder()).BorderForeground(azulAcesso).Render(b.String())
}
