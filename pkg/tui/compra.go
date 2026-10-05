package tui

import (
	"context"
	"errors"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"kofre/pkg/compra"
	"kofre/pkg/config"
	"kofre/pkg/storage"
)

type compraIniciadaMsg struct {
	geracao uint64
	sessao  *compra.Sessao
	ctx     context.Context
	err     error
}
type compraConcluidaMsg struct {
	geracao   uint64
	resultado compra.Resultado
	err       error
}

var abrirNavegadorCompra = compra.AbrirNavegador

func (m *Model) encerrarEsperaCompra() {
	m.compraGeracao++
	if m.compraCancel != nil {
		m.compraCancel()
		m.compraCancel = nil
	}
	if m.compraSessao != nil {
		m.compraSessao.Close()
		m.compraSessao = nil
	}
	m.compraEmAndamento = false
}

func (m Model) iniciarCompra() (tea.Model, tea.Cmd) {
	if m.compraEmAndamento {
		return m, nil
	}
	if _, ok := m.storage.(*storage.LocalStorage); !ok {
		m.err = errors.New("A ativação automática exige o cofre local. Sua sincronização atual não foi alterada.")
		return m, nil
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		m.err = err
		return m, nil
	}
	if cfg.CloudEnabled {
		m.err = errors.New("Já existe nuvem configurada. A compra não substituirá sua conta.")
		return m, nil
	}
	m.compraGeracao++
	geracao := m.compraGeracao
	ctx, cancel := context.WithCancel(context.Background())
	m.compraCancel = cancel
	m.compraEmAndamento = true
	m.err = nil
	m.notification = "Abrindo a compra no navegador…"
	endpoint := config.GetCloudEndpoint()
	hostname, _ := os.Hostname()
	return m, func() tea.Msg {
		sessao, err := compra.Iniciar(ctx, endpoint, hostname)
		return compraIniciadaMsg{geracao: geracao, sessao: sessao, ctx: ctx, err: err}
	}
}

func (m Model) receberInicioCompra(msg compraIniciadaMsg) (tea.Model, tea.Cmd) {
	if msg.geracao != m.compraGeracao || !m.compraEmAndamento {
		if msg.sessao != nil {
			msg.sessao.Close()
		}
		return m, nil
	}
	if msg.err != nil {
		m.encerrarEsperaCompra()
		m.err = msg.err
		m.notification = ""
		return m, nil
	}
	m.compraSessao = msg.sessao
	if err := abrirNavegadorCompra(msg.sessao.URL); err != nil {
		m.err = errors.New("Não consegui abrir o navegador. Pressione [r] para reabrir a compra vinculada a este Kofre.")
	}
	m.notification = "Compra iniciada. Pague no navegador; o Kofre ativa automaticamente após a confirmação."
	return m, func() tea.Msg {
		r, err := msg.sessao.Aguardar(msg.ctx)
		return compraConcluidaMsg{geracao: msg.geracao, resultado: r, err: err}
	}
}

func (m Model) receberConclusaoCompra(msg compraConcluidaMsg) (tea.Model, tea.Cmd) {
	if msg.geracao != m.compraGeracao || !m.compraEmAndamento {
		return m, nil
	}
	if msg.err != nil {
		m.encerrarEsperaCompra()
		m.err = msg.err
		m.notification = ""
		return m, nil
	}
	if msg.resultado.Status == "sandbox_confirmed" && msg.resultado.Sandbox {
		sessao := m.compraSessao
		m.encerrarEsperaCompra()
		return m, tea.Batch(m.notify("Compra de homologação confirmada. Sua conta e o cofre continuam como estavam; Sandbox não ativa Pro real."), confirmarCompraCmd(sessao))
	}
	if msg.resultado.Status != "active" || msg.resultado.Sandbox {
		m.encerrarEsperaCompra()
		m.err = errors.New("Resposta de ativação inválida; consulte seu pedido.")
		return m, nil
	}
	if m.compraSessao == nil {
		m.encerrarEsperaCompra()
		m.err = errors.New("Sessão de compra encerrada; consulte seu pedido.")
		return m, nil
	}
	endpoint := m.compraSessao.Endpoint()
	if err := config.AtivarLicencaComprada(endpoint, msg.resultado.Token); err != nil {
		m.encerrarEsperaCompra()
		m.err = errors.New("Pagamento confirmado, mas não consegui salvar a licença: " + err.Error() + ". Ela continua disponível no pedido.")
		m.notification = ""
		return m, nil
	}
	local, ok := m.storage.(*storage.LocalStorage)
	if !ok {
		m.encerrarEsperaCompra()
		m.err = errors.New("Licença salva; reabra o Kofre para aplicar a sincronização.")
		return m, nil
	}
	remote := storage.NewKofreCloudStorage(endpoint, msg.resultado.Token)
	syncer := storage.NewSyncStorage(local, remote)
	m.storage = syncer
	// Primeira sincronização conserva exatamente o arquivo cifrado local existente.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	if data, err := local.Load(ctx); err == nil {
		if err = syncer.Save(ctx, data); err != nil {
			m.err = err
		}
	} else if !errors.Is(err, storage.ErrNotFound) {
		m.err = err
	}
	cancel()
	sessao := m.compraSessao
	m.encerrarEsperaCompra()
	if m.state == ViewPro {
		if m.vault != nil {
			m.state = ViewList
			m.refreshList()
		} else {
			m.state = ViewUnlock
		}
	}
	if m.err != nil {
		m.err = errors.New("Pro ativado, mas o primeiro envio não foi concluído: " + m.err.Error())
		m.notification = ""
		return m, confirmarCompraCmd(sessao)
	}
	return m, tea.Batch(m.notify("Pro ativado automaticamente! Seu cofre local foi preservado; a sincronização está configurada."), confirmarCompraCmd(sessao))
}

func confirmarCompraCmd(sessao *compra.Sessao) tea.Cmd {
	return func() tea.Msg {
		if sessao != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			_ = sessao.Confirmar(ctx)
		}
		return nil
	}
}
