package tui

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"kofre/pkg/vault"
)

func TestViewerApagaBuffersAntesDaEsperaESoConsomeConfirmacao(t *testing.T) {
	field, err := vault.NewProtectedField("Senha", []byte("segredo-ficticio"))
	if err != nil {
		t.Fatal(err)
	}
	defer field.Close()
	input := bytes.NewReader([]byte("\rX"))
	var output bytes.Buffer
	var held []byte
	waits := 0
	viewer := ephemeralSecretViewer{
		field: field, stdin: input, stdout: &output,
		withValue: func(use func([]byte) error) error {
			return field.WithValue(func(value []byte) error { held = value; return use(value) })
		},
		wait: func(duration time.Duration) {
			waits++
			if duration != 10*time.Second {
				t.Fatal("janela de exibição inesperada")
			}
			if len(held) == 0 || !bytes.Equal(held, make([]byte, len(held))) {
				t.Fatal("buffer do segredo ficou vivo durante a espera")
			}
			if input.Len() != 1 {
				t.Fatal("viewer consumiu teclas além da confirmação")
			}
		},
	}
	if err := viewer.Run(); err != nil {
		t.Fatal(err)
	}
	if waits != 1 || input.Len() != 1 {
		t.Fatal("espera incorreta ou leitor residual")
	}
	if !bytes.HasSuffix(output.Bytes(), []byte("\x1b[2J\x1b[H")) {
		t.Fatal("saída não foi limpa")
	}
}

func TestViewerCanceladoNaoAbreSegredo(t *testing.T) {
	for _, input := range []string{"q", "x", "\x1b"} {
		var output bytes.Buffer
		viewer := ephemeralSecretViewer{
			stdin: bytes.NewBufferString(input), stdout: &output,
			withValue: func(func([]byte) error) error { t.Fatal("cancelamento abriu segredo"); return nil },
			wait:      func(time.Duration) { t.Fatal("cancelamento aguardou janela") },
		}
		if err := viewer.Run(); err != nil {
			t.Fatal(err)
		}
		if !bytes.HasSuffix(output.Bytes(), []byte("\x1b[2J\x1b[H")) {
			t.Fatal("cancelamento não limpou saída")
		}
	}
}

type failSecretWriter struct {
	bytes.Buffer
	failure error
}

func (w *failSecretWriter) Write(data []byte) (int, error) {
	if bytes.Equal(data, []byte("segredo-ficticio")) {
		return 0, w.failure
	}
	return w.Buffer.Write(data)
}

func TestViewerLimpaMesmoQuandoEscritaDoSegredoFalha(t *testing.T) {
	field, err := vault.NewProtectedField("Senha", []byte("segredo-ficticio"))
	if err != nil {
		t.Fatal(err)
	}
	defer field.Close()
	expected := errors.New("saída fictícia indisponível")
	writer := &failSecretWriter{failure: expected}
	viewer := ephemeralSecretViewer{
		field: field, stdin: bytes.NewBufferString("\n"), stdout: writer,
		wait: func(time.Duration) { t.Fatal("erro de saída iniciou espera") },
	}
	if err := viewer.Run(); !errors.Is(err, expected) {
		t.Fatalf("erro não propagado: %v", err)
	}
	if !bytes.HasSuffix(writer.Bytes(), []byte("\x1b[2J\x1b[H")) {
		t.Fatal("erro deixou de tentar limpeza")
	}
}

func TestTextoTerminalEscapaComandosEApagaCopia(t *testing.T) {
	value := []byte("normal\n\t\x1b]52;c;dados\x07\b\x7f\u009b31m")
	var held []byte
	writer := writerFunc(func(data []byte) (int, error) {
		held = data
		if bytes.ContainsAny(data, "\x1b\x07\b\x7f") || bytes.Contains(data, []byte("\u009b")) {
			t.Fatal("controle de terminal foi emitido")
		}
		if !bytes.Contains(data, []byte(`\x1b]52`)) || !bytes.Contains(data, []byte("normal\n\t")) {
			t.Fatal("escape ou texto multilinha alterado")
		}
		return len(data), nil
	})
	if err := escreverTextoTerminal(writer, value, true); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(held, make([]byte, len(held))) {
		t.Fatal("cópia de exibição não apagada")
	}
	if err := escreverTextoTerminal(writerFunc(func([]byte) (int, error) { return 0, nil }), []byte("valor"), true); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("escrita parcial não detectada")
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(data []byte) (int, error) { return f(data) }
