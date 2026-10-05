package storage

import (
	"context"
	"fmt"
	"kofre/internal/arquivo"
	"os"
	"path/filepath"

	mycrypto "kofre/pkg/crypto"
)

// LocalStorage implementa o armazenamento em arquivo local
type LocalStorage struct {
	filePath string
}

// NewLocalStorage inicializa o storage local apontando para o caminho indicado
func NewLocalStorage(path string) (*LocalStorage, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("caminho de arquivo invalido: %w", err)
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("falha ao criar diretorio para o cofre: %w", err)
	}

	return &LocalStorage{filePath: absPath}, nil
}

func (l *LocalStorage) Load(ctx context.Context) ([]byte, error) {
	if _, err := os.Stat(l.filePath); os.IsNotExist(err) {
		return nil, ErrNotFound
	}

	data, err := os.ReadFile(l.filePath)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler arquivo do cofre: %w", err)
	}

	return data, nil
}

func (l *LocalStorage) Save(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return arquivo.Gravar(l.filePath, data, mycrypto.RestrictFilePermissions)
}

func (l *LocalStorage) Exists(ctx context.Context) (bool, error) {
	_, err := os.Stat(l.filePath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (l *LocalStorage) Location() string {
	return fmt.Sprintf("local: %s", l.filePath)
}
