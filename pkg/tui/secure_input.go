package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"runtime"
	"strings"
	"unicode/utf8"
)

type secretInputState struct {
	runes []rune
	pos   int
}

// input mantém a API visual usual, mas não entrega senhas ao textinput.
// O estado por ponteiro permite apagar também os modelos anteriores do Bubble Tea.
type input struct {
	textinput.Model
	secret *secretInputState
}

func newInput(secret bool) input {
	i := input{Model: textinput.New()}
	if secret {
		i.secret = &secretInputState{}
	}
	return i
}

func wipeRunes(value []rune) { clear(value); runtime.KeepAlive(value) }

func (i *input) Reset() {
	if i.secret == nil {
		i.Model.Reset()
		return
	}
	wipeRunes(i.secret.runes)
	i.secret.runes, i.secret.pos = nil, 0
}

func (i *input) SetValue(value string) {
	if i.secret == nil {
		i.Model.SetValue(value)
		return
	}
	i.Reset()
	i.secret.runes = []rune(value)
	i.secret.pos = len(i.secret.runes)
}

func (i *input) SetBytes(value []byte) {
	i.Reset()
	i.secret.runes = make([]rune, 0, len(value))
	for len(value) > 0 {
		r, size := utf8.DecodeRune(value)
		i.secret.runes = append(i.secret.runes, r)
		value = value[size:]
	}
	i.secret.pos = len(i.secret.runes)
}

// Value só é usado na derivação/salvamento. A string temporária não é retida pelo modelo.
func (i input) Value() string {
	if i.secret == nil {
		return i.Model.Value()
	}
	return string(i.secret.runes)
}

func (i input) Bytes() []byte {
	if i.secret == nil {
		return []byte(i.Model.Value())
	}
	value := make([]byte, 0, len(i.secret.runes)*4)
	for _, r := range i.secret.runes {
		value = utf8.AppendRune(value, r)
	}
	return value
}

func (i input) View() string {
	if i.secret == nil {
		return i.Model.View()
	}
	if len(i.secret.runes) == 0 {
		return i.Prompt + i.Placeholder
	}
	masked := strings.Repeat("•", len(i.secret.runes))
	if i.Focused() {
		masked += "│"
	}
	return i.Prompt + masked
}

func (i input) Update(msg tea.Msg) (input, tea.Cmd) {
	if i.secret == nil {
		var cmd tea.Cmd
		i.Model, cmd = i.Model.Update(msg)
		return i, cmd
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok || !i.Focused() {
		return i, nil
	}
	s := i.secret
	switch key.Type {
	case tea.KeyRunes, tea.KeySpace:
		if key.Type == tea.KeySpace && len(key.Runes) == 0 {
			key.Runes = []rune{' '}
		}
		// Uma alocação controlada por edição; apaga o buffer anterior antes de soltá-lo.
		added := make([]rune, 0, len(key.Runes))
		for _, r := range key.Runes {
			if r >= 32 && r != 127 && utf8.ValidRune(r) {
				added = append(added, r)
			}
		}
		next := make([]rune, 0, len(s.runes)+len(added))
		next = append(next, s.runes[:s.pos]...)
		next = append(next, added...)
		next = append(next, s.runes[s.pos:]...)
		s.pos += len(added)
		wipeRunes(added)
		wipeRunes(s.runes)
		s.runes = next
	case tea.KeyBackspace, tea.KeyCtrlH:
		if s.pos > 0 {
			copy(s.runes[s.pos-1:], s.runes[s.pos:])
			s.runes[len(s.runes)-1] = 0
			s.runes = s.runes[:len(s.runes)-1]
			s.pos--
		}
	case tea.KeyDelete:
		if s.pos < len(s.runes) {
			copy(s.runes[s.pos:], s.runes[s.pos+1:])
			s.runes[len(s.runes)-1] = 0
			s.runes = s.runes[:len(s.runes)-1]
		}
	case tea.KeyLeft:
		if s.pos > 0 {
			s.pos--
		}
	case tea.KeyRight:
		if s.pos < len(s.runes) {
			s.pos++
		}
	case tea.KeyHome, tea.KeyCtrlA:
		s.pos = 0
	case tea.KeyEnd, tea.KeyCtrlE:
		s.pos = len(s.runes)
	case tea.KeyCtrlU:
		i.Reset()
	}
	return i, nil
}
