package vault

import (
	"bytes"
	mycrypto "kofre/pkg/crypto"
	"testing"
)

func TestContaSegueSenhaSemParticiparDasCredenciais(t *testing.T) {
	v := NewManaged()
	defer v.Close()
	entry, err := v.AddEntry(SecretEntry{Title: "Credencial fictícia", Category: CategoryPassword})
	if err != nil {
		t.Fatal(err)
	}
	identidade := []byte(`{"token":"segredo-conta-ficticio","privada":"ficticia"}`)
	if err = v.DefinirConta(identidade); err != nil {
		t.Fatal(err)
	}
	key, salt, err := mycrypto.DeriveKeyBytes([]byte("senha-mestra-ficticia"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(key)
	packed, err := v.Pack(key, salt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(packed, ContaMagicHeader) || bytes.Contains(packed, identidade) {
		t.Fatal("identidade exposta ou formato sem proteção contra downgrade")
	}
	s, payload, err := UnpackHeader(packed)
	if err != nil {
		t.Fatal(err)
	}
	aberto, err := DecryptAndLoad(payload, key, s)
	if err != nil {
		t.Fatal(err)
	}
	defer aberto.Close()
	if err = aberto.LerConta(func(b []byte) error {
		if !bytes.Equal(b, identidade) {
			t.Fatal("conta alterada")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if aberto.Count() != 1 || len(aberto.Search("segredo-conta", "")) != 0 {
		t.Fatal("identidade vazou para listagem")
	}
	exportado, err := aberto.ExportarEntrada(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(exportado, []byte("segredo-conta")) || bytes.Contains(exportado, []byte(`"conta"`)) {
		t.Fatal("compartilhamento expôs a conta")
	}
	env, err := aberto.ToEnvMap("")
	if err != nil {
		t.Fatal(err)
	}
	for _, valor := range env {
		if bytes.Contains([]byte(valor), []byte("segredo-conta")) {
			t.Fatal("identidade exposta ao ambiente")
		}
	}
	newKey, newSalt, err := mycrypto.DeriveKeyBytes([]byte("outra-senha-mestra"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(newKey)
	packed, err = aberto.Pack(newKey, newSalt)
	if err != nil {
		t.Fatal(err)
	}
	s, payload, err = UnpackHeader(packed)
	if err != nil {
		t.Fatal(err)
	}
	if antigo, err := DecryptAndLoad(payload, key, s); err == nil {
		antigo.Close()
		t.Fatal("senha antiga ainda abriu")
	}
	novo, err := DecryptAndLoad(payload, newKey, s)
	if err != nil {
		t.Fatal(err)
	}
	if !novo.TemConta() {
		t.Fatal("troca de senha perdeu conta")
	}
	novo.Close()
	if err := novo.LerConta(func([]byte) error { t.Fatal("conta acessível após fechar"); return nil }); err == nil {
		t.Fatal("cofre fechado aceitou acesso")
	}
}
