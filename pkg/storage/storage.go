package storage

import (
	"context"
	"errors"
)

var (
	ErrNotFound = errors.New("arquivo de cofre nao encontrado no storage")
	ErrConflito = errors.New("conflito de sincronização; confira as cópias local e remota")
)

// StorageProvider define a interface abstrata para persistencia do cofre criptografado
type StorageProvider interface {
	// Load recupera os bytes criptografados do cofre
	Load(ctx context.Context) ([]byte, error)

	// Save grava os bytes criptografados do cofre
	Save(ctx context.Context, data []byte) error

	// Exists verifica se o cofre ja existe
	Exists(ctx context.Context) (bool, error)

	// Location retorna uma descricao legivel da localizacao (ex: "local: ./vault.enc" ou "s3://meu-bucket/vault.enc")
	Location() string
}

// ConfirmarCarga deve ser chamado após validar formato e autenticação
// criptográfica dos bytes retornados por Load. Provedores locais não promovem
// candidatos; SyncStorage só substitui o arquivo nesta confirmação explícita.
func ConfirmarCarga(ctx context.Context, provider StorageProvider, data []byte) error {
	if confirmavel, ok := provider.(interface {
		ConfirmarCarga(context.Context, []byte) error
	}); ok {
		return confirmavel.ConfirmarCarga(ctx, data)
	}
	return nil
}
