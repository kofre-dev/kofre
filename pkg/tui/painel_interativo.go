package tui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"kofre/pkg/config"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/updater"
)

var ErrCancelado = errors.New("ação cancelada")

// PainelInterativo mantém um único programa e a tela alternativa durante todo
// o fluxo. A lógica de conta roda fora do loop visual; não há captura de stdout.
type PainelInterativo struct {
	programa *tea.Program
	fechado  chan struct{}
}

type pedidoPainel struct {
	titulo, texto string
	opcoes        []string
	descricoes    []string
	tipo          string
	segredo       bool
	resposta      chan respostaPainel
	trabalho      func(context.Context) error
	revelar       func(func([]byte) error) error
}
type respostaPainel struct {
	indice int
	valor  []byte
	err    error
}
type fimPainel struct{ err error }
type trabalhoPainelConcluido struct{ err error }
type tickPainel time.Time

type modeloPainel struct {
	painel                                *PainelInterativo
	acao                                  func() error
	pedido                                *pedidoPainel
	entrada                               input
	cursor, largura, altura, deslocamento int
	tema                                  paletaTema
	atividade                             time.Time
	cancel                                context.CancelFunc
	err                                   error
	aviso                                 string
}

func ExecutarPainel(acao func(*PainelInterativo) error) error {
	return executarPainel(acao)
}

func executarPainel(acao func(*PainelInterativo) error, opcoes ...tea.ProgramOption) error {
	p := &PainelInterativo{fechado: make(chan struct{})}
	tema := temas[0]
	if cfg, err := config.LoadConfig(); err == nil {
		tema = temas[indiceTema(cfg.Tema)]
	}
	m := modeloPainel{painel: p, acao: func() error { return acao(p) }, tema: tema, largura: 80, altura: 24, atividade: time.Now()}
	opcoes = append([]tea.ProgramOption{tea.WithAltScreen()}, opcoes...)
	p.programa = tea.NewProgram(m, opcoes...)
	defer close(p.fechado)
	final, err := p.programa.Run()
	if modelo, ok := final.(modeloPainel); ok {
		modelo.entrada.Reset()
		if modelo.cancel != nil {
			modelo.cancel()
		}
	}
	if err != nil {
		return err
	}
	return final.(modeloPainel).err
}

func (p *PainelInterativo) solicitar(pedido pedidoPainel) respostaPainel {
	pedido.resposta = make(chan respostaPainel, 1)
	p.programa.Send(pedido)
	select {
	case r := <-pedido.resposta:
		return r
	case <-p.fechado:
		return respostaPainel{indice: -1, err: ErrCancelado}
	}
}
func (p *PainelInterativo) Escolher(titulo string, opcoes []string) (int, error) {
	return p.EscolherComDescricao(titulo, opcoes, nil)
}
func (p *PainelInterativo) EscolherComDescricao(titulo string, opcoes, descricoes []string) (int, error) {
	r := p.solicitar(pedidoPainel{titulo: titulo, opcoes: opcoes, descricoes: descricoes, tipo: "menu"})
	if errors.Is(r.err, ErrCancelado) {
		return -1, nil
	}
	return r.indice, r.err
}
func (p *PainelInterativo) Entrada(titulo, texto string, segredo bool) ([]byte, error) {
	r := p.solicitar(pedidoPainel{titulo: titulo, texto: texto, tipo: "entrada", segredo: segredo})
	return r.valor, r.err
}
func (p *PainelInterativo) Mensagem(titulo, texto string) error {
	return p.solicitar(pedidoPainel{titulo: titulo, texto: texto, tipo: "mensagem"}).err
}
func (p *PainelInterativo) Esperar(titulo, texto string, trabalho func(context.Context) error) error {
	return p.solicitar(pedidoPainel{titulo: titulo, texto: texto, tipo: "espera", trabalho: trabalho}).err
}
func (p *PainelInterativo) Revelar(titulo string, revelar func(func([]byte) error) error) error {
	return p.solicitar(pedidoPainel{titulo: titulo, tipo: "segredo", revelar: revelar}).err
}

func pulsoPainel() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickPainel(t) })
}
func (m modeloPainel) Init() tea.Cmd {
	return tea.Batch(pulsoPainel(), func() tea.Msg { return fimPainel{m.acao()} })
}
func (m *modeloPainel) responder(r respostaPainel) {
	m.entrada.Reset()
	m.pedido.resposta <- r
	m.pedido = nil
	m.aviso = ""
}
func (m modeloPainel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.largura, m.altura = v.Width, v.Height
	case pedidoPainel:
		m.pedido = &v
		m.cursor = 0
		m.deslocamento = 0
		m.aviso = ""
		m.atividade = time.Now()
		m.entrada.Reset()
		m.entrada = newInput(v.segredo)
		m.entrada.Prompt = "> "
		m.entrada.Placeholder = "Digite aqui"
		if v.tipo == "entrada" {
			return m, m.entrada.Focus()
		}
		if v.tipo == "espera" {
			ctx, cancel := context.WithCancel(context.Background())
			m.cancel = cancel
			return m, func() tea.Msg { return trabalhoPainelConcluido{v.trabalho(ctx)} }
		}
		if v.tipo == "segredo" {
			return m, tea.Exec(&ephemeralSecretViewer{title: v.titulo, withValue: v.revelar}, func(err error) tea.Msg { return trabalhoPainelConcluido{err} })
		}
	case trabalhoPainelConcluido:
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}
		if m.pedido != nil && (m.pedido.tipo == "espera" || m.pedido.tipo == "segredo") {
			m.responder(respostaPainel{err: v.err})
		}
	case fimPainel:
		m.entrada.Reset()
		m.err = v.err
		return m, tea.Quit
	case tickPainel:
		if m.pedido != nil && time.Since(m.atividade) > 5*time.Minute {
			if m.cancel != nil {
				m.cancel()
			} else {
				m.responder(respostaPainel{indice: -1, err: ErrCancelado})
			}
		}
		return m, pulsoPainel()
	case tea.KeyMsg:
		m.atividade = time.Now()
		if m.pedido == nil {
			return m, nil
		}
		p := m.pedido
		if v.String() == "esc" || v.String() == "ctrl+c" {
			if p.tipo == "espera" {
				if m.cancel != nil {
					m.cancel()
				}
				m.aviso = "Interrompendo a espera…"
			} else {
				m.responder(respostaPainel{indice: -1, err: ErrCancelado})
			}
			return m, nil
		}
		if p.tipo == "menu" {
			switch v.String() {
			case "up", "k":
				m.cursor = max(0, m.cursor-1)
			case "down", "j":
				m.cursor = min(len(p.opcoes)-1, m.cursor+1)
			case "enter":
				if len(p.opcoes) > 0 {
					m.responder(respostaPainel{indice: m.cursor})
				}
			}
		} else if p.tipo == "mensagem" {
			switch v.String() {
			case "enter":
				m.responder(respostaPainel{})
			case "up", "k":
				m.deslocamento = max(0, m.deslocamento-1)
			case "down", "j":
				m.deslocamento++
			case "pgdown":
				m.deslocamento += max(1, m.altura-13)
			case "pgup":
				m.deslocamento = max(0, m.deslocamento-max(1, m.altura-13))
			}
		} else if p.tipo == "entrada" {
			if v.String() == "enter" {
				valor := m.entrada.Bytes()
				if len(bytes.TrimSpace(valor)) == 0 {
					mycrypto.ZeroBytes(valor)
					m.aviso = "Preencha este campo para continuar."
					return m, nil
				}
				m.responder(respostaPainel{valor: valor})
				return m, nil
			}
			var cmd tea.Cmd
			m.entrada, cmd = m.entrada.Update(v)
			return m, cmd
		}
	}
	return m, nil
}

func (m modeloPainel) View() string {
	w := max(12, min(72, m.largura-8))
	titulo, texto, rodape := "Conta e empresas", "Processando…", "Aguarde a conclusão da operação."
	espacoRodape := "\n\n"
	p := m.pedido
	if p != nil {
		titulo, texto = p.titulo, p.texto
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(m.tema.Destaque).Width(w).Render(ansi.Truncate(textoSeguroMenu(titulo), w, "…")) + "\n\n")
	if p == nil {
		b.WriteString(lipgloss.NewStyle().Foreground(m.tema.Suave).Render(texto))
	} else {
		switch p.tipo {
		case "menu":
			var explicacao string
			reserva := 0
			if m.cursor >= 0 && m.cursor < len(p.descricoes) && p.descricoes[m.cursor] != "" {
				linhas := strings.Split(ansi.Wrap(textoSeguroMenu(p.descricoes[m.cursor]), w-4, ""), "\n")
				if len(linhas) > 2 {
					linhas = linhas[:2]
					linhas[1] = ansi.Truncate(linhas[1], w-5, "") + "…"
				}
				explicacao = lipgloss.NewStyle().Width(w-2).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(m.tema.Borda).Foreground(m.tema.Suave).Render(strings.Join(linhas, "\n"))
				reserva = lipgloss.Height(explicacao) + 1
			}
			visiveis := max(1, m.altura-17-reserva)
			if explicacao != "" {
				visiveis = max(1, m.altura-14-reserva)
				espacoRodape = "\n"
			}
			inicio := max(0, m.cursor-visiveis+1)
			for i := inicio; i < min(len(p.opcoes), inicio+visiveis); i++ {
				linha := "  " + textoSeguroMenu(p.opcoes[i])
				estilo := lipgloss.NewStyle().Width(w).Foreground(m.tema.Texto)
				if i == m.cursor {
					linha = "› " + textoSeguroMenu(p.opcoes[i])
					estilo = estilo.Bold(true).Background(m.tema.Selecao)
				}
				b.WriteString(estilo.Render(ansi.Truncate(linha, w, "…")) + "\n")
			}
			if explicacao != "" {
				b.WriteString("\n" + explicacao)
			}
			rodape = "↑↓ Escolher  ·  Enter Abrir  ·  Esc Voltar"
		case "entrada":
			b.WriteString(lipgloss.NewStyle().Width(w).Foreground(m.tema.Suave).Render(textoSeguroMenu(texto)) + "\n\n")
			b.WriteString(lipgloss.NewStyle().Width(w-4).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(m.tema.Destaque).Foreground(m.tema.Texto).Render(ansi.Truncate(m.entrada.View(), w-4, "…")))
			rodape = "Enter Continuar  ·  Esc Cancelar"
		case "mensagem", "espera":
			linhas := strings.Split(ansi.Wrap(textoSeguroPainel(texto), w, ""), "\n")
			h := max(1, m.altura-17)
			inicio := min(m.deslocamento, max(0, len(linhas)-h))
			b.WriteString(lipgloss.NewStyle().Foreground(m.tema.Texto).Render(strings.Join(linhas[inicio:min(len(linhas), inicio+h)], "\n")))
			rodape = "Enter Voltar  ·  Esc Voltar"
			if len(linhas) > h {
				rodape = "↑↓ Rolar  ·  " + rodape
			}
			if p.tipo == "espera" {
				rodape = "Esc Interromper espera · o pedido permanece disponível"
			}
		}
	}
	if m.aviso != "" {
		b.WriteString("\n\n" + lipgloss.NewStyle().Width(w).Foreground(m.tema.Suave).Render(m.aviso))
	}
	b.WriteString(espacoRodape + lipgloss.NewStyle().Foreground(m.tema.Borda).Render(strings.Repeat("─", w)) + "\n" + lipgloss.NewStyle().Width(w).Foreground(m.tema.Suave).Render(rodape))
	cabecalho := lipgloss.NewStyle().Foreground(m.tema.Destaque).Bold(true).Render("🛡️  Kofre") + lipgloss.NewStyle().Foreground(m.tema.Suave).Render("   v"+updater.CurrentVersion+"   ·   Conta e empresas")
	cabecalho = ansi.Truncate(cabecalho, w+6, "…")
	return "\n" + cabecalho + "\n" + lipgloss.NewStyle().Foreground(m.tema.Destaque).Render(strings.Repeat("─", w+6)) + "\n\n" + lipgloss.NewStyle().Padding(1, 2).Border(lipgloss.RoundedBorder()).BorderForeground(m.tema.Destaque).Render(b.String()) + "\n"
}

func textoSeguroPainel(v string) string {
	linhas := strings.Split(v, "\n")
	for i := range linhas {
		linhas[i] = textoSeguroMenu(linhas[i])
	}
	return strings.Join(linhas, "\n")
}
