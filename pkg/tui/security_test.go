package tui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/vault"
	"strings"
	"testing"
	"time"
)

func TestClipboardRetentaSemPerderControleOuApagarNovoConteudo(t *testing.T) {
	previousRead, previousWrite := readClipboard, writeClipboard
	t.Cleanup(func() { readClipboard, writeClipboard = previousRead, previousWrite })
	clipboardValue := "fixture"
	falhar := true
	tentativas := 0
	readClipboard = func() (string, error) { return clipboardValue, nil }
	writeClipboard = func(s string) error {
		tentativas++
		if falhar {
			return errors.New("ocupado")
		}
		clipboardValue = s
		return nil
	}
	m := fixtureModel(t)
	m.clipboardHash, m.clipboardOwned = sha256.Sum256([]byte(clipboardValue)), true
	m.clearClipboard()
	if !m.clipboardOwned || m.err == nil {
		t.Fatal("falha de limpeza foi tratada como sucesso")
	}
	falhar = false
	m.clipboardRetryAt = time.Now().Add(-time.Second)
	next, _ := m.Update(securityTickMsg{})
	m = next.(Model)
	if clipboardValue != "" || m.clipboardOwned || tentativas != 2 {
		t.Fatal("limpeza não foi repetida após falha transitória")
	}
	clipboardValue = "fixture"
	m.clipboardHash, m.clipboardOwned = sha256.Sum256([]byte(clipboardValue)), true
	falhar = true
	m.clearClipboard()
	clipboardValue = "outro aplicativo"
	m.clearClipboard()
	if clipboardValue != "outro aplicativo" || m.clipboardOwned {
		t.Fatal("limpeza interferiu no conteúdo de outro aplicativo")
	}
}

func TestClipboardFalhaPersistenteTemLimite(t *testing.T) {
	previousRead, previousWrite := readClipboard, writeClipboard
	t.Cleanup(func() { readClipboard, writeClipboard = previousRead, previousWrite })
	tentativas := 0
	readClipboard = func() (string, error) { tentativas++; return "", errors.New("ocupado") }
	m := fixtureModel(t)
	m.clipboardOwned = true
	for i := 0; i < 10; i++ {
		m.clearClipboard()
	}
	if tentativas != 3 || !m.clipboardOwned || m.err == nil {
		t.Fatal("falha não foi limitada e informada")
	}
	m.clipboardOwned = false // Evita repetir o aviso da fixture no cleanup.
}

func TestNotasNaoVazamNaViewEEdicaoPreservaConteudo(t *testing.T) {
	m := fixtureModel(t)
	const nota = "nota-ficticia-nao-pode-aparecer-na-view"
	entry, err := m.vault.AddEntry(vault.SecretEntry{Title: "Teste", Notes: nota})
	if err != nil {
		t.Fatal(err)
	}
	m.selectedEntry = entry
	m.state = ViewDetail
	if strings.Contains(m.viewDetail(), nota) {
		t.Fatal("nota apareceu no buffer de renderização")
	}
	if !m.notasSelecionadas() {
		t.Fatal("nota sem campos não pode ser selecionada")
	}
	m.initForm(entry, true)
	if !strings.Contains(m.viewForm(), nota) {
		t.Fatal("nota deve ficar legível durante a edição explícita")
	}
	m.formInputs[0].SetValue("Novo título")
	next, _ := m.saveForm()
	m = next.(Model)
	if m.err != nil {
		t.Fatal(m.err)
	}
	atual, err := m.vault.GetEntry(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = atual.WithNotes(func(b []byte) error {
		if string(b) != nota {
			t.Fatal("edição descartou nota protegida")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.lock()
	if err = atual.WithNotes(func([]byte) error { return nil }); err == nil {
		t.Fatal("lock deixou nota acessível")
	}
}

type confirmarStore struct {
	memoryStore
	confirmacoes int
	confirmarErr error
}

func (s *confirmarStore) ConfirmarCarga(context.Context, []byte) error {
	s.confirmacoes++
	return s.confirmarErr
}

func TestDesbloqueioConfirmaCargaSomenteDepoisDaAutenticacao(t *testing.T) {
	key, salt, err := mycrypto.DeriveKeyBytes([]byte("teste123"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(key)
	v := vault.NewManaged()
	defer v.Close()
	data, err := v.Pack(key, salt)
	if err != nil {
		t.Fatal(err)
	}
	s := &confirmarStore{memoryStore: memoryStore{data: data}, confirmarErr: errors.New("conflito local")}
	m := NewModel(s)
	defer func() { m.Close() }()
	m.passInput.SetValue("incorreta")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if s.confirmacoes != 0 {
		t.Fatal("senha errada promoveu candidato")
	}
	m.passInput.SetValue("teste123")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if s.confirmacoes != 1 || m.vault != nil || m.state != ViewUnlock {
		t.Fatal("falha de confirmação abriu sessão")
	}
	s.confirmarErr = nil
	m.passInput.SetValue("teste123")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if s.confirmacoes != 2 || m.vault == nil || m.state != ViewList {
		t.Fatalf("candidato validado não abriu: %v", m.err)
	}
}

func TestTresErrosImpedemNovaTentativaMesmoComSenhaCorreta(t *testing.T) {
	key, salt, err := mycrypto.DeriveKeyBytes([]byte("teste123"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(key)
	v := vault.NewManaged()
	defer v.Close()
	data, err := v.Pack(key, salt)
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(&memoryStore{data: data})
	defer func() { m.Close() }()
	for i := 0; i < 3; i++ {
		m.passInput.SetValue("errada")
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(Model)
	}
	m.passInput.SetValue("teste123")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.vault != nil || m.err == nil || !strings.Contains(m.err.Error(), "aguarde") {
		t.Fatalf("espera não aplicada: %v", m.err)
	}
	if !strings.Contains(m.painelAcesso(), "Aguarde") {
		t.Fatal("contador não aparece na tela")
	}
}

func TestClipboardExpiryPreservesExternalContent(t *testing.T) {
	previousRead, previousWrite := readClipboard, writeClipboard
	t.Cleanup(func() { readClipboard, writeClipboard = previousRead, previousWrite })
	var clipboardValue string
	readClipboard = func() (string, error) { return clipboardValue, nil }
	writeClipboard = func(value string) error { clipboardValue = value; return nil }
	m := fixtureModel(t)
	f, err := vault.NewProtectedField("Senha", []byte("teste"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	m.copyField(f)
	if clipboardValue != "teste" {
		t.Fatal("copia nao decifrou campo")
	}
	next, _ := m.Update(clearClipboardMsg{hash: sha256.Sum256([]byte("teste"))})
	m = next.(Model)
	if clipboardValue != "" {
		t.Fatal("prazo nao limpou clipboard")
	}
	m.copyField(f)
	clipboardValue = "conteudo de outro aplicativo"
	m.clearClipboard()
	if clipboardValue != "conteudo de outro aplicativo" {
		t.Fatal("limpeza apagou conteudo externo")
	}
	writeClipboard = func(string) error { return errors.New("clipboard ocupado") }
	m.copyField(f)
	if m.err == nil {
		t.Fatal("falha de copia ignorada")
	}
}

type memoryStore struct {
	data []byte
	fail bool
}

func (s *memoryStore) Load(context.Context) ([]byte, error) {
	return append([]byte(nil), s.data...), nil
}
func (s *memoryStore) Save(_ context.Context, data []byte) error {
	if s.fail {
		return errors.New("falha simulada")
	}
	s.data = append([]byte(nil), data...)
	return nil
}
func (s *memoryStore) Exists(context.Context) (bool, error) { return len(s.data) > 0, nil }
func (*memoryStore) Location() string                       { return "fixture em memoria" }

func fixtureModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(&memoryStore{})
	m.vault = vault.NewManaged()
	var err error
	m.sessionKey, err = mycrypto.SealMemory(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	m.salt = make([]byte, 16)
	m.lastActivity = time.Now()
	m.state = ViewList
	m.isNewVault = false
	t.Cleanup(func() { m.Close() })
	return m
}

func TestSaveCancelAndLockClearSecrets(t *testing.T) {
	m := fixtureModel(t)
	m.initForm(vault.SecretEntry{}, false)
	m.formInputs[0].SetValue("Fixture")
	m.formInputs[3].SetValue(" a1234F ")
	old := m.formInputs[3].secret.runes
	next, _ := m.saveForm()
	m = next.(Model)
	if m.err != nil {
		t.Fatal(m.err)
	}
	for _, r := range old {
		if r != 0 {
			t.Fatal("salvar nao apagou buffer do formulario")
		}
	}
	if m.formInputs != nil || len(m.selectedEntry.Fields) > 0 || m.filteredItems[0].Fields[0].Value != "" {
		t.Fatal("interface reteve segredo")
	}
	field := m.filteredItems[0].Fields[0]
	if err := field.WithValue(func(value []byte) error {
		if string(value) != " a1234F " {
			t.Fatal("espacos da senha foram perdidos")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.initForm(vault.SecretEntry{}, false)
	m.formInputs[3].SetValue("cancelar")
	old = m.formInputs[3].secret.runes
	next, _ = m.updateForm(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	for _, r := range old {
		if r != 0 {
			t.Fatal("cancelar nao apagou buffer")
		}
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	m = next.(Model)
	if m.vault != nil || m.sessionKey != nil || m.state != ViewUnlock || m.filteredItems != nil {
		t.Fatal("bloqueio incompleto")
	}
	if err := field.WithValue(func([]byte) error { return nil }); err == nil {
		t.Fatal("bloqueio deixou handle acessivel")
	}
}

func TestEditPreservesUnchangedSecretAndOtherFields(t *testing.T) {
	m := fixtureModel(t)
	e, err := m.vault.AddEntry(vault.SecretEntry{Title: "Antes", Fields: []vault.Field{{Name: "Senha", Value: "principal", Protected: true}, {Name: "Token", Value: "extra", Protected: true}, {Name: "URL", Value: "https://example.test"}}})
	if err != nil {
		t.Fatal(err)
	}
	m.selectedEntry = e
	m.initForm(e, true)
	if m.formInputs[3].Value() != "" {
		t.Fatal("editar decifrou senha sem necessidade")
	}
	m.formInputs[0].SetValue("Depois")
	next, _ := m.saveForm()
	m = next.(Model)
	if m.err != nil {
		t.Fatal(m.err)
	}
	got, _ := m.vault.GetEntry(e.ID)
	if len(got.Fields) != 3 || got.Title != "Depois" {
		t.Fatal("edicao perdeu campos")
	}
	for index, want := range []string{"principal", "extra", "https://example.test"} {
		if err := got.Fields[index].WithValue(func(b []byte) error {
			if string(b) != want {
				t.Fatal("campo alterado")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFailedSaveCanRetryWithoutDuplicate(t *testing.T) {
	m := fixtureModel(t)
	store := m.storage.(*memoryStore)
	store.fail = true
	m.initForm(vault.SecretEntry{}, false)
	m.formInputs[0].SetValue("Retry")
	m.formInputs[3].SetValue("segredo")
	next, _ := m.saveForm()
	m = next.(Model)
	if m.err == nil || m.vault.Count() != 1 {
		t.Fatal("falha nao propagada")
	}
	store.fail = false
	next, _ = m.saveForm()
	m = next.(Model)
	if m.vault.Count() != 1 || len(store.data) == 0 {
		t.Fatal("retry duplicou entrada ou perdeu persistencia")
	}
}

func TestInactivityAndRevealTimeout(t *testing.T) {
	m := fixtureModel(t)
	m.revealed = true
	m.revealUntil = time.Now().Add(-time.Second)
	next, _ := m.Update(securityTickMsg{})
	m = next.(Model)
	if m.revealed {
		t.Fatal("revelacao nao expirou")
	}
	m.lastActivity = time.Now().Add(-6 * time.Minute)
	next, _ = m.Update(securityTickMsg{})
	m = next.(Model)
	if m.state != ViewUnlock || m.vault != nil {
		t.Fatal("inatividade nao bloqueou")
	}
}

func TestSecureInputWipesReallocationsAndSharedModels(t *testing.T) {
	i := newInput(true)
	i.Focus()
	i.SetValue("abc")
	old := i.secret.runes
	previous := i
	i, _ = i.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("🔐")})
	for _, r := range old {
		if r != 0 {
			t.Fatal("edicao reteve buffer anterior")
		}
	}
	if !bytes.Equal(i.Bytes(), []byte("abc🔐")) {
		t.Fatal("unicode alterado")
	}
	if i.Model.Value() != "" {
		t.Fatal("senha entrou no componente generico")
	}
	i.Reset()
	if previous.Value() != "" {
		t.Fatal("modelo anterior reteve senha")
	}
	i, _ = i.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !bytes.Equal(i.Bytes(), []byte(" ")) {
		t.Fatal("tecla de espaco perdida")
	}
}

func TestUnlockClearsMasterAndAcceptsLetters(t *testing.T) {
	m := NewModel(&memoryStore{})
	defer func() { m.Close() }()
	for _, r := range "ptPassword" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	if m.state != ViewUnlock || m.passInput.Value() != "ptPassword" {
		t.Fatal("atalhos capturaram senha")
	}
	old := m.passInput.secret.runes
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.err != nil || m.state != ViewList {
		t.Fatalf("unlock falhou: %v", m.err)
	}
	for _, r := range old {
		if r != 0 {
			t.Fatal("senha mestre retida")
		}
	}
	m.lock()
	m.passInput.SetValue("ptPassword")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.err != nil || m.state != ViewList {
		t.Fatalf("reabertura falhou: %v", m.err)
	}
}

func TestViewDetailNeverExposesProtectedSecret(t *testing.T) {
	m := fixtureModel(t)
	canary := "CANARY_10891089"
	e, err := m.vault.AddEntry(vault.SecretEntry{
		Title: "EntradaTeste",
		Fields: []vault.Field{
			{Name: "Usuario", Value: "teste@teste"},
			{Name: "Segredo", Value: canary, Protected: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	m.selectedEntry = e
	m.state = ViewDetail
	m.detailCursor = 1 // Cursor no campo 'Segredo'

	// 1. Verifica que a ViewDetail mascara o segredo e nao o materializa no view
	view := m.viewDetail()
	if bytes.Contains([]byte(view), []byte(canary)) {
		t.Fatal("FALHA: viewDetail expos o segredo em claro no buffer de renderizacao!")
	}
	if !bytes.Contains([]byte(view), []byte("••••••••••••••••")) {
		t.Fatal("FALHA: campo protegido nao foi mascarado na view")
	}

	// 2. Acionar 'v' lanca o comando efemero isolado e NAO injeta a senha na view regular
	next, cmd := m.updateDetail(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("esperava comando efemero ao pressionar 'v'")
	}

	viewAfterV := m.viewDetail()
	if bytes.Contains([]byte(viewAfterV), []byte(canary)) {
		t.Fatal("FALHA: viewDetail expos segredo apos pressionar 'v'!")
	}
}
