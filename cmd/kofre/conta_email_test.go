package main

import (
	"kofre/pkg/corporativo"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/vault"
	"testing"
)

func TestDestinoContaPreservaEntradasERecusaOutraIdentidade(t *testing.T) {
	i, err := corporativo.PrepararCadastro("Pessoa fictícia")
	if err != nil {
		t.Fatal(err)
	}
	defer i.Fechar()
	i.ID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	i.Pins["bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"] = "fingerprint-ficticio"
	v := vault.NewManaged()
	defer v.Close()
	if _, err = v.AddEntry(vault.SecretEntry{Title: "Entrada fictícia", Fields: []vault.Field{{Name: "Senha", Value: "segredo-ficticio", Protected: true}}}); err != nil {
		t.Fatal(err)
	}
	data, err := i.Serializar()
	if err != nil {
		t.Fatal(err)
	}
	if err = v.DefinirConta(data); err != nil {
		t.Fatal(err)
	}
	mycrypto.ZeroBytes(data)
	pass := []byte("senha-mestra-ficticia")
	key, salt, err := mycrypto.DeriveKeyBytes(pass, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(key)
	raw, err := v.Pack(key, salt)
	if err != nil {
		t.Fatal(err)
	}
	i.Pins = nil
	recovered, k, _, err := prepararDestinoConta(raw, pass, i)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Count() != 1 || i.Pins["bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"] != "fingerprint-ficticio" {
		t.Fatal("entrada ou fingerprint local perdidos")
	}
	recovered.Close()
	mycrypto.ZeroBytes(k)
	if _, _, _, err = prepararDestinoConta(raw, []byte("senha-incorreta"), i); err == nil {
		t.Fatal("senha incorreta aceita")
	}
	other, err := corporativo.PrepararCadastro("Outra pessoa")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Fechar()
	other.ID = "cccccccccccccccccccccccccccccccc"
	if _, _, _, err = prepararDestinoConta(raw, pass, other); err == nil {
		t.Fatal("outra conta substituiu identidade local")
	}
}
