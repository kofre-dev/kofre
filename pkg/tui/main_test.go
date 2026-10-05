package tui

import (
	"fmt"
	"os"
	"testing"
)

// A TUI consulta configuração mesmo com storage em memória. Nunca usa a conta pessoal nos testes.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "kofre-tui-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("APPDATA", dir)
	os.Setenv("XDG_CONFIG_HOME", dir)
	os.Setenv("KOFRE_CLOUD_ENDPOINT", "http://127.0.0.1:1")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
