package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	mycrypto "kofre/pkg/crypto"
	"unicode/utf16"
	"unicode/utf8"
)

// NewProtectedField recebe um buffer temporário; o chamador deve apagá-lo.
func NewProtectedField(name string, value []byte) (Field, error) {
	raw := appendQuotedBytes(value)
	defer mycrypto.ZeroBytes(raw)
	sealed, err := mycrypto.SealMemory(raw)
	return Field{Name: name, Protected: true, sealed: sealed}, err
}

// Close revoga este handle. Use para campos temporários após AddEntry/UpdateEntry.
func (f *Field) Close() { f.sealed.Close(); *f = Field{} }

// UnmarshalJSON não materializa campos protegidos como strings imutáveis.
func (f *Field) UnmarshalJSON(data []byte) error {
	var wire struct {
		Name      string          `json:"name"`
		Value     json.RawMessage `json:"value"`
		Protected bool            `json:"protected"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		mycrypto.ZeroBytes(wire.Value)
		return err
	}
	defer mycrypto.ZeroBytes(wire.Value)
	*f = Field{Name: wire.Name, Protected: wire.Protected}
	if !wire.Protected {
		return json.Unmarshal(wire.Value, &f.Value)
	}
	if len(wire.Value) == 0 || bytes.Equal(wire.Value, []byte("null")) {
		wire.Value = []byte(`""`)
	}
	plain, err := decodeValue(wire.Value)
	if err != nil {
		return err
	}
	defer mycrypto.ZeroBytes(plain)
	f.sealed, err = mycrypto.SealMemory(wire.Value)
	return err
}

// Evita exportação acidental e cópias em buffers internos do encoding/json.
func (f Field) MarshalJSON() ([]byte, error) {
	if f.Protected {
		return nil, errors.New("campo protegido exige serializacao controlada do cofre")
	}
	type public Field
	return json.Marshal(public(f))
}

// WithValue limita a vida útil do buffer decifrado ao callback.
func (f Field) WithValue(use func([]byte) error) error {
	if !f.Protected || f.sealed == nil {
		if f.Protected {
			return errors.New("campo protegido nao foi selado")
		}
		data := []byte(f.Value)
		defer mycrypto.ZeroBytes(data)
		return use(data)
	}
	return f.sealed.WithBytes(func(raw []byte) error {
		value, err := decodeValue(raw)
		if err != nil {
			return err
		}
		defer mycrypto.ZeroBytes(value)
		return use(value)
	})
}

// sealEntry cria novos buffers; não adota slices nem segredos do chamador.
func sealEntry(entry SecretEntry) (SecretEntry, error) {
	entry.Fields = append([]Field(nil), entry.Fields...)
	for i := range entry.Fields {
		f := &entry.Fields[i]
		if !f.Protected {
			continue
		}
		previous := f.sealed
		f.sealed = nil
		var err error
		if previous != nil && f.Value == "" {
			err = previous.WithBytes(func(raw []byte) error { f.sealed, err = mycrypto.SealMemory(raw); return err })
		} else {
			raw := appendQuoted(nil, f.Value)
			f.sealed, err = mycrypto.SealMemory(raw)
			mycrypto.ZeroBytes(raw)
		}
		f.Value = ""
		if err != nil {
			for j := 0; j <= i; j++ {
				entry.Fields[j].sealed.Close()
			}
			return SecretEntry{}, err
		}
	}
	entry.Attachments = append([]Attachment(nil), entry.Attachments...)
	for i := range entry.Attachments {
		entry.Attachments[i].Data = append([]byte(nil), entry.Attachments[i].Data...)
	}
	return entry, nil
}

func cloneEntry(entry SecretEntry) SecretEntry {
	entry.Fields = append([]Field(nil), entry.Fields...)
	entry.Attachments = append([]Attachment(nil), entry.Attachments...)
	for i := range entry.Attachments {
		entry.Attachments[i].Data = append([]byte(nil), entry.Attachments[i].Data...)
	}
	return entry
}

func closeEntry(entry *SecretEntry) {
	for i := range entry.Fields {
		entry.Fields[i].sealed.Close()
		entry.Fields[i] = Field{}
	}
	for i := range entry.Attachments {
		mycrypto.ZeroBytes(entry.Attachments[i].Data)
	}
	*entry = SecretEntry{}
}

// Acrescenta JSON sem passar o segredo pelos pools do encoding/json.
func appendQuoted(dst []byte, value string) []byte {
	plain := []byte(value)
	defer mycrypto.ZeroBytes(plain)
	encoded := appendQuotedBytes(plain)
	if len(dst) == 0 {
		return encoded
	}
	defer mycrypto.ZeroBytes(encoded)
	return append(dst, encoded...)
}

func appendQuotedBytes(value []byte) []byte {
	// Reserva antes da escrita para não abandonar cópias secretas em realocações.
	out := make([]byte, 0, 6*len(value)+2)
	out = append(out, '"')
	const hex = "0123456789abcdef"
	for len(value) > 0 {
		r, size := utf8.DecodeRune(value)
		value = value[size:]
		switch {
		case r == '"' || r == '\\':
			out = append(out, '\\', byte(r))
		case r < 32:
			out = append(out, '\\', 'u', '0', '0', hex[byte(r)>>4], hex[byte(r)&15])
		default:
			out = utf8.AppendRune(out, r)
		}
	}
	return append(out, '"')
}

// decodeValue aceita a mesma sintaxe JSON, incluindo pares substitutos Unicode.
func decodeValue(raw []byte) ([]byte, error) {
	raw = bytes.Trim(raw, " \t\r\n")
	if !json.Valid(raw) || len(raw) < 2 || raw[0] != '"' {
		return nil, errors.New("valor de campo invalido")
	}
	out := make([]byte, 0, len(raw)*3)
	hex4 := func(b []byte) rune {
		var r rune
		for _, c := range b {
			r <<= 4
			switch {
			case c >= '0' && c <= '9':
				r += rune(c - '0')
			case c >= 'a' && c <= 'f':
				r += rune(c - 'a' + 10)
			default:
				r += rune(c - 'A' + 10)
			}
		}
		return r
	}
	for i := 1; i < len(raw)-1; i++ {
		c := raw[i]
		if c != '\\' {
			if c < utf8.RuneSelf {
				out = append(out, c)
			} else {
				r, size := utf8.DecodeRune(raw[i : len(raw)-1])
				out = utf8.AppendRune(out, r)
				i += size - 1
			}
			continue
		}
		i++
		switch raw[i] {
		case '"', '\\', '/':
			out = append(out, raw[i])
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'u':
			r := hex4(raw[i+1 : i+5])
			i += 4
			if r >= 0xD800 && r <= 0xDBFF && i+6 < len(raw)-1 && raw[i+1] == '\\' && raw[i+2] == 'u' {
				next := hex4(raw[i+3 : i+7])
				if next >= 0xDC00 && next <= 0xDFFF {
					r = utf16.DecodeRune(r, next)
					i += 6
				}
			}
			out = utf8.AppendRune(out, r)
		}
	}
	return out, nil
}

// marshalVault serializa campos sensíveis sem o pool de buffers de json.Marshal.
// Cada crescimento apaga o buffer anterior; o chamador apaga o resultado.
func marshalVault(v *Vault) (result []byte, err error) {
	appendSafe := func(data []byte) {
		if len(result)+len(data) > cap(result) {
			next := make([]byte, len(result), 2*(len(result)+len(data)))
			copy(next, result)
			mycrypto.ZeroBytes(result)
			result = next
		}
		result = append(result, data...)
	}
	defer func() {
		if err != nil {
			mycrypto.ZeroBytes(result)
			result = nil
		}
	}()
	meta := *v
	meta.Entries = nil
	meta.Conta = nil
	head, err := json.Marshal(struct {
		*Vault
		Entries []SecretEntry `json:"entries,omitempty"`
	}{Vault: &meta})
	if err != nil {
		return nil, err
	}
	appendSafe(head[:len(head)-1])
	if v.Conta != nil {
		appendSafe([]byte(`,"conta":{"name":"Identidade da conta","protected":true,"value":`))
		if err = v.Conta.sealed.WithBytes(func(raw []byte) error { appendSafe(raw); return nil }); err != nil {
			return result, err
		}
		appendSafe([]byte(`}`))
	}
	appendSafe([]byte(`,"entries":[`))
	for i, entry := range v.Entries {
		if i > 0 {
			appendSafe([]byte(","))
		}
		fields := entry.Fields
		entry.Fields = nil
		encoded, e := json.Marshal(struct {
			*SecretEntry
			Fields []Field `json:"fields,omitempty"`
		}{SecretEntry: &entry})
		if e != nil {
			return result, e
		}
		appendSafe(encoded[:len(encoded)-1])
		appendSafe([]byte(`,"fields":[`))
		for j, field := range fields {
			if j > 0 {
				appendSafe([]byte(","))
			}
			name, _ := json.Marshal(field.Name)
			appendSafe([]byte(`{"name":`))
			appendSafe(name)
			appendSafe([]byte(`,"value":`))
			if field.Protected {
				err = field.sealed.WithBytes(func(raw []byte) error { appendSafe(raw); return nil })
				if err != nil {
					return result, err
				}
				appendSafe([]byte(`,"protected":true}`))
			} else {
				value := appendQuoted(nil, field.Value)
				appendSafe(value)
				mycrypto.ZeroBytes(value)
				appendSafe([]byte(`,"protected":false}`))
			}
		}
		appendSafe([]byte("]}"))
	}
	appendSafe([]byte("]}"))
	return result, nil
}
