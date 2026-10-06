package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	mycrypto "kofre/pkg/crypto"
)

var (
	MagicHeader       = []byte("KOFRE001") // 8 bytes identificadores
	LegacyMagicHeader = []byte("MYCOFRE1") // compatibilidade retroativa
	ContaMagicHeader  = []byte("KOFRE002") // Impede clientes antigos de descartar a identidade ao regravar.
	ErrBadMagic       = errors.New("arquivo de cofre invalido: cabecalho magico nao confere")
	ErrNotFound       = errors.New("credencial nao encontrada")
)

// ManagedVault gerencia o cofre em memoria com thread-safety
type ManagedVault struct {
	mu     sync.RWMutex
	data   *Vault
	salt   []byte
	dirty  bool
	closed bool
}

// NewManaged cria um cofre gerenciado novo
func NewManaged() *ManagedVault {
	return &ManagedVault{
		data: NewVault(),
	}
}

// Wrap cria um cofre gerenciado a partir de dados existentes e salt
func Wrap(v *Vault, salt []byte) (*ManagedVault, error) {
	mv := NewManaged()
	mv.salt = append([]byte(nil), salt...)
	mv.data.SchemaVersion, mv.data.CreatedAt, mv.data.UpdatedAt = v.SchemaVersion, v.CreatedAt, v.UpdatedAt
	if v.Conta != nil {
		if !v.Conta.Protected {
			return nil, errors.New("identidade da conta deve ser protegida")
		}
		if err := v.Conta.WithValue(mv.DefinirConta); err != nil {
			mv.Close()
			return nil, err
		}
		mv.dirty = false
	}
	for _, entry := range v.Entries {
		sealed, err := sealEntry(entry)
		if err != nil {
			mv.Close()
			return nil, err
		}
		mv.data.Entries = append(mv.data.Entries, sealed)
	}
	return mv, nil
}

// Close revoga inclusive os handles que já foram entregues para a interface.
func (mv *ManagedVault) Close() {
	mv.mu.Lock()
	defer mv.mu.Unlock()
	for i := range mv.data.Entries {
		closeEntry(&mv.data.Entries[i])
	}
	mv.data.Entries = nil
	if mv.data.Conta != nil {
		mv.data.Conta.Close()
		mv.data.Conta = nil
	}
	mv.closed = true
}

// Entries retorna uma copia de todas as entradas
func (mv *ManagedVault) Entries() []SecretEntry {
	mv.mu.RLock()
	defer mv.mu.RUnlock()

	entries := make([]SecretEntry, len(mv.data.Entries))
	for i, entry := range mv.data.Entries {
		entries[i] = cloneEntry(entry)
	}
	return entries
}

// GetEntry busca uma entrada pelo ID
func (mv *ManagedVault) GetEntry(id string) (SecretEntry, error) {
	mv.mu.RLock()
	defer mv.mu.RUnlock()

	for _, e := range mv.data.Entries {
		if e.ID == id {
			return cloneEntry(e), nil
		}
	}
	return SecretEntry{}, ErrNotFound
}

// AddEntry adiciona uma nova credencial ao cofre
func (mv *ManagedVault) AddEntry(entry SecretEntry) (SecretEntry, error) {
	mv.mu.Lock()
	defer mv.mu.Unlock()
	if mv.closed {
		return SecretEntry{}, errors.New("cofre encerrado")
	}
	var err error
	entry, err = sealEntry(entry)
	if err != nil {
		return SecretEntry{}, err
	}

	now := time.Now()
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	entry.CreatedAt = now
	entry.UpdatedAt = now
	entry.Version = 1

	mv.data.Entries = append(mv.data.Entries, entry)
	mv.data.UpdatedAt = now
	mv.dirty = true
	return cloneEntry(entry), nil
}

// UpdateEntry atualiza uma credencial existente
func (mv *ManagedVault) UpdateEntry(entry SecretEntry) error {
	mv.mu.Lock()
	defer mv.mu.Unlock()

	now := time.Now()
	for i, e := range mv.data.Entries {
		if e.ID == entry.ID {
			var err error
			entry, err = sealEntry(entry)
			if err != nil {
				return err
			}
			entry.CreatedAt = e.CreatedAt
			entry.UpdatedAt = now
			entry.Version = e.Version + 1
			closeEntry(&mv.data.Entries[i])
			mv.data.Entries[i] = entry
			mv.data.UpdatedAt = now
			mv.dirty = true
			return nil
		}
	}
	return ErrNotFound
}

// DeleteEntry remove uma credencial pelo ID
func (mv *ManagedVault) DeleteEntry(id string) error {
	mv.mu.Lock()
	defer mv.mu.Unlock()

	for i, e := range mv.data.Entries {
		if e.ID == id {
			closeEntry(&mv.data.Entries[i])
			copy(mv.data.Entries[i:], mv.data.Entries[i+1:])
			mv.data.Entries[len(mv.data.Entries)-1] = SecretEntry{}
			mv.data.Entries = mv.data.Entries[:len(mv.data.Entries)-1]
			mv.data.UpdatedAt = time.Now()
			mv.dirty = true
			return nil
		}
	}
	return ErrNotFound
}

// Search faz uma busca textual por titulo, campos e notas com filtro opcional de categoria
func (mv *ManagedVault) Search(query string, category Category) []SecretEntry {
	mv.mu.RLock()
	defer mv.mu.RUnlock()

	q := strings.ToLower(strings.TrimSpace(query))
	results := make([]SecretEntry, 0)

	for _, e := range mv.data.Entries {
		if category != "" && e.Category != category {
			continue
		}

		if q == "" {
			results = append(results, cloneEntry(e))
			continue
		}

		matched := false
		if strings.Contains(strings.ToLower(e.Title), q) ||
			strings.Contains(strings.ToLower(e.Notes), q) ||
			strings.Contains(strings.ToLower(string(e.Category)), q) {
			matched = true
		}

		if !matched {
			for _, f := range e.Fields {
				if strings.Contains(strings.ToLower(f.Name), q) ||
					(!f.Protected && strings.Contains(strings.ToLower(f.Value), q)) {
					matched = true
					break
				}
			}
		}

		if matched {
			results = append(results, cloneEntry(e))
		}
	}

	return results
}

// Count retorna a quantidade total de credenciais
func (mv *ManagedVault) Count() int {
	mv.mu.RLock()
	defer mv.mu.RUnlock()
	return len(mv.data.Entries)
}

// IsDirty informa se ha mudancas nao salvas
func (mv *ManagedVault) IsDirty() bool {
	mv.mu.RLock()
	defer mv.mu.RUnlock()
	return mv.dirty
}

// MarkClean redefine o status de alteracoes
func (mv *ManagedVault) MarkClean() {
	mv.mu.Lock()
	defer mv.mu.Unlock()
	mv.dirty = false
}

// Pack serializa e criptografa o cofre gerando o payload seguro em envelope:
// [8 bytes Magic Header][16 bytes Salt][Payload AES-256-GCM]
func (mv *ManagedVault) Pack(key, salt []byte) ([]byte, error) {
	mv.mu.RLock()
	defer mv.mu.RUnlock()

	if mv.closed {
		return nil, errors.New("cofre encerrado")
	}
	if len(salt) != mycrypto.SaltLength {
		return nil, errors.New("salt invalido")
	}
	jsonData, err := marshalVault(mv.data)
	if err != nil {
		return nil, fmt.Errorf("falha ao serializar cofre: %w", err)
	}
	defer mycrypto.ZeroBytes(jsonData)

	encrypted, err := mycrypto.Encrypt(jsonData, key)
	if err != nil {
		return nil, fmt.Errorf("falha ao criptografar cofre: %w", err)
	}

	var buf bytes.Buffer
	if mv.data.Conta != nil {
		buf.Write(ContaMagicHeader)
	} else {
		buf.Write(MagicHeader)
	}
	buf.Write(salt)
	buf.Write(encrypted)

	return buf.Bytes(), nil
}

// Unpack decodifica o cabecalho, extrai o salt e o payload encriptado
func UnpackHeader(raw []byte) (salt, encryptedPayload []byte, err error) {
	headerLen := 8
	minHeaderLen := headerLen + mycrypto.SaltLength
	if len(raw) < minHeaderLen {
		return nil, nil, errors.New("tamanho de arquivo insuficiente para cabecalho do cofre")
	}

	magic := raw[:headerLen]
	if !bytes.Equal(magic, MagicHeader) && !bytes.Equal(magic, LegacyMagicHeader) && !bytes.Equal(magic, ContaMagicHeader) {
		return nil, nil, ErrBadMagic
	}

	salt = raw[headerLen : headerLen+mycrypto.SaltLength]
	encryptedPayload = raw[headerLen+mycrypto.SaltLength:]
	return salt, encryptedPayload, nil
}

// DecryptAndLoad abre o cofre decifrando o payload com a chave informada
func DecryptAndLoad(encryptedPayload, key, salt []byte) (*ManagedVault, error) {
	plaintext, err := mycrypto.Decrypt(encryptedPayload, key)
	if err != nil {
		return nil, err
	}
	defer mycrypto.ZeroBytes(plaintext)

	var v Vault
	defer func() {
		if v.Conta != nil {
			v.Conta.Close()
		}
		for i := range v.Entries {
			closeEntry(&v.Entries[i])
		}
	}()
	if err := json.Unmarshal(plaintext, &v); err != nil {
		return nil, fmt.Errorf("falha ao desserializar conteudo do cofre: %w", err)
	}

	return Wrap(&v, salt)
}

// ToEnvMap converte as credenciais do cofre em um mapa de variaveis de ambiente
// Se filterTitle for especificado, exporta apenas as chaves daquela credencial especifica
func (mv *ManagedVault) ToEnvMap(filterTitle string) (map[string]string, error) {
	mv.mu.RLock()
	defer mv.mu.RUnlock()
	if mv.closed {
		return nil, errors.New("cofre encerrado")
	}

	envMap := make(map[string]string)
	filter := strings.ToLower(strings.TrimSpace(filterTitle))

	for _, entry := range mv.data.Entries {
		if filter != "" && !strings.Contains(strings.ToLower(entry.Title), filter) {
			continue
		}

		cleanTitle := SanitizeEnvKey(entry.Title)

		// Se a credencial tiver campos estruturados
		for _, f := range entry.Fields {
			fName := SanitizeEnvKey(f.Name)
			var value string
			if err := f.WithValue(func(raw []byte) error { value = string(raw); return nil }); err != nil {
				return nil, err
			}

			// Se o campo ja tem cara de ENV (ex: AWS_ACCESS_KEY_ID ou OPENAI_API_KEY)
			if strings.Contains(fName, "_") || strings.ToUpper(f.Name) == f.Name {
				envMap[fName] = value
				continue
			}

			// Se for um campo generico ("token", "senha", "key") de uma credencial
			if f.Protected || strings.EqualFold(f.Name, "token") || strings.EqualFold(f.Name, "secret") || strings.EqualFold(f.Name, "senha") {
				envMap[cleanTitle] = value
			} else {
				// Combina Titulo + Nome do Campo (ex: BANCO_PROD_USUARIO)
				combinedKey := cleanTitle + "_" + fName
				envMap[combinedKey] = value
			}
		}

		// Se nao tiver campos, mas tiver titulo formato ENV
		if len(entry.Fields) == 0 && cleanTitle != "" {
			envMap[cleanTitle] = entry.Notes
		}
	}

	return envMap, nil
}

// SanitizeEnvKey normaliza um texto para o padrao de variavel de ambiente (UPPERCASE_WITH_UNDERSCORES)
func SanitizeEnvKey(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '.' || r == '/' || r == ':' {
			b.WriteRune('_')
		}
	}
	res := strings.ToUpper(b.String())
	// Remove underscores duplicados
	for strings.Contains(res, "__") {
		res = strings.ReplaceAll(res, "__", "_")
	}
	return strings.Trim(res, "_")
}
