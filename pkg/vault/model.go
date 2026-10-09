package vault

import (
	mycrypto "kofre/pkg/crypto"
	"time"
)

type Category string

const (
	CategoryPassword    Category = "password"
	CategoryToken       Category = "token"
	CategoryCertificate Category = "certificate"
	CategorySSHKey      Category = "ssh_key"
	CategoryAuth        Category = "auth"
	CategoryNote        Category = "note"
	CategoryDatabase    Category = "database"
)

var AllCategories = []Category{
	CategoryPassword,
	CategoryToken,
	CategoryCertificate,
	CategorySSHKey,
	CategoryAuth,
	CategoryNote,
	CategoryDatabase,
}

// Field representa um par chave/valor sensivel dentro de uma credencial
type Field struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Protected bool   `json:"protected"` // se true, mascara na interface com ••••••
	sealed    *mycrypto.SealedBuffer
}

// Attachment permite armazenar certificados (.pem, .pfx, .crt) ou chaves privadas
type Attachment struct {
	Filename string `json:"filename"`
	Data     []byte `json:"data"`
	Size     int64  `json:"size"`
	sealed   *mycrypto.SealedBuffer
}

// SecretEntry representa uma credencial ou segredo individual
type SecretEntry struct {
	ID             string       `json:"id"`
	Title          string       `json:"title"`
	Category       Category     `json:"category"`
	Fields         []Field      `json:"fields"`
	Notes          string       `json:"notes"`
	Attachments    []Attachment `json:"attachments,omitempty"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	Version        int          `json:"version"` // contador de alteracoes locais
	notes          *mycrypto.SealedBuffer
	notesTemporary bool
}

// Vault representa a colecao completa de segredos descriptografados em memoria
type Vault struct {
	SchemaVersion       int           `json:"schema_version"`
	CreatedAt           time.Time     `json:"created_at"`
	UpdatedAt           time.Time     `json:"updated_at"`
	Entries             []SecretEntry `json:"entries"`
	Conta               *Field        `json:"conta,omitempty"`
	ContaBackupPendente []byte        `json:"conta_backup_pendente,omitempty"` // somente cifra, assinatura e revisão
}

// NewVault cria uma nova instancia vazia do cofre
func NewVault() *Vault {
	now := time.Now()
	return &Vault{
		SchemaVersion: 1,
		CreatedAt:     now,
		UpdatedAt:     now,
		Entries:       make([]SecretEntry, 0),
	}
}
