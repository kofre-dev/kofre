package vault

import (
	"bytes"
	"encoding/json"
	mycrypto "kofre/pkg/crypto"
	"strings"
	"testing"
)

func fieldValue(t *testing.T, field Field) string {
	t.Helper()
	var result string
	if err := field.WithValue(func(value []byte) error { result = string(value); return nil }); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestProtectedFieldWireCompatibility(t *testing.T) {
	for _, value := range []string{"", " a****F ", "a\"b\\c\n\x00", "acentuação 🔐", strings.Repeat("segredo", 10000)} {
		t.Run("tamanho", func(t *testing.T) {
			v := NewManaged()
			defer v.Close()
			entry, err := v.AddEntry(SecretEntry{Title: "fixture", Fields: []Field{{Name: "Senha", Value: value, Protected: true}}})
			if err != nil {
				t.Fatal(err)
			}
			if entry.Fields[0].Value != "" || v.data.Entries[0].Fields[0].Value != "" {
				t.Fatal("senha retida em string")
			}
			if got := fieldValue(t, entry.Fields[0]); got != value {
				t.Fatal("senha alterada ao selar")
			}
			key, salt := make([]byte, 32), make([]byte, 16)
			packed, err := v.Pack(key, salt)
			if err != nil {
				t.Fatal(err)
			}
			_, payload, err := UnpackHeader(packed)
			if err != nil {
				t.Fatal(err)
			}
			plain, err := mycrypto.Decrypt(payload, key)
			if err != nil {
				t.Fatal(err)
			}
			defer mycrypto.ZeroBytes(plain)
			// Leitor independente representa o formato usado pelas versões antigas.
			var wire struct {
				Entries []struct {
					Fields []struct {
						Value     string
						Protected bool
					}
				}
			}
			if err := json.Unmarshal(plain, &wire); err != nil {
				t.Fatal(err)
			}
			if wire.Entries[0].Fields[0].Value != value || !wire.Entries[0].Fields[0].Protected {
				t.Fatal("formato persistido mudou")
			}
			loaded, err := DecryptAndLoad(payload, key, salt)
			if err != nil {
				t.Fatal(err)
			}
			defer loaded.Close()
			if got := fieldValue(t, loaded.Entries()[0].Fields[0]); got != value {
				t.Fatal("recarga alterou senha")
			}
		})
	}
}

func TestLegacyJSONProtectedLoad(t *testing.T) {
	raw := []byte(`{"schema_version":1,"entries":[{"id":"legacy","title":"Compatibilidade","fields":[{"name":"Senha","value":"a\uD83D\uDD10\nF","protected":true}]}]}`)
	key := make([]byte, 32)
	payload, err := mycrypto.Encrypt(raw, key)
	if err != nil {
		t.Fatal(err)
	}
	v, err := DecryptAndLoad(payload, key, make([]byte, 16))
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	entry, err := v.GetEntry("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Fields[0].Value != "" || fieldValue(t, entry.Fields[0]) != "a🔐\nF" {
		t.Fatal("fixture legada alterada")
	}
}

func TestVaultRevocationAndCopies(t *testing.T) {
	v := NewManaged()
	entry, err := v.AddEntry(SecretEntry{Title: "AWS", Fields: []Field{{Name: "Senha", Value: "temporario", Protected: true}, {Name: "Usuario", Value: "original"}}})
	if err != nil {
		t.Fatal(err)
	}
	entry.Fields[1].Value = "alteracao externa"
	if v.Entries()[0].Fields[1].Value != "original" {
		t.Fatal("slice publico altera cofre")
	}
	for _, e := range v.Search("", "") {
		if e.Fields[0].Value != "" {
			t.Fatal("busca revelou senha")
		}
	}
	old := entry.Fields[0]
	entry.Title = "Novo titulo"
	if err := v.UpdateEntry(entry); err != nil {
		t.Fatal(err)
	}
	if err := old.WithValue(func([]byte) error { return nil }); err == nil {
		t.Fatal("handle substituido continua acessivel")
	}
	fresh, _ := v.GetEntry(entry.ID)
	if fieldValue(t, fresh.Fields[0]) != "temporario" {
		t.Fatal("edicao perdeu segredo preservado")
	}
	env, err := v.ToEnvMap("")
	if err != nil || env["NOVO_TITULO"] != "temporario" {
		t.Fatal("exportacao explicita falhou")
	}
	v.Close()
	if err := fresh.Fields[0].WithValue(func([]byte) error { return nil }); err == nil {
		t.Fatal("Close nao revogou handle externo")
	}
	if _, err := v.Pack(make([]byte, 32), make([]byte, 16)); err == nil {
		t.Fatal("cofre encerrado foi salvo vazio")
	}
	if _, err := v.AddEntry(SecretEntry{}); err == nil {
		t.Fatal("cofre encerrado aceitou entrada")
	}
}

func TestDeleteRevokesAndClearsTail(t *testing.T) {
	v := NewManaged()
	defer v.Close()
	e, err := v.AddEntry(SecretEntry{Fields: []Field{{Name: "Senha", Value: "fixture", Protected: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.DeleteEntry(e.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.Fields[0].WithValue(func([]byte) error { return nil }); err == nil {
		t.Fatal("handle excluido ainda funciona")
	}
	if v.Count() != 0 {
		t.Fatal("entrada nao excluida")
	}
}

func TestTemporaryValueWipedOnError(t *testing.T) {
	f, err := NewProtectedField("Senha", []byte("canario"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var held []byte
	if err := f.WithValue(func(value []byte) error { held = value; return ErrNotFound }); err != ErrNotFound {
		t.Fatal("erro perdido")
	}
	if !bytes.Equal(held, make([]byte, len(held))) {
		t.Fatal("senha temporaria nao apagada")
	}
}

func FuzzDecodeValueCompatibility(f *testing.F) {
	for _, seed := range []string{`""`, `"a\n\uD83D\uDD10F"`, `"\uD800"`, `"\uDC00"`, " \"unicode 🔐\" ", "\"\xff\""} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || trimmed[0] != '"' {
			return
		}
		var expected string
		referenceErr := json.Unmarshal(raw, &expected)
		actual, err := decodeValue(raw)
		defer mycrypto.ZeroBytes(actual)
		if (referenceErr == nil) != (err == nil) {
			t.Fatal("validacao diverge do decoder JSON")
		}
		if err == nil && string(actual) != expected {
			t.Fatal("valor diverge do decoder JSON")
		}
	})
}
