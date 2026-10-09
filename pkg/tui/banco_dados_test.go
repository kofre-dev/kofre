package tui

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"kofre/pkg/vault"
)

func formularioBancoFixture(t *testing.T) Model {
	m := fixtureModel(t)
	m.width, m.height = 100, 40
	m.initForm(vault.SecretEntry{Category: vault.CategoryDatabase}, false)
	m.state = ViewForm
	m.formInputs[0].SetValue("Produção fictícia")
	m.formInputs[campoHostBanco].SetValue("db.fixture.invalid")
	m.formInputs[campoNomeBanco].SetValue("delphos_fixture")
	m.formInputs[2].SetValue("usuario_fixture")
	m.formInputs[3].SetBytes([]byte(" senha-ficticia-db "))
	return m
}

func campoBancoTeste(t *testing.T, entry vault.SecretEntry, nome string) vault.Field {
	t.Helper()
	for _, f := range entry.Fields {
		if f.Name == nome {
			return f
		}
	}
	t.Fatalf("campo ausente: %s", nome)
	return vault.Field{}
}

func TestFormularioBancoDadosPersisteEditaSemExporSenha(t *testing.T) {
	m := formularioBancoFixture(t)
	defer m.Close()
	if strings.Contains(m.viewForm(), "senha-ficticia-db") {
		t.Fatal("senha no formulário")
	}
	antigoBuffer := m.formInputs[3].secret.runes
	next, _ := m.saveForm()
	m = next.(Model)
	if m.err != nil {
		t.Fatal(m.err)
	}
	for _, r := range antigoBuffer {
		if r != 0 {
			t.Fatal("senha digitada não foi apagada")
		}
	}
	raw := m.storage.(*memoryStore).data
	for _, texto := range []string{"db.fixture.invalid", "usuario_fixture", "delphos_fixture", "senha-ficticia-db"} {
		if bytes.Contains(raw, []byte(texto)) {
			t.Fatalf("arquivo cifrado expôs %s", texto)
		}
	}
	salt, payload, err := vault.UnpackHeader(raw)
	if err != nil {
		t.Fatal(err)
	}
	var reaberto *vault.ManagedVault
	err = m.sessionKey.WithBytes(func(key []byte) error { reaberto, err = vault.DecryptAndLoad(payload, key, salt); return err })
	if err != nil {
		t.Fatal(err)
	}
	defer reaberto.Close()
	entry := reaberto.Entries()[0]
	if entry.Category != vault.CategoryDatabase {
		t.Fatal("categoria perdida")
	}
	for nome, valor := range map[string]string{"Host": "db.fixture.invalid", "Porta": "5432", "Banco de dados": "delphos_fixture", "TLS": "Validar certificado", "Tipo de banco": "PostgreSQL"} {
		if campoBancoTeste(t, entry, nome).Value != valor {
			t.Fatalf("campo %s alterado", nome)
		}
	}
	secret := campoBancoTeste(t, entry, "Segredo")
	if !secret.Protected || secret.Value != "" {
		t.Fatal("senha não selada em memória")
	}
	if err := secret.WithValue(func(b []byte) error {
		if string(b) != " senha-ficticia-db " {
			t.Fatal("senha alterada")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.selectedEntry = m.vault.Entries()[0]
	m.initForm(m.selectedEntry, true)
	if m.formInputs[3].Value() != "" {
		t.Fatal("edição materializou senha")
	}
	m.formInputs[campoHostBanco].SetValue("novo.fixture.invalid")
	m.formInputs[campoPortaBanco].SetValue("6432")
	next, _ = m.saveForm()
	m = next.(Model)
	if m.err != nil {
		t.Fatal(m.err)
	}
	entry = m.vault.Entries()[0]
	if m.vault.Count() != 1 || campoBancoTeste(t, entry, "Host").Value != "novo.fixture.invalid" || campoBancoTeste(t, entry, "Porta").Value != "6432" {
		t.Fatal("edição duplicou item ou perdeu conexão")
	}
	if err := campoBancoTeste(t, entry, "Segredo").WithValue(func(b []byte) error {
		if string(b) != " senha-ficticia-db " {
			t.Fatal("edição vazia apagou senha")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.selectedEntry, m.state = entry, ViewDetail
	if strings.Contains(m.View(), "senha-ficticia-db") {
		t.Fatal("senha no detalhe")
	}
	m.selectedCatIdx = len(vault.AllCategories)
	m.searchInput.SetValue("novo.fixture.invalid")
	m.refreshList()
	if len(m.filteredItems) != 1 {
		t.Fatal("filtro/busca não encontrou conexão")
	}
	m.lock()
	if err := campoBancoTeste(t, entry, "Segredo").WithValue(func([]byte) error { return nil }); err == nil {
		t.Fatal("bloqueio manteve acesso à senha")
	}
}

func TestFormularioBancoDadosValidaPortaSQLiteECancelamento(t *testing.T) {
	for _, porta := range []string{"0", "65536", "abc", "-1"} {
		m := formularioBancoFixture(t)
		m.formInputs[campoPortaBanco].SetValue(porta)
		next, _ := m.saveForm()
		m = next.(Model)
		if m.err == nil || m.vault.Count() != 0 || len(m.storage.(*memoryStore).data) != 0 {
			t.Fatal("porta inválida gravada")
		}
		m.Close()
	}
	m := formularioBancoFixture(t)
	defer m.Close()
	m.formInputs[campoTipoBanco].SetValue("SQLite")
	next, _ := m.saveForm()
	m = next.(Model)
	if m.err == nil || m.vault.Count() != 0 {
		t.Fatal("SQLite sem caminho aceito")
	}
	m.formInputs[campoArquivoBanco].SetValue("./fixture.db")
	fields, err := m.camposBanco()
	if err != nil || len(fields) != 2 || fields[1].Name != "Arquivo" {
		t.Fatal("SQLite mantém campos de rede", err)
	}
	old := m.formInputs[3].secret.runes
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	for _, r := range old {
		if r != 0 {
			t.Fatal("cancelamento reteve senha")
		}
	}
	if m.vault.Count() != 0 || m.state != ViewList {
		t.Fatal("cancelamento salvou conexão")
	}
}

func TestFormularioBancoDadosSeletoresEResponsividade(t *testing.T) {
	for _, largura := range []int{40, 80, 120} {
		for _, altura := range []int{24, 30, 40, 65} {
			m := formularioBancoFixture(t)
			m.width, m.height = largura, altura
			for _, indice := range m.ordemFormulario() {
				m.formFocusIndex = indice
				view := m.View()
				if lipgloss.Width(view) > largura || lipgloss.Height(view) > altura {
					t.Fatalf("campo %d excede terminal %dx%d: %dx%d", indice, largura, altura, lipgloss.Width(view), lipgloss.Height(view))
				}
				if !strings.Contains(view, strings.TrimSuffix(m.formInputs[indice].Prompt, ": ")) {
					t.Fatalf("campo focado %d fora da área visível", indice)
				}
			}
			m.Close()
		}
	}
	m := formularioBancoFixture(t)
	defer m.Close()
	m.formFocusIndex = campoTipoBanco
	next, _ := m.updateForm(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	if m.formInputs[campoTipoBanco].Value() != "MySQL" || m.formInputs[campoPortaBanco].Value() != "3306" {
		t.Fatal("seletor não atualizou porta sugerida")
	}
	m.formInputs[campoPortaBanco].SetValue("3307")
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	if m.formInputs[campoPortaBanco].Value() != "3307" {
		t.Fatal("seletor sobrescreveu porta personalizada")
	}
	m.formFocusIndex = 1
	m.formInputs[1].SetValue("note")
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(Model)
	if !m.formularioBanco() {
		t.Fatal("categoria inacessível por setas")
	}
}
