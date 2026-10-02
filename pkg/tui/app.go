package tui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"kofre/pkg/config"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/storage"
	"kofre/pkg/vault"
)

type ViewState int

// Substituídos apenas nos testes de ciclo da área de transferência.
var readClipboard = clipboard.ReadAll
var writeClipboard = clipboard.WriteAll

const (
	ViewUnlock ViewState = iota
	ViewTelegramChallenge
	ViewList
	ViewDetail
	ViewForm
	ViewConfirmDelete
	ViewPro
)

type Model struct {
	storage       storage.StorageProvider
	vault         *vault.ManagedVault
	state         ViewState
	previousState ViewState
	err           error
	notification  string
	notifTimer    *time.Timer
	isNewVault    bool

	// Chave e salt da sessao
	sessionKey *mycrypto.SealedBuffer
	salt       []byte

	// Inputs da tela de Unlock
	passInput input

	// Desafio Telegram com Timeout
	challengeID        string
	challengeCodeInput textinput.Model
	challengeExpiresAt time.Time
	challengeSeconds   int

	// Tela de Lista
	searchInput    textinput.Model
	searchFocused  bool
	showHelp       bool
	helpOffset     int
	filteredItems  []vault.SecretEntry
	cursor         int
	scrollOffset   int
	selectedCatIdx int // 0 = Todas, 1 = Senhas, 2 = Tokens, etc.

	// Tela de Detalhes
	selectedEntry  vault.SecretEntry
	detailCursor   int
	revealed       bool
	revealUntil    time.Time
	lastActivity   time.Time
	clipboardHash  [32]byte
	clipboardOwned bool

	// Tela de Formulario (Criar / Editar)
	isEditing      bool
	formInputs     []input
	formFocusIndex int
	formCategory   vault.Category

	// Confirmacao de Delete
	deleteID    string
	deleteTitle string

	width  int
	height int
}

func NewModel(store storage.StorageProvider) Model {
	mycrypto.ProtectProcess()
	ctx := context.Background()
	exists, _ := store.Exists(ctx)

	// Input de desbloqueio
	pi := newInput(true)
	pi.Placeholder = "Digite seu PIN ou senha mestre"
	pi.EchoMode = textinput.EchoPassword
	pi.EchoCharacter = '•'
	pi.Focus()

	// Input de busca
	si := textinput.New()
	si.Placeholder = "Buscar por titulo, usuario, chave ou nota..."
	si.Prompt = "🔍 "

	m := Model{
		storage:       store,
		state:         ViewUnlock,
		isNewVault:    !exists,
		passInput:     pi,
		searchInput:   si,
		searchFocused: false,
	}

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, securityTick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.adjustScroll()

	case tea.MouseMsg:
		m.lastActivity = time.Now()
		if m.showHelp {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.helpOffset = max(0, m.helpOffset-1)
			case tea.MouseButtonWheelDown:
				m.helpOffset = min(m.helpOffset+1, m.helpMaxOffset())
			}
			return m, nil
		}
		if m.state == ViewList {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				if m.cursor > 0 {
					m.cursor -= 2
					m.adjustScroll()
				}
				return m, nil
			case tea.MouseButtonWheelDown:
				if m.cursor < len(m.filteredItems)-1 {
					m.cursor += 2
					m.adjustScroll()
				}
				return m, nil
			}
		} else if m.state == ViewDetail {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				if m.detailCursor > 0 {
					m.detailCursor--
				}
				return m, nil
			case tea.MouseButtonWheelDown:
				if m.detailCursor < len(m.selectedEntry.Fields)-1 {
					m.detailCursor++
				}
				return m, nil
			}
		}

	case tea.KeyMsg:
		m.lastActivity = time.Now()
		if msg.Type == tea.KeyCtrlL && m.vault != nil {
			m.lock()
			return m, nil
		}
		// Tecla global para sair
		if msg.Type == tea.KeyCtrlC {
			m.cleanup()
			return m, tea.Quit
		}
		if m.state == ViewList && msg.Type == tea.KeyF1 {
			m.showHelp = !m.showHelp
			m.helpOffset = 0
			return m, nil
		}
		if m.showHelp {
			switch msg.String() {
			case "esc", "q":
				m.showHelp = false
			case "up", "k":
				m.helpOffset = max(0, m.helpOffset-1)
			case "down", "j":
				m.helpOffset = min(m.helpOffset+1, m.helpMaxOffset())
			}
			return m, nil
		}

		switch m.state {
		case ViewUnlock:
			return m.updateUnlock(msg)
		case ViewTelegramChallenge:
			return m.updateTelegramChallenge(msg)
		case ViewList:
			return m.updateList(msg)
		case ViewDetail:
			return m.updateDetail(msg)
		case ViewForm:
			return m.updateForm(msg)
		case ViewConfirmDelete:
			return m.updateConfirmDelete(msg)
		case ViewPro:
			return m.updatePro(msg)
		}

	case challengeTickMsg:
		if m.state == ViewTelegramChallenge {
			m.challengeSeconds--
			if m.challengeSeconds <= 0 {
				m.state = ViewUnlock
				m.err = fmt.Errorf("tempo limite do Telegram expirou (3 minutos)")
				return m, nil
			}
			return m, m.tickChallengeTimer()
		}

	case challengePollMsg:
		if m.state == ViewTelegramChallenge {
			if msg.status == "approved" {
				return m.finishTelegramUnlock()
			} else if msg.status == "rejected" {
				m.state = ViewUnlock
				m.err = fmt.Errorf("solicitação recusada no Telegram ou cofre bloqueado")
				return m, nil
			} else if msg.status == "expired" {
				m.state = ViewUnlock
				m.err = fmt.Errorf("tempo limite do desafio expirou")
				return m, nil
			}
			ep := config.GetCloudEndpoint()
			return m, m.pollChallengeStatus(m.challengeID, ep)
		}

	case ephemeralRevealDoneMsg:
		m.revealed = false
		return m, nil
	case clearNotifMsg:
		m.notification = ""
	case securityTickMsg:
		if m.revealed && time.Now().After(m.revealUntil) {
			m.revealed = false
		}
		if m.vault != nil && time.Since(m.lastActivity) >= 5*time.Minute {
			m.lock()
		}
		return m, securityTick()
	case clearClipboardMsg:
		if m.clipboardOwned && m.clipboardHash == msg.hash {
			m.clearClipboard()
		}
	}

	return m, tea.Batch(cmds...)
}

type clearNotifMsg struct{}
type securityTickMsg struct{}
type clearClipboardMsg struct{ hash [32]byte }

func securityTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return securityTickMsg{} })
}

func (m *Model) clearClipboard() {
	if !m.clipboardOwned {
		return
	}
	if value, err := readClipboard(); err == nil && sha256.Sum256([]byte(value)) == m.clipboardHash {
		_ = writeClipboard("")
	}
	m.clipboardOwned = false
	m.clipboardHash = [32]byte{}
}

func (m *Model) copyField(field vault.Field) tea.Cmd {
	err := field.WithValue(func(value []byte) error {
		if err := writeClipboard(string(value)); err != nil {
			return err
		}
		m.clipboardHash = sha256.Sum256(value)
		m.clipboardOwned = true
		return nil
	})
	if err != nil {
		m.err = err
		return nil
	}
	hash := m.clipboardHash
	return tea.Batch(m.notify("Campo copiado; limpeza em 30 segundos"), tea.Tick(30*time.Second, func(time.Time) tea.Msg { return clearClipboardMsg{hash} }))
}

func (m *Model) clearForm() {
	for i := range m.formInputs {
		m.formInputs[i].Reset()
		m.formInputs[i] = input{}
	}
	m.formInputs = nil
}

func (m *Model) lock() {
	m.showHelp = false
	if m.vault != nil && m.vault.IsDirty() {
		m.err = fmt.Errorf("cofre bloqueado; alteracoes que falharam ao gravar foram descartadas")
	}
	m.sessionKey.Close()
	m.sessionKey = nil
	if m.vault != nil {
		m.vault.Close()
		m.vault = nil
	}
	m.clearForm()
	m.selectedEntry = vault.SecretEntry{}
	m.filteredItems = nil
	m.revealed = false
	m.passInput.Reset()
	m.challengeCodeInput.Reset()
	m.challengeID = ""
	m.clearClipboard()
	m.state = ViewUnlock
	m.passInput.Focus()
}

func (m *Model) pack() (packed []byte, err error) {
	err = m.sessionKey.WithBytes(func(key []byte) error { packed, err = m.vault.Pack(key, m.salt); return err })
	return
}

// Close também permite limpar o modelo final quando o terminal encerra o programa.
func (m *Model) Close() { m.cleanup() }

type challengeTickMsg time.Time
type challengePollMsg struct {
	status string
	err    error
}

func (m *Model) notify(msg string) tea.Cmd {
	m.notification = msg
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return clearNotifMsg{}
	})
}

func (m *Model) cleanup() {
	m.lock()
	if syncer, ok := m.storage.(interface{ Flush(time.Duration) }); ok {
		syncer.Flush(3 * time.Second)
	}
}

// ======================== TELA UNLOCK ========================

func (m Model) updateUnlock(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.Type {
	case tea.KeyEnter:
		secretBuffer := m.passInput.Bytes()
		defer mycrypto.ZeroBytes(secretBuffer)
		secret := bytes.TrimSpace(secretBuffer)
		m.passInput.Reset()
		if len(secret) == 0 {
			m.err = fmt.Errorf("informe uma chave ou PIN")
			return m, nil
		}

		ctx := context.Background()
		if m.isNewVault {
			// Criando cofre inicial
			key, salt, err := mycrypto.DeriveKeyBytes(secret, nil)
			if err != nil {
				m.err = err
				return m, nil
			}
			defer mycrypto.ZeroBytes(key)
			m.sessionKey, err = mycrypto.SealMemory(key)
			if err != nil {
				m.err = err
				return m, nil
			}
			m.salt = salt
			m.vault = vault.NewManaged()

			// Salva inicialmente
			packed, err := m.pack()
			if err != nil {
				m.lock()
				m.err = err
				return m, nil
			}
			if err := m.storage.Save(ctx, packed); err != nil {
				m.lock()
				m.err = err
				return m, nil
			}

			m.state = ViewList
			m.isNewVault = false
			m.lastActivity = time.Now()
			m.refreshList()
			return m, m.notify("✓ Cofre inicializado com sucesso!")
		}

		// Carregando cofre existente
		rawData, err := m.storage.Load(ctx)
		if err != nil {
			m.err = fmt.Errorf("falha ao ler cofre: %w", err)
			return m, nil
		}

		salt, encryptedPayload, err := vault.UnpackHeader(rawData)
		if err != nil {
			m.err = err
			return m, nil
		}

		key, _, err := mycrypto.DeriveKeyBytes(secret, salt)
		if err != nil {
			m.err = err
			return m, nil
		}

		v, err := vault.DecryptAndLoad(encryptedPayload, key, salt)
		defer mycrypto.ZeroBytes(key)
		if err != nil {
			m.err = fmt.Errorf("chave/PIN incorreto")
			return m, nil
		}

		m.sessionKey, err = mycrypto.SealMemory(key)
		if err != nil {
			v.Close()
			m.err = err
			return m, nil
		}
		m.salt = salt
		m.vault = v
		m.state = ViewList
		m.lastActivity = time.Now()
		m.err = nil
		m.refreshList()
		return m, m.notify("✓ Cofre desbloqueado em memoria RAM")

	case tea.KeyCtrlP:
		m.previousState = m.state
		m.state = ViewPro
		return m, nil
	case tea.KeyCtrlT:
		return m.startTelegramChallenge()

	case tea.KeyEsc:
		return m, tea.Quit
	}

	m.passInput, cmd = m.passInput.Update(msg)
	return m, cmd
}

// ======================== TELA DESAFIO TELEGRAM ========================

func (m *Model) startTelegramChallenge() (tea.Model, tea.Cmd) {
	cfg, _ := config.LoadConfig()
	if cfg == nil || !cfg.CloudEnabled || cfg.KofreToken == "" {
		m.err = fmt.Errorf("Kofre Cloud / Telegram não vinculado. Execute 'kofre telegram' primeiro.")
		return *m, nil
	}

	endpoint := config.GetCloudEndpoint()

	url := fmt.Sprintf("%s/v1/auth/telegram-challenge", strings.TrimRight(endpoint, "/"))
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		m.err = err
		return *m, nil
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.KofreToken))

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		m.err = fmt.Errorf("falha ao contatar Kofre Cloud: %w", err)
		return *m, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errRes struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errRes)
		if errRes.Error != "" {
			m.err = fmt.Errorf("%s", errRes.Error)
		} else {
			m.err = fmt.Errorf("erro ao gerar desafio (HTTP %d)", resp.StatusCode)
		}
		return *m, nil
	}

	var res struct {
		ChallengeID    string `json:"challenge_id"`
		TimeoutSeconds int    `json:"timeout_seconds"`
		ExpiresAt      string `json:"expires_at"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&res)

	m.challengeID = res.ChallengeID
	m.challengeSeconds = res.TimeoutSeconds
	m.challengeExpiresAt, _ = time.Parse(time.RFC3339, res.ExpiresAt)
	m.state = ViewTelegramChallenge
	m.err = nil

	ci := textinput.New()
	ci.Placeholder = "000000"
	ci.Prompt = "Código OTP: "
	ci.Focus()
	m.challengeCodeInput = ci

	return *m, tea.Batch(
		textinput.Blink,
		m.tickChallengeTimer(),
		m.pollChallengeStatus(m.challengeID, endpoint),
	)
}

func (m Model) tickChallengeTimer() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return challengeTickMsg(t)
	})
}

func (m Model) pollChallengeStatus(id, endpoint string) tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(t time.Time) tea.Msg {
		url := fmt.Sprintf("%s/v1/auth/telegram-challenge/status?id=%s", strings.TrimRight(endpoint, "/"), id)
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			return challengePollMsg{err: err}
		}
		defer resp.Body.Close()

		var res struct {
			Status string `json:"status"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&res)
		return challengePollMsg{status: res.Status}
	})
}

func (m Model) updateTelegramChallenge(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.state = ViewUnlock
		m.err = nil
		return m, nil

	case tea.KeyEnter:
		code := strings.TrimSpace(m.challengeCodeInput.Value())
		if code == "" {
			return m, nil
		}

		ep := config.GetCloudEndpoint()

		url := fmt.Sprintf("%s/v1/auth/telegram-challenge/verify", strings.TrimRight(ep, "/"))
		body, _ := json.Marshal(map[string]string{
			"challenge_id": m.challengeID,
			"code":         code,
		})
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			m.err = fmt.Errorf("falha ao verificar código: %w", err)
			return m, nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			var errRes struct {
				Error string `json:"error"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&errRes)
			if errRes.Error != "" {
				m.err = fmt.Errorf("%s", errRes.Error)
			} else {
				m.err = fmt.Errorf("código incorreto ou expirado")
			}
			return m, nil
		}

		return m.finishTelegramUnlock()
	}

	var cmd tea.Cmd
	m.challengeCodeInput, cmd = m.challengeCodeInput.Update(msg)
	return m, cmd
}

func (m Model) finishTelegramUnlock() (tea.Model, tea.Cmd) {
	m.state = ViewUnlock
	m.isNewVault = false
	m.err = nil
	m.passInput.Focus()
	return m, m.notify("✓ 2FA aprovado via Telegram! Digite sua senha mestre para abrir na RAM.")
}

// ======================== TELA LISTA ========================

func (m Model) visibleListHeight() int {
	h := m.height
	if h <= 0 {
		return 10
	}
	// Reserva o espaço real do rodapé, que cresce em janelas estreitas.
	overhead := 12 + lipgloss.Height(m.listFooter())
	if m.notification != "" || m.err != nil {
		overhead += 2
	}
	avail := h - overhead
	if avail < 1 {
		return 1
	}
	return avail
}

func (m *Model) adjustScroll() {
	total := len(m.filteredItems)
	if total == 0 {
		m.cursor = 0
		m.scrollOffset = 0
		return
	}

	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= total {
		m.cursor = total - 1
	}

	maxVisible := m.visibleListHeight()

	// Se o cursor estiver antes da janela visível, rola para cima
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}

	// Se o cursor estiver além da janela visível, rola para baixo
	if m.cursor >= m.scrollOffset+maxVisible {
		m.scrollOffset = m.cursor - maxVisible + 1
	}

	// Garante que o scrollOffset nunca ultrapasse os limites
	if total <= maxVisible {
		m.scrollOffset = 0
	} else if m.scrollOffset > total-maxVisible {
		m.scrollOffset = total - maxVisible
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m *Model) refreshList() {
	if m.vault == nil {
		m.filteredItems = nil
		m.cursor = 0
		m.scrollOffset = 0
		return
	}

	cat := m.currentCategoryFilter()
	m.filteredItems = m.vault.Search(m.searchInput.Value(), cat)
	m.adjustScroll()
}

func (m Model) currentCategoryFilter() vault.Category {
	if m.selectedCatIdx == 0 {
		return ""
	}
	idx := m.selectedCatIdx - 1
	if idx >= 0 && idx < len(vault.AllCategories) {
		return vault.AllCategories[idx]
	}
	return ""
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.searchFocused {
		switch msg.Type {
		case tea.KeyEsc, tea.KeyEnter:
			m.searchFocused = false
			m.searchInput.Blur()
			return m, nil
		case tea.KeyDown:
			m.searchFocused = false
			m.searchInput.Blur()
			if m.cursor < len(m.filteredItems)-1 {
				m.cursor++
				m.adjustScroll()
			}
			return m, nil
		default:
			m.searchInput, cmd = m.searchInput.Update(msg)
			m.refreshList()
			return m, cmd
		}
	}

	// 1. Checagem por tipo de tecla (navegação por teclado em qualquer terminal)
	switch msg.Type {
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
			m.adjustScroll()
		}
		return m, nil

	case tea.KeyDown:
		if m.cursor < len(m.filteredItems)-1 {
			m.cursor++
			m.adjustScroll()
		}
		return m, nil

	case tea.KeyPgUp:
		pageSize := m.visibleListHeight()
		m.cursor -= pageSize
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.adjustScroll()
		return m, nil

	case tea.KeyPgDown:
		pageSize := m.visibleListHeight()
		m.cursor += pageSize
		if m.cursor >= len(m.filteredItems) {
			m.cursor = max(0, len(m.filteredItems)-1)
		}
		m.adjustScroll()
		return m, nil

	case tea.KeyHome:
		m.cursor = 0
		m.adjustScroll()
		return m, nil

	case tea.KeyEnd:
		m.cursor = max(0, len(m.filteredItems)-1)
		m.adjustScroll()
		return m, nil
	case tea.KeyShiftTab:
		totalCats := len(vault.AllCategories) + 1
		m.selectedCatIdx = (m.selectedCatIdx - 1 + totalCats) % totalCats
		m.cursor = 0
		m.scrollOffset = 0
		m.refreshList()
		return m, nil
	}

	// 2. Checagem por texto/caractere (inclui alias vim j/k, pgup/pgdn, etc.)
	switch msg.String() {
	case "q":
		m.cleanup()
		return m, tea.Quit

	case "/":
		m.searchFocused = true
		m.searchInput.Focus()
		return m, textinput.Blink

	case "tab":
		m.selectedCatIdx = (m.selectedCatIdx + 1) % (len(vault.AllCategories) + 1)
		m.cursor = 0
		m.scrollOffset = 0
		m.refreshList()
		return m, nil

	case "shift+tab", "backtab":
		totalCats := len(vault.AllCategories) + 1
		m.selectedCatIdx = (m.selectedCatIdx - 1 + totalCats) % totalCats
		m.cursor = 0
		m.scrollOffset = 0
		m.refreshList()
		return m, nil

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.adjustScroll()
		}
		return m, nil

	case "down", "j":
		if m.cursor < len(m.filteredItems)-1 {
			m.cursor++
			m.adjustScroll()
		}
		return m, nil

	case "pgup", "pageup", "b":
		pageSize := m.visibleListHeight()
		m.cursor -= pageSize
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.adjustScroll()
		return m, nil

	case "pgdown", "pagedown", "f":
		pageSize := m.visibleListHeight()
		m.cursor += pageSize
		if m.cursor >= len(m.filteredItems) {
			m.cursor = max(0, len(m.filteredItems)-1)
		}
		m.adjustScroll()
		return m, nil

	case "home", "g":
		m.cursor = 0
		m.adjustScroll()
		return m, nil

	case "end", "G":
		m.cursor = max(0, len(m.filteredItems)-1)
		m.adjustScroll()
		return m, nil

	case "enter":
		if len(m.filteredItems) > 0 {
			m.selectedEntry = m.filteredItems[m.cursor]
			m.detailCursor = 0
			m.revealed = false
			m.state = ViewDetail
		}
		return m, nil

	case "c":
		// Copia rapida da senha/token principal
		if len(m.filteredItems) > 0 {
			entry := m.filteredItems[m.cursor]
			index := 0
			for i, f := range entry.Fields {
				if f.Protected {
					index = i
					break
				}
			}
			if len(entry.Fields) > 0 {
				cmd := m.copyField(entry.Fields[index])
				return m, cmd
			}
		}

	case "n":
		m.initForm(vault.SecretEntry{}, false)
		m.state = ViewForm
		return m, nil

	case "d":
		if len(m.filteredItems) > 0 {
			entry := m.filteredItems[m.cursor]
			m.deleteID = entry.ID
			m.deleteTitle = entry.Title
			m.state = ViewConfirmDelete
		}
		return m, nil

	case "p", "P":
		m.previousState = m.state
		m.state = ViewPro
		return m, nil
	}

	return m, nil
}

// ======================== TELA DETALHES ========================

func (m Model) updateDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "b", "q":
		m.state = ViewList
		m.revealed = false
		m.selectedEntry = vault.SecretEntry{}
		m.refreshList()
		return m, nil

	case "v", " ":
		if len(m.selectedEntry.Fields) > 0 && m.detailCursor < len(m.selectedEntry.Fields) {
			field := m.selectedEntry.Fields[m.detailCursor]
			if field.Protected {
				return m, m.revealFieldEphemeral(field, m.selectedEntry.Title)
			}
		}
		return m, nil

	case "up", "k":
		if m.detailCursor > 0 {
			m.detailCursor--
		}
		return m, nil

	case "down", "j":
		totalFields := len(m.selectedEntry.Fields)
		if m.detailCursor < totalFields-1 {
			m.detailCursor++
		}
		return m, nil

	case "enter", "c":
		if len(m.selectedEntry.Fields) > 0 && m.detailCursor < len(m.selectedEntry.Fields) {
			field := m.selectedEntry.Fields[m.detailCursor]
			cmd := m.copyField(field)
			return m, cmd
		}

	case "e":
		m.revealed = false
		m.initForm(m.selectedEntry, true)
		m.state = ViewForm
		return m, nil

	case "d":
		m.revealed = false
		m.deleteID = m.selectedEntry.ID
		m.deleteTitle = m.selectedEntry.Title
		m.state = ViewConfirmDelete
		return m, nil
	}

	return m, nil
}

// ======================== TELA FORMULARIO (ADD/EDIT) ========================

func (m *Model) initForm(entry vault.SecretEntry, isEdit bool) {
	m.clearForm()
	m.isEditing = isEdit
	m.formFocusIndex = 0

	// 0: Titulo
	// 1: Categoria (password, token, certificate, ssh_key, auth, note)
	// 2: Usuario / Identificador
	// 3: Segredo / Senha / Token
	// 4: Notas
	m.formInputs = make([]input, 5)

	m.formInputs[0] = newInput(false)
	m.formInputs[0].Placeholder = "Ex: AWS Producao, Banco Master, GitHub PAT"
	m.formInputs[0].Prompt = "Titulo: "
	m.formInputs[0].SetValue(entry.Title)
	m.formInputs[0].Focus()

	m.formInputs[1] = newInput(false)
	m.formInputs[1].Placeholder = "password, token, certificate, ssh_key, auth, note"
	m.formInputs[1].Prompt = "Categoria: "
	catVal := string(entry.Category)
	if catVal == "" {
		catVal = "password"
	}
	m.formInputs[1].SetValue(catVal)

	userVal := ""
	passVal := ""
	for _, f := range entry.Fields {
		if strings.EqualFold(f.Name, "usuario") || strings.EqualFold(f.Name, "username") || strings.EqualFold(f.Name, "login") {
			userVal = f.Value
		} else if f.Protected || strings.EqualFold(f.Name, "senha") || strings.EqualFold(f.Name, "token") || strings.EqualFold(f.Name, "secret") {
			passVal = f.Value
		}
	}

	m.formInputs[2] = newInput(false)
	m.formInputs[2].Placeholder = "admin / root / seu@email.com"
	m.formInputs[2].Prompt = "Usuario/ID: "
	m.formInputs[2].SetValue(userVal)

	m.formInputs[3] = newInput(true)
	m.formInputs[3].Placeholder = "•••••••• (ou aperte 'g' para gerar)"
	m.formInputs[3].Prompt = "Segredo/Senha: "
	m.formInputs[3].EchoMode = textinput.EchoPassword
	m.formInputs[3].EchoCharacter = '•'
	m.formInputs[3].SetValue(passVal)
	if isEdit {
		m.formInputs[3].Placeholder = "Deixe vazio para manter o segredo atual"
	}

	m.formInputs[4] = newInput(false)
	m.formInputs[4].Placeholder = "Detalhes, URLs, portas ou lembretes"
	m.formInputs[4].Prompt = "Notas: "
	m.formInputs[4].SetValue(entry.Notes)
}

func (m Model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg.Type {
	case tea.KeyEsc:
		m.clearForm()
		m.selectedEntry = vault.SecretEntry{}
		m.revealed = false
		m.state = ViewList
		return m, nil

	case tea.KeyTab, tea.KeyDown:
		m.formFocusIndex = (m.formFocusIndex + 1) % len(m.formInputs)
		for i := range m.formInputs {
			if i == m.formFocusIndex {
				m.formInputs[i].Focus()
			} else {
				m.formInputs[i].Blur()
			}
		}
		return m, textinput.Blink

	case tea.KeyShiftTab, tea.KeyUp:
		m.formFocusIndex = (m.formFocusIndex - 1 + len(m.formInputs)) % len(m.formInputs)
		for i := range m.formInputs {
			if i == m.formFocusIndex {
				m.formInputs[i].Focus()
			} else {
				m.formInputs[i].Blur()
			}
		}
		return m, textinput.Blink

	case tea.KeyCtrlS, tea.KeyEnter:
		// Se for no ultimo campo ou ctrl+s, salva
		if msg.Type == tea.KeyCtrlS || m.formFocusIndex == len(m.formInputs)-1 {
			return m.saveForm()
		}
		// Caso contrario avanca pro proximo campo
		m.formFocusIndex = (m.formFocusIndex + 1) % len(m.formInputs)
		for i := range m.formInputs {
			if i == m.formFocusIndex {
				m.formInputs[i].Focus()
			} else {
				m.formInputs[i].Blur()
			}
		}
		return m, textinput.Blink
	}

	// Atalho 'ctrl+g' no campo de senha para gerar chave forte
	if msg.Type == tea.KeyCtrlG && m.formFocusIndex == 3 {
		pwd := mycrypto.GenerateSecurePasswordBytes(24)
		m.formInputs[3].SetBytes(pwd)
		mycrypto.ZeroBytes(pwd)
		return m, m.notify("🔑 Senha forte de 24 caracteres gerada!")
	}

	m.formInputs[m.formFocusIndex], _ = m.formInputs[m.formFocusIndex].Update(msg)
	return m, tea.Batch(cmds...)
}

func (m Model) saveForm() (tea.Model, tea.Cmd) {
	title := strings.TrimSpace(m.formInputs[0].Value())
	if title == "" {
		m.err = fmt.Errorf("titulo e obrigatorio")
		return m, nil
	}

	cat := vault.Category(strings.TrimSpace(m.formInputs[1].Value()))
	if cat == "" {
		cat = vault.CategoryPassword
	}

	userVal := strings.TrimSpace(m.formInputs[2].Value())
	secretVal := m.formInputs[3].Bytes()
	defer mycrypto.ZeroBytes(secretVal)
	var secretField vault.Field
	if len(secretVal) > 0 {
		var err error
		secretField, err = vault.NewProtectedField("Segredo", secretVal)
		if err != nil {
			m.err = err
			return m, nil
		}
		defer secretField.Close()
	}
	notesVal := strings.TrimSpace(m.formInputs[4].Value())

	fields := make([]vault.Field, 0)
	if userVal != "" {
		fields = append(fields, vault.Field{Name: "Usuario", Value: userVal, Protected: false})
	}
	if len(secretVal) > 0 {
		fields = append(fields, secretField)
	}

	if m.isEditing {
		entry, err := m.vault.GetEntry(m.selectedEntry.ID)
		if err != nil {
			m.err = err
			return m, nil
		}
		entry.Title, entry.Category, entry.Notes = title, cat, notesVal
		userUpdated, secretUpdated := false, false
		for i := range entry.Fields {
			f := &entry.Fields[i]
			if !f.Protected && !userUpdated && (strings.EqualFold(f.Name, "usuario") || strings.EqualFold(f.Name, "username") || strings.EqualFold(f.Name, "login")) {
				f.Value = userVal
				userUpdated = true
			}
			if f.Protected && !secretUpdated {
				if len(secretVal) > 0 {
					name := f.Name
					*f = secretField
					f.Name = name
				}
				secretUpdated = true
			}
		}
		if !userUpdated && userVal != "" {
			entry.Fields = append(entry.Fields, vault.Field{Name: "Usuario", Value: userVal})
		}
		if !secretUpdated && len(secretVal) > 0 {
			entry.Fields = append(entry.Fields, secretField)
		}
		if err := m.vault.UpdateEntry(entry); err != nil {
			m.err = err
			return m, nil
		}
	} else {
		newEntry := vault.SecretEntry{
			Title:    title,
			Category: cat,
			Fields:   fields,
			Notes:    notesVal,
		}
		entry, err := m.vault.AddEntry(newEntry)
		if err != nil {
			m.err = err
			return m, nil
		}
		m.selectedEntry = entry
		m.isEditing = true // Em falha de armazenamento, tentar novamente não duplica a entrada.
	}
	// Depois de selar, o formulário não precisa manter a senha digitada.
	m.formInputs[3].Reset()
	m.formInputs[3].EchoMode = textinput.EchoPassword
	m.formInputs[3].Placeholder = "Deixe vazio para manter o segredo atual"

	// Persiste o cofre criptografado
	ctx := context.Background()
	packed, err := m.pack()
	if err != nil {
		m.err = err
		return m, nil
	}

	if err := m.storage.Save(ctx, packed); err != nil {
		m.err = err
		return m, nil
	}

	m.vault.MarkClean()
	m.err = nil
	m.clearForm()
	m.selectedEntry = vault.SecretEntry{}
	m.revealed = false
	m.state = ViewList
	m.refreshList()
	return m, m.notify("💾 Credencial salva e criptografada com sucesso!")
}

// ======================== TELA CONFIRM DELETE ========================

func (m Model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "s", "Y", "S":
		if err := m.vault.DeleteEntry(m.deleteID); err != nil && err != vault.ErrNotFound {
			m.err = err
			return m, nil
		}
		ctx := context.Background()
		packed, err := m.pack()
		if err != nil {
			m.err = err
			return m, nil
		}
		if err := m.storage.Save(ctx, packed); err != nil {
			m.err = err
			return m, nil
		}
		m.vault.MarkClean()
		m.err = nil
		m.selectedEntry = vault.SecretEntry{}
		m.state = ViewList
		m.refreshList()
		return m, m.notify(fmt.Sprintf("🗑️ '%s' excluida com sucesso", m.deleteTitle))

	case "n", "N", "esc", "q":
		m.state = ViewList
		return m, nil
	}

	return m, nil
}

// ======================== TELA PLANO PRO ========================

func isProPlan() bool {
	cfg, err := config.LoadConfig()
	if err != nil || cfg == nil {
		return false
	}
	return cfg.CloudEnabled && cfg.KofreToken != ""
}

func (m Model) updatePro(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		if m.previousState == ViewUnlock {
			m.state = ViewUnlock
		} else {
			m.state = ViewList
			m.refreshList()
		}
		return m, nil

	case tea.KeyEnter:
		if isProPlan() {
			if m.previousState == ViewUnlock {
				m.state = ViewUnlock
			} else {
				m.state = ViewList
				m.refreshList()
			}
			return m, nil
		}
		return m.activateProPlan()
	}

	return m, nil
}

func (m Model) activateProPlan() (tea.Model, tea.Cmd) {
	cfg, _ := config.LoadConfig()
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	endpoint := config.GetCloudEndpoint()

	provisionURL := fmt.Sprintf("%s/v1/auth/provision", strings.TrimRight(endpoint, "/"))
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(provisionURL, "application/json", nil)
	if err != nil {
		m.err = fmt.Errorf("falha ao ativar Pro: %w", err)
		return m, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		m.err = fmt.Errorf("erro do servidor: %s", string(body))
		return m, nil
	}

	var res struct {
		Token   string `json:"token"`
		Plan    string `json:"plan"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		m.err = err
		return m, nil
	}

	cfg.Mode = "kofre_cloud"
	cfg.CloudEnabled = true
	cfg.KofreToken = res.Token
	cfg.CloudEndpoint = endpoint
	_ = config.SaveConfig(cfg)

	// Embrulha o storage atual em SyncStorage
	cloudStore := storage.NewKofreCloudStorage(endpoint, res.Token)
	m.storage = storage.NewSyncStorage(m.storage, cloudStore)

	// Sincroniza cofre para a nuvem se estiver aberto em memória
	if m.vault != nil && m.sessionKey != nil && len(m.salt) > 0 {
		ctx := context.Background()
		if packed, err := m.pack(); err == nil && len(packed) >= 60 {
			_ = m.storage.Save(ctx, packed)
		}
	}

	if m.previousState == ViewUnlock {
		m.state = ViewUnlock
	} else {
		m.state = ViewList
		m.refreshList()
	}
	m.err = nil
	return m, m.notify("🎉 Plano Cloud Pro ativado com sucesso! Nuvem Zero-Knowledge ativa.")
}

// ======================== VIEWS (RENDERIZACAO) ========================

func (m Model) View() string {
	if m.showHelp {
		return m.viewHelp()
	}
	var s strings.Builder

	// Header padrao
	s.WriteString(m.renderHeader())
	s.WriteString("\n")

	// Notificacao / Alerta
	if m.notification != "" {
		s.WriteString(successStyle.Render(m.notification) + "\n\n")
	} else if m.err != nil {
		s.WriteString(dangerStyle.Render("Erro: "+m.err.Error()) + "\n\n")
	}

	switch m.state {
	case ViewUnlock:
		s.WriteString(m.viewUnlock())
	case ViewTelegramChallenge:
		s.WriteString(m.viewTelegramChallenge())
	case ViewList:
		s.WriteString(m.viewList())
	case ViewDetail:
		s.WriteString(m.viewDetail())
	case ViewForm:
		s.WriteString(m.viewForm())
	case ViewConfirmDelete:
		s.WriteString(m.viewConfirmDelete())
	case ViewPro:
		s.WriteString(m.viewPro())
	}

	return s.String()
}

func (m Model) renderHeader() string {
	isUnlocked := m.state != ViewUnlock && m.state != ViewTelegramChallenge && m.state != ViewPro

	lockText := "🔒 TRANCADO"
	if isUnlocked {
		lockText = "🔓 DESBLOQUEADO (RAM)"
	}

	title := titleStyle.Render(" Kofre ")
	status := statusBadgeStyle.Render(lockText)

	planBadge := dimStyle.Render("[FREE]")
	if isProPlan() {
		planBadge = badgeToken.Render("★ PRO")
	}

	header := fmt.Sprintf("%s  %s  %s", title, planBadge, status)
	if isUnlocked && m.vault != nil {
		countBadge := badgeSSH.Render(fmt.Sprintf("%d segredos", m.vault.Count()))
		header = fmt.Sprintf("%s  %s", header, countBadge)
	}

	return headerBoxStyle.Render(header)
}

func (m Model) viewUnlock() string {
	var b strings.Builder

	if isProPlan() {
		b.WriteString(badgeToken.Render("★ KOFRE CLOUD PRO ATIVO (Zero-Knowledge E2EE)") + "\n\n")
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("★ Plano FREE (Offline / Local) • Pressione [Ctrl+P] para ativar Plano Cloud Pro") + "\n\n")
	}

	if m.isNewVault {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorSuccess).Render("★ Inicializacao de Novo Cofre\n"))
		b.WriteString("Defina seu PIN ou senha mestre. Essa chave gerara a criptografia AES-256 do cofre.\n\n")
	} else {
		b.WriteString(lipgloss.NewStyle().Bold(true).Render("Cofre Criptografado Encontrado\n"))
		b.WriteString("Insira sua chave de acesso para carregar os segredos na memoria RAM:\n\n")
	}

	b.WriteString(m.passInput.View() + "\n\n")
	b.WriteString(helpStyle.Render("[Enter] Confirmar Senha  •  [Ctrl+P] Plano Pro  •  [Ctrl+T] Desbloquear com Telegram (com Timeout)  •  [Esc] Sair"))

	return boxStyle.Render(b.String())
}

func (m Model) viewTelegramChallenge() string {
	var b strings.Builder

	mins := m.challengeSeconds / 60
	secs := m.challengeSeconds % 60
	timeStr := fmt.Sprintf("%02d:%02d", mins, secs)

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render("📱 Autenticação / Recuperação via Telegram\n"))
	b.WriteString(fmt.Sprintf("Enviamos uma solicitação para o seu bot @%s no Telegram.\n\n", config.GetTelegramBot()))
	b.WriteString("Opção 1: Toque em ")
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorSuccess).Render("[ ✅ Autorizar Desbloqueio ]"))
	b.WriteString(" no seu celular.\n")
	b.WriteString("Opção 2: Digite o código OTP de 6 dígitos enviado no Telegram:\n\n")

	b.WriteString(m.challengeCodeInput.View() + "\n\n")

	timerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
	b.WriteString(fmt.Sprintf("⏱️ Tempo restante: %s\n\n", timerStyle.Render(timeStr)))
	b.WriteString(helpStyle.Render("[Enter] Validar Código  •  [Esc] Voltar"))

	return boxStyle.Render(b.String())
}

func (m Model) viewList() string {
	var b strings.Builder

	// Categorias filtro
	catNames := []string{"[ Todas ]", "Senhas", "Tokens", "Certificados", "SSH", "Auth/2FA", "Notas"}
	var catTabs []string
	for i, name := range catNames {
		if i == m.selectedCatIdx {
			catTabs = append(catTabs, lipgloss.NewStyle().Bold(true).Underline(true).Foreground(colorAccent).Render(name))
		} else {
			catTabs = append(catTabs, dimStyle.Render(name))
		}
	}
	b.WriteString(strings.Join(catTabs, "  ") + "\n\n")

	// Barra de busca
	b.WriteString(searchStyle.Render(m.searchInput.View()) + "\n")

	// Lista de segredos
	total := len(m.filteredItems)
	if total == 0 {
		message := "Nenhuma credencial corresponde aos filtros."
		if strings.TrimSpace(m.searchInput.Value()) != "" {
			message = "Nenhuma credencial corresponde à busca."
		} else if m.vault != nil && m.vault.Count() == 0 {
			message = "Cofre vazio. Pressione 'n' para adicionar a primeira credencial."
		}
		b.WriteString("\n" + dimStyle.Render(wrapHelp([]string{message}, m.helpWidth())) + "\n\n")
	} else {
		maxVisible := m.visibleListHeight()
		start := m.scrollOffset
		if start < 0 {
			start = 0
		}
		if start >= total {
			start = max(0, total-1)
		}
		end := start + maxVisible
		if end > total {
			end = total
		}

		// Indicador de itens acima
		if start > 0 {
			b.WriteString(dimStyle.Render(fmt.Sprintf("    ▲ %d item(ns) acima...", start)) + "\n")
		} else {
			b.WriteString("\n")
		}

		for i := start; i < end; i++ {
			item := m.filteredItems[i]
			badge := renderCategoryBadge(string(item.Category))
			title := item.Title
			loginInfo := ""
			for _, f := range item.Fields {
				if !f.Protected {
					loginInfo = " (" + f.Value + ")"
					break
				}
			}

			line := fmt.Sprintf(" %s  %-26s %s", badge, title, dimStyle.Render(loginInfo))

			if i == m.cursor {
				b.WriteString(selectedItemStyle.Render("▶ "+line) + "\n")
			} else {
				b.WriteString(normalItemStyle.Render("  "+line) + "\n")
			}
		}

		// Indicador de itens abaixo
		remaining := total - end
		if remaining > 0 {
			b.WriteString(dimStyle.Render(fmt.Sprintf("    ▼ %d item(ns) abaixo...", remaining)) + "\n")
		} else {
			b.WriteString("\n")
		}

		// Informação de posição
		b.WriteString(dimStyle.Render(fmt.Sprintf("  [Item %d de %d]", m.cursor+1, total)) + "\n")
	}

	b.WriteString(m.listFooter())

	return b.String()
}

func (m Model) viewDetail() string {
	var b strings.Builder

	entry := m.selectedEntry
	badge := renderCategoryBadge(string(entry.Category))

	b.WriteString(fmt.Sprintf("%s  %s\n", badge, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(entry.Title)))
	b.WriteString(dimStyle.Render(fmt.Sprintf("ID: %s  •  Versao: %d  •  Atualizado em: %s\n\n",
		entry.ID[:8], entry.Version, entry.UpdatedAt.Format("02/01/2006 15:04"))))

	b.WriteString(lipgloss.NewStyle().Underline(true).Render("Campos e Credenciais:") + "\n")

	if len(entry.Fields) == 0 {
		b.WriteString(dimStyle.Render("  Nenhum campo cadastrado.") + "\n")
	} else {
		for i, f := range entry.Fields {
			displayVal := f.Value
			if f.Protected {
				displayVal = "••••••••••••••••"
			}

			prefix := "  "
			if i == m.detailCursor {
				prefix = "▶ "
			}

			line := fmt.Sprintf("%s%-14s : %s", prefix, f.Name, displayVal)
			if i == m.detailCursor {
				b.WriteString(selectedItemStyle.Render(line) + "\n")
			} else {
				b.WriteString(normalItemStyle.Render(line) + "\n")
			}
		}
	}

	if entry.Notes != "" {
		b.WriteString("\n" + lipgloss.NewStyle().Underline(true).Render("Notas:") + "\n")
		b.WriteString("  " + entry.Notes + "\n")
	}

	b.WriteString(helpStyle.Render("\n[↑/↓] Selecionar Campo • [Enter/c] Copiar Campo • [v] Revelar/Ocultar • [e] Editar • [Esc] Voltar"))

	return boxStyle.Render(b.String())
}

func (m Model) viewForm() string {
	var b strings.Builder

	titleText := "Cadastrar Nova Credencial"
	if m.isEditing {
		titleText = "Editar Credencial: " + m.selectedEntry.Title
	}

	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render(titleText) + "\n\n")

	for i := range m.formInputs {
		b.WriteString(m.formInputs[i].View() + "\n")
		if i == 3 {
			b.WriteString(dimStyle.Render("  (Dica: aperte Ctrl+G para gerar senha forte)") + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(helpStyle.Render("[Tab/Shift+Tab] Alternar Campos • [Ctrl+G] Gerar Senha • [Ctrl+S ou Enter no fim] Salvar • [Esc] Cancelar"))

	return boxStyle.Render(b.String())
}

func (m Model) viewConfirmDelete() string {
	var b strings.Builder

	b.WriteString(dangerStyle.Render("⚠️  Confirmar Exclusao\n\n"))
	b.WriteString(fmt.Sprintf("Deseja realmente apagar o segredo '%s'?\nEsta acao nao pode ser desfeita.\n\n", m.deleteTitle))
	b.WriteString(helpStyle.Render("[Y / S] Sim, excluir  •  [N / Esc] Cancelar"))

	return boxStyle.Render(b.String())
}

func (m Model) viewPro() string {
	var b strings.Builder

	if isProPlan() {
		cfg, _ := config.LoadConfig()
		tokenDisplay := "kfr_pro_..."
		if cfg != nil && len(cfg.KofreToken) > 16 {
			tokenDisplay = cfg.KofreToken[:16] + "..."
		}

		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorSuccess).Render("╔══════════════════════════════════════════════════════════════╗\n"))
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorSuccess).Render("║  🌟 Kofre Cloud Pro — Assinatura Ativa                       ║\n"))
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorSuccess).Render("╚══════════════════════════════════════════════════════════════╝\n\n"))

		b.WriteString("Status:           " + lipgloss.NewStyle().Bold(true).Foreground(colorSuccess).Render("ATIVO (PILOTO)") + "\n")
		b.WriteString(fmt.Sprintf("Licença:          %s\n", tokenDisplay))
		b.WriteString("Sincronização:    Nuvem S3 Criptografada (Zero-Knowledge E2EE)\n")
		b.WriteString(fmt.Sprintf("Bot Telegram:     @%s (Alertas e Desbloqueio com Timeout)\n\n", config.GetTelegramBot()))
		b.WriteString("Seu cofre está protegido e sincronizado continuamente com a nuvem.\n\n")
		b.WriteString(helpStyle.Render("[Enter / Esc] Voltar"))
	} else {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render("╔══════════════════════════════════════════════════════════════╗\n"))
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render("║  🌟 Upgrade para Kofre Cloud Pro                             ║\n"))
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render("╚══════════════════════════════════════════════════════════════╝\n\n"))

		b.WriteString("Desbloqueie o poder máximo do seu cofre:\n\n")
		b.WriteString("  ✓ Sincronização em nuvem Zero-Knowledge (criptografado no cliente)\n")
		b.WriteString("  ✓ Acesso contínuo e sincronizado entre seus múltiplos computadores\n")
		b.WriteString(fmt.Sprintf("  ✓ Desbloqueio e recuperação remota via Telegram com Timeout (@%s)\n", config.GetTelegramBot()))
		b.WriteString("  ✓ Botão de Pânico no Telegram para blindagem ou bloqueio imediato\n")
		b.WriteString("  ✓ Backups versionados contínuos no S3\n\n")

		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).Render("★ MODO PILOTO: Ativação instantânea liberada para testes!\n\n"))

		b.WriteString(helpStyle.Render("[Enter] Ativar Plano Cloud Pro Agora  •  [Esc] Voltar"))
	}

	return boxStyle.Render(b.String())
}
