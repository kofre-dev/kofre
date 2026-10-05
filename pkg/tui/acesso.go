package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/updater"
)

var (
	azulAcesso  = lipgloss.Color("#009DFF")
	textoAcesso = lipgloss.Color("#A5BEDF")
	verdeAcesso = lipgloss.Color("#36E6BA")
)

func (m Model) larguraAcesso() int {
	if m.width > 0 {
		return max(24, min(104, m.width-4))
	}
	return 96
}

func (m Model) cabecalhoAcesso() string {
	w := m.larguraAcesso()
	marca := lipgloss.NewStyle().Bold(true).Foreground(azulAcesso).Render("\U0001F6E1\uFE0F  Kofre")
	versao := lipgloss.NewStyle().Foreground(textoAcesso).Render("v" + updater.CurrentVersion)
	plano := "FREE"
	if isProPlan() {
		plano = "PRO"
	}
	badge := lipgloss.NewStyle().Foreground(azulAcesso).Border(lipgloss.RoundedBorder()).BorderForeground(azulAcesso).Padding(0, 1).Render(plano)
	esquerda := lipgloss.JoinHorizontal(lipgloss.Center, marca+"   "+versao+"   ", badge)
	if w < 60 {
		esquerda = marca + "  " + versao + "  " + plano
	}
	larguraQuadro := min(72, w-4) + 2 // Inclui as duas bordas do painel.
	if m.state == ViewList {
		larguraQuadro = w + 2
	}
	return esquerda + "\n" + lipgloss.NewStyle().Foreground(azulAcesso).Render(strings.Repeat("─", larguraQuadro))
}

func cadeadoAcesso() string {
	linhas := []string{"   ▄████▄   ", "  ██    ██  ", "  ██    ██  ", "▄██████████▄", "█████▀▀█████", "█████  █████", "▀██████████▀"}
	cores := []string{"#36D9FF", "#25CCFF", "#16BCFF", "#07ADFF", "#009DFF", "#008CFF", "#0078F0"}
	for i := range linhas {
		linhas[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(cores[i])).Render(linhas[i])
	}
	return strings.Join(linhas, "\n")
}

func (m Model) painelAcesso() string {
	w := m.larguraAcesso()
	compacto := (m.height > 0 && m.height < 36) || w < 60
	panelWidth := min(72, w-4)
	conteudo := panelWidth - 6
	centro := lipgloss.NewStyle().Width(conteudo).Align(lipgloss.Center)
	var b strings.Builder
	if !compacto {
		b.WriteString(centro.Render(cadeadoAcesso()) + "\n\n")
	}
	titulo, acao := "Abra seu cofre", "Abrir cofre"
	if m.isNewVault {
		titulo, acao = "Crie seu cofre", "Criar cofre"
	}
	b.WriteString(centro.Bold(true).Foreground(colorText).Render(titulo))
	b.WriteString("\n" + centro.Foreground(textoAcesso).Render("Senha mestra ou PIN") + "\n\n")
	entrada := m.passInput
	entrada.Prompt = "> "
	entrada.Placeholder = "Digite sua senha"
	campo := ansi.Truncate(entrada.View(), conteudo-4, "…")
	b.WriteString(lipgloss.NewStyle().Width(conteudo-2).Padding(0, 1).Foreground(textoAcesso).Border(lipgloss.RoundedBorder()).BorderForeground(azulAcesso).Render(campo))
	b.WriteString("\n\n")
	b.WriteString(centro.Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#006CFF")).Render(acao + "  [Enter]  →"))
	b.WriteString("\n\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("#24445E")).Render(strings.Repeat("─", conteudo)) + "\n")
	link := lipgloss.NewStyle().Foreground(azulAcesso).Render(botaoAtivarPro)
	if isProPlan() {
		link = lipgloss.NewStyle().Foreground(verdeAcesso).Render("Cloud Pro ativo")
		if !m.isNewVault && mycrypto.HasTelegramUnlockEnvelope() {
			link = lipgloss.NewStyle().Foreground(azulAcesso).Render("Telegram  [Ctrl+T]")
		}
	}
	sair := lipgloss.NewStyle().Foreground(textoAcesso).Render("[Esc] Sair")
	if conteudo >= lipgloss.Width(link)+lipgloss.Width(sair)+2 {
		b.WriteString(link + strings.Repeat(" ", conteudo-lipgloss.Width(link)-lipgloss.Width(sair)) + sair)
	} else {
		b.WriteString(link + "\n" + sair)
	}
	painel := lipgloss.NewStyle().Width(panelWidth).Padding(1, 3).Border(lipgloss.RoundedBorder()).BorderForeground(azulAcesso).Render(b.String())
	saida := painel
	if !compacto {
		rodape := lipgloss.NewStyle().Foreground(verdeAcesso).Render("🔐  Criptografia E2E  ·  💻  Local primeiro  ·  ☁\uFE0F  Nuvem opcional")
		saida += "\n\n" + lipgloss.NewStyle().Width(panelWidth+2).Render(rodape)
	}
	return saida
}
