package tui

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"kofre/pkg/config"
	"kofre/pkg/storage"
)

func TestTemasPreviaCancelarSalvarERetomar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := config.DefaultConfig()
	cfg.VaultPath = filepath.Join(dir, "cofre.enc")
	cfg.CloudEndpoint = "https://exemplo.test"
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	store, err := storage.NewLocalStorage(cfg.VaultPath)
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(store)
	defer m.Close()
	outro := NewModel(store)
	defer outro.Close()
	m.passInput.SetValue("senha-preservada")
	key := func(tipo tea.KeyType) {
		next, _ := m.Update(tea.KeyMsg{Type: tipo})
		m = next.(Model)
	}
	key(tea.KeyF2)
	key(tea.KeyDown)
	if m.cores().ID != "violeta" || outro.cores().ID != "azul" {
		t.Fatal("prévia não mudou ou contaminou outro modelo")
	}
	key(tea.KeyEsc)
	saved, _ := config.LoadConfig()
	if m.cores().ID != "azul" || saved.Tema != "" || m.passInput.Value() != "senha-preservada" {
		t.Fatal("cancelar alterou configuração ou senha")
	}
	key(tea.KeyF2)
	key(tea.KeyDown)
	key(tea.KeyEnter)
	saved, err = config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Tema != "violeta" || saved.VaultPath != cfg.VaultPath || saved.CloudEndpoint != cfg.CloudEndpoint || saved.CloudEnabled != cfg.CloudEnabled {
		t.Fatal("tema não persistiu ou alterou configuração alheia")
	}
	reopened := NewModel(store)
	defer reopened.Close()
	if reopened.cores().ID != "violeta" || m.state != ViewUnlock || m.mostrarTemas {
		t.Fatal("tema não restaurou ou seleção alterou navegação")
	}
}

func TestTemaAntigoOuDesconhecidoMantemAzul(t *testing.T) {
	for _, id := range []string{"", "tema-inexistente"} {
		if (Model{temaID: id}).cores().ID != "azul" {
			t.Fatal("configuração anterior perdeu o tema padrão")
		}
	}
}
