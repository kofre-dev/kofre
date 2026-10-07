package vault

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	mycrypto "kofre/pkg/crypto"
	"time"
)

const (
	vaultIDLength        = 16
	envelopePrefixLength = 8 + mycrypto.SaltLength + vaultIDLength
	wrappedKeyLength     = mycrypto.NonceLength + mycrypto.KeyLength + 16
)

// O catálogo cifrado contém somente metadados e outras cifras. Cada detalhe
// possui nonce e tag próprios. A identidade de conta recebe propósito separado.
type vaultCatalog struct {
	SchemaVersion       int            `json:"schema_version"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	Entries             []catalogEntry `json:"entries"`
	Conta               []byte         `json:"conta,omitempty"`
	ContaBackupPendente []byte         `json:"conta_backup_pendente,omitempty"`
}

type catalogEntry struct {
	ID         string `json:"id"`
	Ciphertext []byte `json:"ciphertext"`
}

func envelopeContext(prefix []byte, purpose, id string) []byte {
	context := make([]byte, 0, len(prefix)+len(purpose)+len(id)+2)
	context = append(context, prefix...)
	context = append(context, 0)
	context = append(context, purpose...)
	context = append(context, 0)
	return append(context, id...)
}

func (mv *ManagedVault) initializeDataKey() error {
	if mv.dataKey != nil {
		return nil
	}
	return mv.rotateDataKey()
}

// RotacionarChaveDados deve acompanhar a troca explícita da senha. Uma DEK
// extraída de um backup antigo não poderá abrir os detalhes gravados depois.
// As novas chaves são preparadas antes da revogação das anteriores.
func (mv *ManagedVault) RotacionarChaveDados() error {
	mv.mu.Lock()
	defer mv.mu.Unlock()
	if mv.closed {
		return errors.New("cofre encerrado")
	}
	if err := mv.rotateDataKey(); err != nil {
		return err
	}
	mv.dirty = true
	return nil
}

func (mv *ManagedVault) rotateDataKey() error {
	key := make([]byte, mycrypto.KeyLength)
	defer mycrypto.ZeroBytes(key)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	id := make([]byte, vaultIDLength)
	if _, err := rand.Read(id); err != nil {
		return err
	}
	sealed, err := mycrypto.SealMemory(key)
	if err != nil {
		return err
	}
	previous := mv.dataKey
	mv.dataKey, mv.vaultID = sealed, id
	previous.Close()
	return nil
}

func (mv *ManagedVault) packEnvelope(key, salt []byte) (result []byte, err error) {
	if mv.data.SchemaVersion != 1 {
		return nil, errors.New("versão do catálogo não suportada; atualize o Kofre")
	}
	if len(key) != mycrypto.KeyLength {
		return nil, errors.New("chave de cofre inválida")
	}
	if err := mv.initializeDataKey(); err != nil {
		return nil, err
	}
	prefix := append(bytes.Clone(EnvelopeMagicHeader), salt...)
	prefix = append(prefix, mv.vaultID...)
	err = mv.dataKey.WithBytes(func(dataKey []byte) error {
		wrapped, err := mycrypto.EncryptWithAAD(dataKey, key, envelopeContext(prefix, "chave", ""))
		if err != nil {
			return err
		}
		catalog := vaultCatalog{
			SchemaVersion: mv.data.SchemaVersion, CreatedAt: mv.data.CreatedAt,
			UpdatedAt: mv.data.UpdatedAt, ContaBackupPendente: mv.data.ContaBackupPendente,
		}
		ids := make(map[string]bool, len(mv.data.Entries))
		for _, entry := range mv.data.Entries {
			if entry.ID == "" || ids[entry.ID] {
				return errors.New("identificador de item vazio ou duplicado")
			}
			ids[entry.ID] = true
			ciphertext, err := encryptEntry(entry, dataKey, envelopeContext(prefix, "item", entry.ID))
			if err != nil {
				return err
			}
			catalog.Entries = append(catalog.Entries, catalogEntry{ID: entry.ID, Ciphertext: ciphertext})
		}
		if mv.data.Conta != nil {
			err := mv.data.Conta.WithValue(func(value []byte) error {
				var err error
				catalog.Conta, err = mycrypto.EncryptWithAAD(value, dataKey, envelopeContext(prefix, "conta", ""))
				return err
			})
			if err != nil {
				return err
			}
		}
		// json.Marshal recebe apenas cifras e metadados, jamais senhas/notas.
		encoded, err := json.Marshal(catalog)
		if err != nil {
			return err
		}
		defer mycrypto.ZeroBytes(encoded)
		encrypted, err := mycrypto.EncryptWithAAD(encoded, dataKey, envelopeContext(prefix, "catalogo", ""))
		if err != nil {
			return err
		}
		result = make([]byte, 0, len(prefix)+len(wrapped)+len(encrypted))
		result = append(result, prefix...)
		result = append(result, wrapped...)
		result = append(result, encrypted...)
		return nil
	})
	return result, err
}

func encryptEntry(entry SecretEntry, key, context []byte) ([]byte, error) {
	plain, err := marshalVault(&Vault{Entries: []SecretEntry{entry}})
	if err != nil {
		return nil, err
	}
	defer mycrypto.ZeroBytes(plain)
	return mycrypto.EncryptWithAAD(plain, key, context)
}

func loadEnvelope(raw, key, salt []byte) (_ *ManagedVault, err error) {
	if len(raw) < envelopePrefixLength+wrappedKeyLength+mycrypto.NonceLength+16 ||
		!bytes.HasPrefix(raw, EnvelopeMagicHeader) || len(salt) != mycrypto.SaltLength ||
		!bytes.Equal(raw[8:8+mycrypto.SaltLength], salt) || len(key) != mycrypto.KeyLength {
		return nil, mycrypto.ErrInvalidPayload
	}
	prefix := raw[:envelopePrefixLength]
	dataKey, err := mycrypto.DecryptWithAAD(raw[envelopePrefixLength:envelopePrefixLength+wrappedKeyLength], key, envelopeContext(prefix, "chave", ""))
	if err != nil {
		return nil, err
	}
	defer mycrypto.ZeroBytes(dataKey)
	if len(dataKey) != mycrypto.KeyLength {
		return nil, mycrypto.ErrInvalidPayload
	}
	encoded, err := mycrypto.DecryptWithAAD(raw[envelopePrefixLength+wrappedKeyLength:], dataKey, envelopeContext(prefix, "catalogo", ""))
	if err != nil {
		return nil, err
	}
	defer mycrypto.ZeroBytes(encoded)
	var catalog vaultCatalog
	if err := json.Unmarshal(encoded, &catalog); err != nil {
		return nil, fmt.Errorf("catálogo inválido: %w", err)
	}
	if catalog.SchemaVersion != 1 {
		return nil, errors.New("versão do catálogo não suportada; atualize o Kofre")
	}
	v := NewManaged()
	defer func() {
		if err != nil {
			v.Close()
		}
	}()
	v.salt = bytes.Clone(salt)
	v.vaultID = bytes.Clone(prefix[8+mycrypto.SaltLength:])
	v.dataKey, err = mycrypto.SealMemory(dataKey)
	if err != nil {
		return nil, err
	}
	v.data.SchemaVersion, v.data.CreatedAt, v.data.UpdatedAt = catalog.SchemaVersion, catalog.CreatedAt, catalog.UpdatedAt
	v.data.ContaBackupPendente = bytes.Clone(catalog.ContaBackupPendente)
	ids := make(map[string]bool, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		if entry.ID == "" || ids[entry.ID] {
			return nil, errors.New("identificador de item vazio ou duplicado")
		}
		ids[entry.ID] = true
		decoded, e := decryptEntry(entry, dataKey, envelopeContext(prefix, "item", entry.ID))
		if e != nil {
			return nil, e
		}
		v.data.Entries = append(v.data.Entries, decoded)
	}
	if len(catalog.Conta) != 0 {
		err = loadAccount(v, catalog.Conta, dataKey, envelopeContext(prefix, "conta", ""))
		if err != nil {
			return nil, err
		}
	}
	v.dirty = false
	return v, nil
}

func decryptEntry(record catalogEntry, key, context []byte) (SecretEntry, error) {
	plain, err := mycrypto.DecryptWithAAD(record.Ciphertext, key, context)
	if err != nil {
		return SecretEntry{}, err
	}
	defer mycrypto.ZeroBytes(plain)
	v, err := decodeVaultProtected(plain)
	if err != nil {
		return SecretEntry{}, err
	}
	if v.Conta != nil || len(v.ContaBackupPendente) != 0 || len(v.Entries) != 1 || v.Entries[0].ID != record.ID {
		if v.Conta != nil {
			v.Conta.Close()
		}
		for i := range v.Entries {
			closeEntry(&v.Entries[i])
		}
		return SecretEntry{}, errors.New("detalhes de item não correspondem ao catálogo")
	}
	return v.Entries[0], nil
}

func loadAccount(v *ManagedVault, ciphertext, key, context []byte) error {
	plain, err := mycrypto.DecryptWithAAD(ciphertext, key, context)
	if err != nil {
		return err
	}
	defer mycrypto.ZeroBytes(plain)
	return v.DefinirConta(plain)
}
