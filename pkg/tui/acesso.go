package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/updater"
)

func (m Model) larguraAcesso() int {
	if m.width > 0 {
		if m.state == ViewList {
			return max(24, m.width-4)
		}
		return max(24, min(104, m.width-4))
	}
	return 96
}

func (m Model) cabecalhoAcesso() string {
	w := m.larguraAcesso()
	marca := lipgloss.NewStyle().Bold(true).Foreground(m.cores().Destaque).Render("\U0001F6E1\uFE0F  Kofre")
	versao := lipgloss.NewStyle().Foreground(m.cores().Suave).Render("v" + updater.CurrentVersion)
	plano := "FREE"
	if isProPlan() {
		plano = "PRO"
	}
	badge := lipgloss.NewStyle().Foreground(m.cores().Destaque).Border(lipgloss.RoundedBorder()).BorderForeground(m.cores().Destaque).Padding(0, 1).Render(plano)
	esquerda := lipgloss.JoinHorizontal(lipgloss.Center, marca+"   "+versao+"   ", badge)
	if w < 60 {
		esquerda = marca + "  " + versao + "  " + plano
	}
	larguraQuadro := min(72, w-4) + 2 // Inclui as duas bordas do painel.
	if m.state == ViewList || m.state == ViewForm {
		larguraQuadro = w + 2
	}
	if m.vault != nil && m.state != ViewUnlock && m.state != ViewTelegramChallenge {
		// Mesma cor do indicador da landing; VS15 solicita glifo de texto sem cores de emoji.
		status := lipgloss.NewStyle().Foreground(lipgloss.Color("#67E8B3")).Render("\U0001F513\uFE0E ABERTO")
		espaco := larguraQuadro - lipgloss.Width(esquerda) - lipgloss.Width(status)
		if espaco >= 2 {
			esquerda = lipgloss.JoinHorizontal(lipgloss.Center, esquerda, strings.Repeat(" ", espaco), status)
		} else {
			esquerda += "\n" + lipgloss.NewStyle().Width(larguraQuadro).Align(lipgloss.Right).Render(status)
		}
	}
	return esquerda + "\n" + lipgloss.NewStyle().Foreground(m.cores().Destaque).Render(strings.Repeat("─", larguraQuadro))
}

func (m Model) cadeadoAcesso() string {
	linhas := []string{"   ▄████▄   ", "  ██    ██  ", "  ██    ██  ", "▄██████████▄", "█████▀▀█████", "█████  █████", "▀██████████▀"}
	cores := []lipgloss.Color{m.cores().Brilho, m.cores().Brilho, m.cores().Destaque, m.cores().Destaque, m.cores().Destaque, m.cores().Botao, m.cores().Botao}
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
		b.WriteString(centro.Render(m.cadeadoAcesso()) + "\n\n")
	}
	titulo, acao := "Abra seu cofre", "Abrir cofre"
	orientacao := "Senha mestra ou PIN"
	if m.isNewVault {
		titulo, acao = "Crie seu cofre", "Criar cofre"
		orientacao = "6+ caracteres · letra, símbolo ou espaço interno"
	}
	b.WriteString(centro.Bold(true).Foreground(m.cores().Texto).Render(titulo))
	b.WriteString("\n" + centro.Foreground(m.cores().Suave).Render(orientacao) + "\n\n")
	entrada := m.passInput
	entrada.Prompt = "> "
	entrada.Placeholder = "Digite sua senha"
	campo := ansi.Truncate(entrada.View(), conteudo-4, "…")
	b.WriteString(lipgloss.NewStyle().Width(conteudo-2).Padding(0, 1).Foreground(m.cores().Suave).Border(lipgloss.RoundedBorder()).BorderForeground(m.cores().Destaque).Render(campo))
	b.WriteString("\n\n")
	if segundos := m.tentativas.Restante(time.Now()); segundos > 0 {
		acao = fmt.Sprintf("Aguarde %ds", segundos)
	}
	b.WriteString(centro.Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(m.cores().Botao).Render(acao + "  [Enter]  →"))
	b.WriteString("\n\n" + lipgloss.NewStyle().Foreground(m.cores().Borda).Render(strings.Repeat("─", conteudo)) + "\n")
	link := lipgloss.NewStyle().Foreground(m.cores().Destaque).Render(botaoAtivarPro)
	if isProPlan() {
		link = lipgloss.NewStyle().Foreground(m.cores().Sucesso).Render("Cloud Pro ativo")
		if !m.isNewVault && mycrypto.HasTelegramUnlockEnvelope() {
			link = lipgloss.NewStyle().Foreground(m.cores().Destaque).Render("Telegram  [Ctrl+T]")
		}
	}
	sair := lipgloss.NewStyle().Foreground(m.cores().Suave).Render("[Esc] Sair")
	if conteudo >= lipgloss.Width(link)+lipgloss.Width(sair)+2 {
		b.WriteString(link + strings.Repeat(" ", conteudo-lipgloss.Width(link)-lipgloss.Width(sair)) + sair)
	} else {
		b.WriteString(link + "\n" + sair)
	}
	painel := lipgloss.NewStyle().Width(panelWidth).Padding(1, 3).Border(lipgloss.RoundedBorder()).BorderForeground(m.cores().Destaque).Render(b.String())
	saida := painel
	if !compacto {
		rodape := lipgloss.NewStyle().Foreground(m.cores().Sucesso).Render("🔐  Criptografia E2E  ·  💻  Local primeiro  ·  ☁\uFE0F  Nuvem opcional")
		saida += "\n\n" + lipgloss.NewStyle().Width(panelWidth+2).Render(rodape)
	}
	return saida
}
