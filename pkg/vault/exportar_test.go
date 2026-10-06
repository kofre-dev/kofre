package vault

import (
	"bytes"
	"testing"
)

func TestExportarSomenteEntradaEscolhidaSemAlterarCofre(t *testing.T) {
	v := NewManaged()
	defer v.Close()
	escolhida, err := v.AddEntry(SecretEntry{Title: "Projeto fictício", Category: CategoryPassword, Fields: []Field{{Name: "Senha", Value: "valor-secreto-ficticio", Protected: true}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = v.AddEntry(SecretEntry{Title: "Pessoal privado", Category: CategoryPassword, Fields: []Field{{Name: "Senha", Value: "nao-pode-sair", Protected: true}}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := v.ExportarEntrada(escolhida.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(b)
	if !bytes.Contains(b, []byte("valor-secreto-ficticio")) || bytes.Contains(b, []byte("nao-pode-sair")) || bytes.Contains(b, []byte("Pessoal privado")) {
		t.Fatal("exportação não isolou entrada")
	}
	if v.Count() != 2 {
		t.Fatal("exportação alterou cofre pessoal")
	}
	if _, err = v.ExportarEntrada("inexistente"); err == nil {
		t.Fatal("entrada inexistente aceita")
	}
	v.Close()
	if _, err = v.ExportarEntrada(escolhida.ID); err == nil {
		t.Fatal("cofre fechado exportado")
	}
}
