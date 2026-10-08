package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"kofre/pkg/vault"
)

func TestFormularioCabeNoTerminal(t *testing.T) {
	for _, largura := range []int{40, 80, 120} {
		for _, altura := range []int{30, 40} {
			m := fixtureModel(t)
			m.width, m.height = largura, altura
			m.initForm(vault.SecretEntry{}, false)
			m.state = ViewForm
			view := m.View()
			if lipgloss.Width(view) > largura || lipgloss.Height(view) > altura {
				t.Fatalf("formulário %dx%d excedeu %dx%d", lipgloss.Width(view), lipgloss.Height(view), largura, altura)
			}
		}
	}
}

func TestFormularioCategoriaPrimeiroESegredoVazioComNotasLegiveis(t *testing.T) {
	m := fixtureModel(t)
	m.initForm(vault.SecretEntry{}, false)
	if m.formFocusIndex != 1 || !m.formInputs[1].Focused() {
		t.Fatal("categoria não é o primeiro campo")
	}
	view := m.viewForm()
	if strings.Index(view, "Categoria") > strings.Index(view, "Titulo") || strings.Contains(view, "••••") {
		t.Fatal("ordem ou senha vazia incorreta")
	}
	for _, esperado := range []int{0, 2, 3, 4, 1} {
		next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(Model)
		if m.formFocusIndex != esperado {
			t.Fatalf("foco inesperado: %d", m.formFocusIndex)
		}
	}
	m.formInputs[3].SetBytes([]byte("segredo-ficticio"))
	m.formInputs[4].SetBytes([]byte("https://fixture.example"))
	view = m.viewForm()
	if strings.Contains(view, "segredo-ficticio") || !strings.Contains(view, "https://fixture.example") {
		t.Fatal("senha exposta ou notas mascaradas")
	}
	notas := m.formInputs[4].secret.runes
	next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	for _, r := range notas {
		if r != 0 {
			t.Fatal("cancelar reteve notas")
		}
	}
}

type fixtureEstadoNuvem struct {
	memoryStore
	pendente bool
	err      error
}

func (s *fixtureEstadoNuvem) StatusSincronizacao() (bool, error) { return s.pendente, s.err }

func TestIndicadorNuvemDistingueContaDeSincronizacao(t *testing.T) {
	m := fixtureModel(t)
	if err := m.vault.DefinirConta([]byte(`{"id":"fixture"}`)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.indicadorNuvem(), "cofre não sincronizado") {
		t.Fatal("conta foi confundida com sincronização")
	}
	for _, caso := range []struct {
		pendente bool
		err      error
		texto    string
	}{
		{false, nil, "sem envios pendentes"},
		{true, nil, "alterações pendentes"},
		{true, errors.New("rede indisponível"), "cópia local preservada"},
	} {
		m.storage = &fixtureEstadoNuvem{pendente: caso.pendente, err: caso.err}
		if !strings.Contains(m.indicadorNuvem(), caso.texto) {
			t.Fatalf("estado incorreto: %s", m.indicadorNuvem())
		}
	}
}
