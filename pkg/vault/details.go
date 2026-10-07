package vault

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	mycrypto "kofre/pkg/crypto"
)

// WithNotes disponibiliza somente um buffer temporário, apagado ao retornar.
// Notes é um campo de entrada para importadores; o cofre gerenciado o esvazia.
func (e SecretEntry) WithNotes(use func([]byte) error) error {
	if e.notes != nil {
		return e.notes.WithBytes(use)
	}
	value := []byte(e.Notes)
	defer mycrypto.ZeroBytes(value)
	return use(value)
}

func (e SecretEntry) HasNotes() bool { return e.notes != nil || e.Notes != "" }

// SetNotes cria um handle temporário independente. Depois de AddEntry ou
// UpdateEntry, o chamador deve fechar esse handle com CloseNotes. Não revoga o
// handle anterior, que pode pertencer a uma entrada gerenciada ainda em uso.
func (e *SecretEntry) SetNotes(value []byte) error {
	sealed, err := mycrypto.SealMemory(value)
	if err != nil {
		return err
	}
	if e.notesTemporary {
		e.notes.Close()
	}
	e.notes, e.Notes = sealed, ""
	e.notesTemporary = true
	return nil
}

// CloseNotes fecha somente notas temporárias criadas por SetNotes. Não use em
// entradas obtidas por GetEntry/Entries: esses handles pertencem ao cofre.
func (e *SecretEntry) CloseNotes() {
	if e.notesTemporary {
		e.notes.Close()
	}
	e.notes, e.Notes, e.notesTemporary = nil, "", false
}

// Impede que um json.Marshal incidental descarte notas/anexos silenciosamente
// ou crie cópias desses segredos nos pools do serializador.
func (e SecretEntry) MarshalJSON() ([]byte, error) {
	if e.notes != nil {
		return nil, errors.New("notas protegidas exigem serialização controlada")
	}
	for _, attachment := range e.Attachments {
		if attachment.sealed != nil {
			return nil, errors.New("anexo protegido exige serialização controlada")
		}
	}
	type plainEntry SecretEntry
	return json.Marshal(plainEntry(e))
}

// WithData permite usar o anexo sem entregar uma cópia persistente aos leitores.
func (a Attachment) WithData(use func([]byte) error) error {
	if a.sealed != nil {
		return a.sealed.WithBytes(use)
	}
	value := bytes.Clone(a.Data)
	defer mycrypto.ZeroBytes(value)
	return use(value)
}

func (a Attachment) HasData() bool { return a.sealed != nil || len(a.Data) != 0 }

func (a Attachment) MarshalJSON() ([]byte, error) {
	if a.sealed != nil {
		return nil, errors.New("anexo protegido exige serialização controlada")
	}
	type plainAttachment Attachment
	return json.Marshal(plainAttachment(a))
}

// decodeVaultProtected mantém notas e anexos fora de strings imutáveis do
// encoding/json. Os RawMessage temporários também são apagados ao retornar.
// O decoder público de importação continua aceitando os campos de entrada.
func decodeVaultProtected(data []byte) (_ *Vault, err error) {
	type meta Vault
	var wire struct {
		meta
		Entries []json.RawMessage `json:"entries"`
	}
	defer func() {
		for _, raw := range wire.Entries {
			mycrypto.ZeroBytes(raw)
		}
		if err != nil {
			if wire.Conta != nil {
				wire.Conta.Close()
			}
			for i := range wire.meta.Entries {
				closeEntry(&wire.meta.Entries[i])
			}
		}
	}()
	if err = json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	for _, raw := range wire.Entries {
		entry, e := decodeEntryProtected(raw)
		if e != nil {
			return nil, e
		}
		wire.meta.Entries = append(wire.meta.Entries, entry)
	}
	v := Vault(wire.meta)
	return &v, nil
}

func decodeEntryProtected(data []byte) (_ SecretEntry, err error) {
	type meta SecretEntry
	var wire struct {
		meta
		Notes       json.RawMessage   `json:"notes"`
		Attachments []json.RawMessage `json:"attachments"`
	}
	defer func() {
		mycrypto.ZeroBytes(wire.Notes)
		for _, raw := range wire.Attachments {
			mycrypto.ZeroBytes(raw)
		}
		if err != nil {
			entry := SecretEntry(wire.meta)
			closeEntry(&entry)
		}
	}()
	if err = json.Unmarshal(data, &wire); err != nil {
		return SecretEntry{}, err
	}
	if len(wire.Notes) != 0 && !bytes.Equal(wire.Notes, []byte("null")) {
		plain, e := decodeValue(wire.Notes)
		if e != nil {
			return SecretEntry{}, e
		}
		if len(plain) != 0 {
			wire.notes, err = mycrypto.SealMemory(plain)
		}
		mycrypto.ZeroBytes(plain)
		if err != nil {
			return SecretEntry{}, err
		}
	}
	for _, raw := range wire.Attachments {
		attachment, e := decodeAttachmentProtected(raw)
		if e != nil {
			return SecretEntry{}, e
		}
		wire.meta.Attachments = append(wire.meta.Attachments, attachment)
	}
	return SecretEntry(wire.meta), nil
}

func decodeAttachmentProtected(data []byte) (_ Attachment, err error) {
	var wire struct {
		Filename string          `json:"filename"`
		Size     int64           `json:"size"`
		Data     json.RawMessage `json:"data"`
	}
	defer func() { mycrypto.ZeroBytes(wire.Data) }()
	if err = json.Unmarshal(data, &wire); err != nil {
		return Attachment{}, err
	}
	attachment := Attachment{Filename: wire.Filename, Size: wire.Size}
	if len(wire.Data) == 0 || bytes.Equal(wire.Data, []byte("null")) {
		return attachment, nil
	}
	encoded, err := decodeValue(wire.Data)
	if err != nil {
		return Attachment{}, errors.New("anexo inválido: esperado conteúdo base64")
	}
	defer mycrypto.ZeroBytes(encoded)
	plain := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)))
	defer mycrypto.ZeroBytes(plain)
	n, err := base64.StdEncoding.Decode(plain, encoded)
	if err != nil {
		return Attachment{}, err
	}
	attachment.Size = int64(n)
	attachment.sealed, err = mycrypto.SealMemory(plain[:n])
	return attachment, err
}
