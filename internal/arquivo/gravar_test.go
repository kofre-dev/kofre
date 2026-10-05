package arquivo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPreparacaoFalhaPreservaArquivoAnterior(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vault.enc")
	if err := Gravar(path, []byte("original"), nil); err != nil {
		t.Fatal(err)
	}
	err := Gravar(path, []byte("novo"), func(string) error { return errors.New("proteção recusada") })
	if err == nil {
		t.Fatal("ignorou falha de proteção")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "original" {
		t.Fatal("arquivo anterior perdido")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatal("temporário não foi removido")
	}
	if err = Gravar(path, []byte("novo"), nil); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	if string(got) != "novo" {
		t.Fatal("substituição não confirmou novo conteúdo")
	}
}
