package tui

import (
	"bytes"
	"io"
	"os"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"kofre/pkg/config"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/vault"
)

type ephemeralSecretViewer struct {
	field     vault.Field
	withValue func(func([]byte) error) error
	title     string
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
	wait      func(time.Duration)
}

func (v *ephemeralSecretViewer) SetStdin(r io.Reader)  { v.stdin = r }
func (v *ephemeralSecretViewer) SetStdout(w io.Writer) { v.stdout = w }
func (v *ephemeralSecretViewer) SetStderr(w io.Writer) { v.stderr = w }

func (v *ephemeralSecretViewer) Run() (err error) {
	out := v.stdout
	if out == nil {
		out = os.Stdout
	}
	in := v.stdin
	if in == nil {
		in = os.Stdin
	}

	// A limpeza é tentada inclusive em erro de escrita ou cancelamento. Ela não
	// elimina capturas externas nem garante apagar o histórico de todo terminal.
	defer func() {
		if cleanupErr := limparVisualizacao(out); err == nil {
			err = cleanupErr
		}
	}()
	write := func(text string) error { _, e := io.WriteString(out, text); return e }
	tema := temas[0]
	if cfg, e := config.LoadConfig(); e == nil {
		tema = temas[indiceTema(cfg.Tema)]
	}
	borda := lipgloss.NewStyle().Foreground(tema.Destaque).Render("────────────────────────────────────────────────────")
	header := func() error {
		if e := write("\x1b[2J\x1b[H" + lipgloss.NewStyle().Bold(true).Foreground(tema.Destaque).Render("🛡️  Kofre · Visualização temporária") + "\r\n" + borda + "\r\n\r\nItem: "); e != nil {
			return e
		}
		if e := escreverTextoTerminal(out, []byte(v.title), false); e != nil {
			return e
		}
		if e := write("  |  Campo: "); e != nil {
			return e
		}
		if e := escreverTextoTerminal(out, []byte(v.field.Name), false); e != nil {
			return e
		}
		return write("\r\n" + borda + "\r\n\r\n")
	}
	if err = header(); err != nil {
		return err
	}
	if err = write("[Enter] Revelar por 10 segundos · [Esc] Voltar\r\n"); err != nil {
		return err
	}
	// Lê somente a confirmação, sem prefetch nem goroutine que sobreviva ao
	// retorno e roube teclas do Bubble Tea.
	var confirmation [1]byte
	if _, err = io.ReadFull(in, confirmation[:]); err != nil {
		return err
	}
	if confirmation[0] != '\r' && confirmation[0] != '\n' {
		return write("\r\nOperação cancelada.\r\n")
	}
	if err = header(); err != nil {
		return err
	}
	if err = write("  Segredo: "); err != nil {
		return err
	}

	withValue := v.withValue
	if withValue == nil {
		withValue = v.field.WithValue
	}
	err = withValue(func(value []byte) error {
		_ = mycrypto.LockMemory(value)
		defer mycrypto.UnlockMemory(value)

		// Não converte o segredo para string. Controles de terminal importados
		// são mostrados como escapes, nunca executados como ESC/OSC/BEL.
		return escreverTextoTerminal(out, value, true)
	})
	if err != nil {
		return err
	}

	if err = write("\r\n\r\n" + borda + "\r\nLimpeza e retorno automáticos em 10 segundos.\r\n"); err != nil {
		return err
	}
	wait := v.wait
	if wait == nil {
		wait = time.Sleep
	}
	// O callback já terminou e seus buffers foram apagados antes da espera.
	wait(10 * time.Second)
	return nil
}

func limparVisualizacao(out io.Writer) error {
	if _, err := io.WriteString(out, "\x1b[2J\x1b[H"); err != nil {
		return err
	}
	if _, err := out.Write(bytes.Repeat([]byte(" "), 4096)); err != nil {
		return err
	}
	_, err := io.WriteString(out, "\x1b[2J\x1b[H")
	return err
}

func escreverTextoTerminal(out io.Writer, value []byte, multiline bool) error {
	// Reserva uma vez para não abandonar cópias secretas durante realocações.
	safe := make([]byte, 0, 6*len(value))
	defer mycrypto.ZeroBytes(safe[:cap(safe)])
	const hex = "0123456789abcdef"
	for len(value) > 0 {
		r, size := utf8.DecodeRune(value)
		value = value[size:]
		if (r < 32 || r >= 127 && r <= 159) && !(multiline && (r == '\n' || r == '\t')) {
			safe = append(safe, '\\', 'x', hex[byte(r)>>4], hex[byte(r)&15])
		} else {
			safe = utf8.AppendRune(safe, r)
		}
	}
	n, err := out.Write(safe)
	if err == nil && n != len(safe) {
		return io.ErrShortWrite
	}
	return err
}

func (m *Model) revealNotesEphemeral(entry vault.SecretEntry) tea.Cmd {
	return tea.Exec(&ephemeralSecretViewer{field: vault.Field{Name: "Notas"}, title: entry.Title, withValue: entry.WithNotes}, func(err error) tea.Msg { return ephemeralRevealDoneMsg{err: err} })
}

func (m *Model) revealFieldEphemeral(field vault.Field, title string) tea.Cmd {
	return tea.Exec(&ephemeralSecretViewer{
		field: field,
		title: title,
	}, func(err error) tea.Msg {
		return ephemeralRevealDoneMsg{err: err}
	})
}

type ephemeralRevealDoneMsg struct{ err error }
