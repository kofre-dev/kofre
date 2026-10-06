package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"kofre/pkg/config"
	"kofre/pkg/storage"
	"os"
	"os/exec"
)

type contaRetornouMsg struct {
	err           error
	local, remote storage.StorageProvider
}
type planoAtualizadoMsg struct{ err error }

func atualizarPlanoCmd() tea.Msg { return planoAtualizadoMsg{config.AtualizarPlanoCloud()} }

func (m Model) abrirConta() (tea.Model, tea.Cmd) {
	exe, err := os.Executable()
	if err != nil {
		m.err = err
		return m, nil
	}
	local := m.storage
	var remoto storage.StorageProvider
	if sync, ok := m.storage.(*storage.SyncStorage); ok {
		local, remoto = sync.Providers()
	}
	// Encerra segredos, área de transferência e envio antes de suspender a TUI.
	m.cleanup()
	args := []string{"conta"}
	if arquivo, ok := local.(*storage.LocalStorage); ok {
		args = append(args, "--vault", arquivo.Path())
	}
	cmd := exec.Command(exe, args...)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return contaRetornouMsg{err, local, remoto} })
}

func (m Model) retomarConta(msg contaRetornouMsg) (tea.Model, tea.Cmd) {
	remoto := msg.remote
	_, nuvemKofre := remoto.(*storage.KofreCloudStorage)
	// S3 próprio (inclusive por flag/ambiente) conserva o destino explícito.
	// A conta pessoal só reativa a nuvem quando a configuração ainda a habilita.
	if remoto == nil || nuvemKofre {
		remoto = nil
		if cfg, err := config.LoadConfig(); err == nil && cfg.CloudEnabled && cfg.Mode != "custom_s3" && cfg.KofreToken != "" {
			remoto = storage.NewKofreCloudStorage(config.GetCloudEndpoint(), cfg.KofreToken, cfg.ContaID)
		}
	}
	m.storage = msg.local
	if remoto != nil {
		m.storage = storage.NewSyncStorage(msg.local, remoto)
	}
	m.state = ViewUnlock
	m.err = msg.err
	return m, tea.Batch(m.passInput.Focus(), atualizarPlanoCmd)
}
