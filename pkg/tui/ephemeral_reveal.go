package tui

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/vault"
)

type ephemeralSecretViewer struct {
	field  vault.Field
	title  string
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func (v *ephemeralSecretViewer) SetStdin(r io.Reader)  { v.stdin = r }
func (v *ephemeralSecretViewer) SetStdout(w io.Writer) { v.stdout = w }
func (v *ephemeralSecretViewer) SetStderr(w io.Writer) { v.stderr = w }

func (v *ephemeralSecretViewer) Run() error {
	out := v.stdout
	if out == nil {
		out = os.Stdout
	}
	in := v.stdin
	if in == nil {
		in = os.Stdin
	}

	// 1. Limpa o terminal
	_, _ = io.WriteString(out, "\x1b[2J\x1b[H")

	// 2. Cabecalho de confirmacao
	_, _ = io.WriteString(out, "======================================================================\r\n")
	_, _ = io.WriteString(out, "KOFRE 🔐 VISUALIZACAO SEGURA EFEMERA (Zero-Memory)\r\n")
	_, _ = io.WriteString(out, "Item: "+v.title+"  |  Campo: "+v.field.Name+"\r\n")
	_, _ = io.WriteString(out, "======================================================================\r\n\r\n")
	_, _ = io.WriteString(out, "Pressione ENTER para revelar o segredo por 10 segundos (ou ESC/Q para cancelar): ")

	reader := bufio.NewReader(in)
	firstByte, err := reader.ReadByte()
	if err != nil || firstByte == 27 || firstByte == 'q' || firstByte == 'Q' {
		_, _ = io.WriteString(out, "\r\nOperacao cancelada.\r\n")
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(out, "\x1b[2J\x1b[H")
		return nil
	}

	// 3. Exibicao do segredo diretamente como byte stream
	_, _ = io.WriteString(out, "\x1b[2J\x1b[H")
	_, _ = io.WriteString(out, "======================================================================\r\n")
	_, _ = io.WriteString(out, "KOFRE 🔐 VISUALIZACAO SEGURA EFEMERA (Zero-Memory)\r\n")
	_, _ = io.WriteString(out, "Item: "+v.title+"  |  Campo: "+v.field.Name+"\r\n")
	_, _ = io.WriteString(out, "======================================================================\r\n\r\n")
	_, _ = io.WriteString(out, "  Segredo: ")

	err = v.field.WithValue(func(value []byte) error {
		_ = mycrypto.LockMemory(value)
		defer mycrypto.UnlockMemory(value)

		// Escreve os bytes brutos diretamente na saida, sem converter para string Go
		_, _ = out.Write(value)

		_, _ = io.WriteString(out, "\r\n\r\n======================================================================\r\n")
		_, _ = io.WriteString(out, "[Auto-limpeza em 10s... Pressione qualquer tecla para fechar]\r\n")

		// Aguarda tecla ou timer de 10s
		done := make(chan struct{})
		go func() {
			_, _ = reader.ReadByte()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(10 * time.Second):
		}

		// 4. Sobrescreve toda a tela e o buffer do console com espacos
		wipeBanner := bytes.Repeat([]byte(" "), 4096)
		_, _ = io.WriteString(out, "\x1b[2J\x1b[H")
		_, _ = out.Write(wipeBanner)
		_, _ = io.WriteString(out, "\x1b[2J\x1b[H")

		return nil
	})

	// 5. Expulsa quaisquer residuos e forca desalocacao
	runtime.GC()
	debug.FreeOSMemory()

	return err
}

func (m *Model) revealFieldEphemeral(field vault.Field, title string) tea.Cmd {
	return tea.Exec(&ephemeralSecretViewer{
		field: field,
		title: title,
	}, func(err error) tea.Msg {
		return ephemeralRevealDoneMsg{}
	})
}

type ephemeralRevealDoneMsg struct{}
