package storage

import (
	"context"
	"errors"
)

var (
	ErrNotFound = errors.New("arquivo de cofre nao encontrado no storage")
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
