package tui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func painelTeste() modeloPainel {
	return modeloPainel{tema: temas[0], largura: 80, altura: 24, atividade: time.Now()}
}

func TestPainelExplicacaoAcompanhaSelecaoSemExecutarAcao(t *testing.T) {
	for _, largura := range []int{40, 80, 120} {
		m := painelTeste()
		m.largura = largura
		resposta := make(chan respostaPainel, 1)
		m, _ = atualizarPainelTeste(m, pedidoPainel{tipo: "menu", titulo: "Conta", opcoes: []string{"Nuvem", "Planos", "Empresas", "Minha conta", "Voltar ao cofre"}, descricoes: []string{"Sincronização gratuita entre computadores.", "Contratação opcional para uso pessoal ou equipe."}, resposta: resposta})
		if !strings.Contains(m.View(), "Sincronização") || strings.Contains(m.View(), "Contratação opcional") {
			t.Fatal("explicação inicial incorreta")
		}
		m, _ = atualizarPainelTeste(m, tea.KeyMsg{Type: tea.KeyDown})
		view := m.View()
		if !strings.Contains(view, "Contratação opcional") || strings.Contains(view, "Sincronização gratuita") {
			t.Fatal("explicação não acompanhou a seleção")
		}
		if lipgloss.Width(view) > largura || lipgloss.Height(view) > m.altura {
			t.Fatalf("render ultrapassa terminal: %dx%d", lipgloss.Width(view), lipgloss.Height(view))
		}
		if largura >= 80 && !strings.Contains(view, "Voltar ao cofre") {
			t.Fatal("menu inicial não exibe as cinco opções")
		}
		select {
		case <-resposta:
			t.Fatal("navegar executou a opção")
		default:
		}
		m, _ = atualizarPainelTeste(m, tea.KeyMsg{Type: tea.KeyEnter})
		if r := <-resposta; r.indice != 1 || r.err != nil {
			t.Fatalf("opção errada: %+v", r)
		}
	}
}
func atualizarPainelTeste(m modeloPainel, msg tea.Msg) (modeloPainel, tea.Cmd) {
	n, c := m.Update(msg)
	return n.(modeloPainel), c
}

func TestPainelSenhaNaoApareceELimpaAoCancelarEConfirmar(t *testing.T) {
	for _, tecla := range []tea.KeyType{tea.KeyEsc, tea.KeyEnter} {
		m := painelTeste()
		r := make(chan respostaPainel, 1)
		m, _ = atualizarPainelTeste(m, pedidoPainel{titulo: "Senha mestra", tipo: "entrada", segredo: true, resposta: r})
		m, _ = atualizarPainelTeste(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("teste-ficticio-2468")})
		retido := m.entrada.secret.runes
		if strings.Contains(m.View(), "teste-ficticio") {
			t.Fatal("senha entrou no texto renderizado")
		}
		m, _ = atualizarPainelTeste(m, tea.KeyMsg{Type: tecla})
		resposta := <-r
		for _, r := range retido {
			if r != 0 {
				t.Fatal("buffer anterior não foi apagado")
			}
		}
		if tecla == tea.KeyEsc {
			if !errors.Is(resposta.err, ErrCancelado) || len(resposta.valor) != 0 {
				t.Fatal("cancelamento retornou senha")
			}
		} else {
			if string(resposta.valor) != "teste-ficticio-2468" {
				t.Fatal("senha alterada no transporte")
			}
			clear(resposta.valor)
		}
		if m.pedido != nil {
			t.Fatal("campo permaneceu aberto")
		}
	}
}

func TestPainelEsperaCancelaContextoAntesDeVoltar(t *testing.T) {
	m := painelTeste()
	r := make(chan respostaPainel, 1)
	iniciou := make(chan struct{})
	m, cmd := atualizarPainelTeste(m, pedidoPainel{titulo: "Pagamento", tipo: "espera", resposta: r, trabalho: func(ctx context.Context) error { close(iniciou); <-ctx.Done(); return ctx.Err() }})
	concluiu := make(chan tea.Msg, 1)
	go func() { concluiu <- cmd() }()
	<-iniciou
	m, _ = atualizarPainelTeste(m, tea.KeyMsg{Type: tea.KeyEsc})
	select {
	case <-r:
		t.Fatal("retornou antes de encerrar o trabalho")
	default:
	}
	select {
	case msg := <-concluiu:
		m, _ = atualizarPainelTeste(m, msg)
	case <-time.After(time.Second):
		t.Fatal("espera não recebeu cancelamento")
	}
	if resposta := <-r; !errors.Is(resposta.err, context.Canceled) {
		t.Fatal("erro de cancelamento perdido")
	}
	if m.cancel != nil || m.pedido != nil {
		t.Fatal("espera ou contexto residual")
	}
}

func TestPainelRolaSemEstourarTerminalESemEscapesRemotos(t *testing.T) {
	for _, largura := range []int{40, 80, 120} {
		for _, tipo := range []string{"menu", "mensagem", "espera"} {
			m := painelTeste()
			m.largura = largura
			m.altura = 24
			p := pedidoPainel{titulo: "Empresa \x1b]52;segredo\x07", tipo: tipo, texto: strings.Repeat("Uma linha comprida para consultar o contrato.\n", 40), resposta: make(chan respostaPainel, 1)}
			for i := 0; i < 30; i++ {
				p.opcoes = append(p.opcoes, "Item da empresa")
			}
			m.pedido = &p
			m.cursor = 29
			view := m.View()
			if strings.Contains(view, "\x1b]52") {
				t.Fatal("escape remoto executável")
			}
			if lipgloss.Width(view) > largura || lipgloss.Height(view) > 24 {
				t.Fatalf("%s em %d colunas: render %dx%d", tipo, largura, lipgloss.Width(view), lipgloss.Height(view))
			}
		}
	}
}

func TestPainelCampoVazioNaoConfirmaEInatividadeLimpa(t *testing.T) {
	m := painelTeste()
	r := make(chan respostaPainel, 1)
	m, _ = atualizarPainelTeste(m, pedidoPainel{tipo: "entrada", segredo: true, resposta: r})
	m, _ = atualizarPainelTeste(m, tea.KeyMsg{Type: tea.KeyEnter})
	select {
	case <-r:
		t.Fatal("campo vazio foi confirmado")
	default:
	}
	m, _ = atualizarPainelTeste(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("senha-fixture")})
	buf := m.entrada.secret.runes
	m.atividade = time.Now().Add(-6 * time.Minute)
	m, _ = atualizarPainelTeste(m, tickPainel(time.Now()))
	if !errors.Is((<-r).err, ErrCancelado) {
		t.Fatal("inatividade não cancelou")
	}
	for _, v := range buf {
		if v != 0 {
			t.Fatal("inatividade deixou senha no campo")
		}
	}
}

func TestPainelMantemUmaUnicaTelaDuranteTodoFluxo(t *testing.T) {
	var out bytes.Buffer
	err := executarPainel(func(p *PainelInterativo) error {
		// Envia teclas pelo próprio loop após a instalação de cada pedido.
		go func() { time.Sleep(30 * time.Millisecond); p.programa.Send(tea.KeyMsg{Type: tea.KeyEnter}) }()
		i, err := p.Escolher("Conta", []string{"Criar"})
		if err != nil || i != 0 {
			return errors.New("menu não confirmou")
		}
		go func() {
			time.Sleep(30 * time.Millisecond)
			p.programa.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("senha-fixture")})
			p.programa.Send(tea.KeyMsg{Type: tea.KeyEnter})
		}()
		b, err := p.Entrada("Senha mestra", "Somente neste computador", true)
		clear(b)
		if err != nil {
			return err
		}
		go func() { time.Sleep(30 * time.Millisecond); p.programa.Send(tea.KeyMsg{Type: tea.KeyEnter}) }()
		return p.Mensagem("Resultado", "Concluído")
	}, tea.WithInput(nil), tea.WithOutput(&out), tea.WithoutSignalHandler())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "senha-fixture") {
		t.Fatal("senha no terminal")
	}
	if strings.Count(out.String(), "\x1b[?1049h") != 1 || strings.Count(out.String(), "\x1b[?1049l") != 1 {
		t.Fatal("fluxo alternou entre console e interface")
	}
}
