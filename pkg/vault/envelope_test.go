package vault

import (
	"bytes"
	"encoding/json"
	mycrypto "kofre/pkg/crypto"
	"testing"
)

func envelopeFixture(t *testing.T) (*ManagedVault, []byte, []byte, []byte) {
	t.Helper()
	v := NewManaged()
	t.Cleanup(v.Close)
	for _, id := range []string{"item-a", "item-b"} {
		if _, err := v.AddEntry(SecretEntry{ID: id, Title: id, Notes: "nota-" + id, Fields: []Field{{Name: "Senha", Protected: true, Value: "segredo-" + id}}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := v.DefinirConta([]byte("conta-fixture")); err != nil {
		t.Fatal(err)
	}
	key, salt := bytes.Repeat([]byte{0x11}, 32), bytes.Repeat([]byte{0x22}, 16)
	raw, err := v.Pack(key, salt)
	if err != nil {
		t.Fatal(err)
	}
	return v, raw, key, salt
}

func TestEnvelopeV3ResisteAdulteracaoDoFramingECifra(t *testing.T) {
	_, raw, key, salt := envelopeFixture(t)
	for _, offset := range []int{0, 8, 24, envelopePrefixLength, envelopePrefixLength + wrappedKeyLength - 1, envelopePrefixLength + wrappedKeyLength, len(raw) - 1} {
		corrupt := bytes.Clone(raw)
		corrupt[offset] ^= 1
		s, payload, err := UnpackHeader(corrupt)
		if err != nil {
			continue
		}
		if v, err := DecryptAndLoad(payload, key, s); err == nil {
			v.Close()
			t.Fatalf("alteração aceita no byte %d", offset)
		}
	}
	for _, magic := range [][]byte{MagicHeader, ContaMagicHeader, LegacyMagicHeader} {
		corrupt := bytes.Clone(raw)
		copy(corrupt, magic)
		s, payload, err := UnpackHeader(corrupt)
		if err != nil {
			t.Fatal(err)
		}
		if v, err := DecryptAndLoad(payload, key, s); err == nil {
			v.Close()
			t.Fatal("downgrade do cabeçalho aceito")
		}
	}
	wrongKey := bytes.Clone(key)
	wrongKey[0] ^= 1
	if v, err := DecryptAndLoad(raw, wrongKey, salt); err == nil {
		v.Close()
		t.Fatal("chave errada abriu")
	}
	wrongSalt := bytes.Clone(salt)
	wrongSalt[0] ^= 1
	if v, err := DecryptAndLoad(raw, key, wrongSalt); err == nil {
		v.Close()
		t.Fatal("salt divergente aceito")
	}
	for _, length := range []int{0, 7, 23, 39, 99, 100, len(raw) - 1} {
		s, payload, err := UnpackHeader(raw[:length])
		if err != nil {
			continue
		}
		if v, err := DecryptAndLoad(payload, key, s); err == nil {
			v.Close()
			t.Fatal("truncamento aceito")
		}
	}
}

func TestEnvelopeSeparaChaveECatalogoDosDetalhes(t *testing.T) {
	v, raw, key, _ := envelopeFixture(t)
	prefix := raw[:envelopePrefixLength]
	dataKey, err := mycrypto.DecryptWithAAD(raw[envelopePrefixLength:envelopePrefixLength+wrappedKeyLength], key, envelopeContext(prefix, "chave", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(dataKey)
	if bytes.Equal(dataKey, key) {
		t.Fatal("chave derivada usada como chave de dados")
	}
	encoded, err := mycrypto.DecryptWithAAD(raw[envelopePrefixLength+wrappedKeyLength:], dataKey, envelopeContext(prefix, "catalogo", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(encoded)
	for _, secret := range []string{"segredo-item-a", "segredo-item-b", "nota-item-a", "conta-fixture"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatal("catálogo contém detalhe em texto simples")
		}
	}
	var catalog vaultCatalog
	if err := json.Unmarshal(encoded, &catalog); err != nil {
		t.Fatal(err)
	}
	// Quem altera somente o catálogo, mesmo recalculando sua cifra para testar o
	// limite interno, não consegue trocar o contexto de um detalhe cifrado.
	catalog.Entries[0].Ciphertext, catalog.Entries[1].Ciphertext = catalog.Entries[1].Ciphertext, catalog.Entries[0].Ciphertext
	changed, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := mycrypto.EncryptWithAAD(changed, dataKey, envelopeContext(prefix, "catalogo", ""))
	if err != nil {
		t.Fatal(err)
	}
	corrupt := append(bytes.Clone(raw[:envelopePrefixLength+wrappedKeyLength]), ciphertext...)
	if opened, err := DecryptAndLoad(corrupt, key, raw[8:24]); err == nil {
		opened.Close()
		t.Fatal("troca de detalhes entre itens aceita")
	}
	newKey, newSalt := bytes.Repeat([]byte{0x33}, 32), bytes.Repeat([]byte{0x44}, 16)
	if err := v.RotacionarChaveDados(); err != nil {
		t.Fatal(err)
	}
	repacked, err := v.Pack(newKey, newSalt)
	if err != nil {
		t.Fatal(err)
	}
	nextKey, err := mycrypto.DecryptWithAAD(repacked[envelopePrefixLength:envelopePrefixLength+wrappedKeyLength], newKey, envelopeContext(repacked[:envelopePrefixLength], "chave", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(nextKey)
	if bytes.Equal(dataKey, nextKey) {
		t.Fatal("troca de senha manteve a chave de dados antiga")
	}
	if _, err := mycrypto.DecryptWithAAD(repacked[envelopePrefixLength+wrappedKeyLength:], dataKey, envelopeContext(repacked[:envelopePrefixLength], "catalogo", "")); err == nil {
		t.Fatal("DEK extraída de backup antigo abriu a versão posterior à troca")
	}
	if opened, err := DecryptAndLoad(repacked, key, newSalt); err == nil {
		opened.Close()
		t.Fatal("chave antiga abriu novo envelope")
	}
	opened, err := DecryptAndLoad(repacked, newKey, newSalt)
	if err != nil {
		t.Fatal(err)
	}
	opened.Close()
}

func TestLegadoMigraPreservandoNotasAnexosEConta(t *testing.T) {
	legacy := []byte(`{"schema_version":1,"entries":[{"id":"antigo","title":"Legado","fields":[{"name":"Senha","value":"fixture-segredo","protected":true}],"notes":"nota legada","attachments":[{"filename":"chave.bin","data":"AAH+","size":3}]}],"conta":{"name":"Identidade","value":"conta legada","protected":true}}`)
	key, salt := make([]byte, 32), make([]byte, 16)
	payload, err := mycrypto.Encrypt(legacy, key)
	if err != nil {
		t.Fatal(err)
	}
	for _, magic := range [][]byte{MagicHeader, ContaMagicHeader, LegacyMagicHeader} {
		raw := append(append(bytes.Clone(magic), salt...), payload...)
		s, p, e := UnpackHeader(raw)
		if e != nil {
			t.Fatal(e)
		}
		v, e := DecryptAndLoad(p, key, s)
		if e != nil {
			t.Fatal(e)
		}
		newRaw, e := v.Pack(key, s)
		v.Close()
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.HasPrefix(newRaw, EnvelopeMagicHeader) {
			t.Fatal("gravação não migrou")
		}
		opened, e := DecryptAndLoad(newRaw, key, s)
		if e != nil {
			t.Fatal(e)
		}
		entry, e := opened.GetEntry("antigo")
		if e != nil {
			t.Fatal(e)
		}
		assertNotesAttachment(t, entry, []byte("nota legada"), []byte{0, 1, 254})
		if fieldValue(t, entry.Fields[0]) != "fixture-segredo" || !opened.TemConta() {
			t.Fatal("migração perdeu dados")
		}
		opened.Close()
	}
}

func TestEnvelopeNaoRegravaVersaoFutura(t *testing.T) {
	v, raw, _, salt := envelopeFixture(t)
	var changed []byte
	err := v.dataKey.WithBytes(func(key []byte) error {
		prefix := raw[:envelopePrefixLength]
		plain, err := mycrypto.DecryptWithAAD(raw[envelopePrefixLength+wrappedKeyLength:], key, envelopeContext(prefix, "catalogo", ""))
		if err != nil {
			return err
		}
		defer mycrypto.ZeroBytes(plain)
		var catalog vaultCatalog
		if err := json.Unmarshal(plain, &catalog); err != nil {
			return err
		}
		catalog.SchemaVersion = 2
		encoded, err := json.Marshal(catalog)
		if err != nil {
			return err
		}
		defer mycrypto.ZeroBytes(encoded)
		ciphertext, err := mycrypto.EncryptWithAAD(encoded, key, envelopeContext(prefix, "catalogo", ""))
		if err != nil {
			return err
		}
		changed = append(bytes.Clone(raw[:envelopePrefixLength+wrappedKeyLength]), ciphertext...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if opened, err := DecryptAndLoad(changed, bytes.Repeat([]byte{0x11}, 32), salt); err == nil {
		opened.Close()
		t.Fatal("catálogo futuro foi aceito")
	}
	v.data.SchemaVersion = 2
	if _, err := v.Pack(make([]byte, 32), salt); err == nil {
		t.Fatal("catálogo futuro foi regravado")
	}
}

func FuzzEnvelopeRejectsMalformed(f *testing.F) {
	v := NewManaged()
	_, _ = v.AddEntry(SecretEntry{Title: "Fixture", Notes: "nota"})
	raw, err := v.Pack(make([]byte, 32), make([]byte, 16))
	if err != nil {
		f.Fatal(err)
	}
	v.Close()
	f.Add(raw)
	f.Add([]byte("KOFRE003"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > 1024*1024 {
			return
		}
		s, p, err := UnpackHeader(raw)
		if err != nil {
			return
		}
		v, err := DecryptAndLoad(p, make([]byte, 32), s)
		if err == nil {
			v.Close()
		}
	})
}
