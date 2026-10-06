package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
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

func (l *LocalStorage) Path() string { return l.filePath }

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

// Usado em download/restauração explícitos. Preserva o arquivo cifrado anterior
// antes de instalar outro conteúdo; falha no backup impede a substituição.
func (l *LocalStorage) SaveComBackup(ctx context.Context, data []byte) (string, error) {
	backup, err := l.PreservarBackup(ctx, data)
	if err != nil {
		return "", err
	}
	return backup, l.Save(ctx, data)
}

// Preserva o conteúdo anterior sem instalar o novo arquivo. Assim o chamador
// pode registrar a pendência de sincronização antes da substituição local.
func (l *LocalStorage) PreservarBackup(ctx context.Context, data []byte) (string, error) {
	atual, err := l.Load(ctx)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return "", err
	}
	backup := ""
	if err == nil && !bytes.Equal(atual, data) {
		backup = l.filePath + ".backup-" + rand.Text() + ".enc"
		if err := arquivo.Gravar(backup, atual, mycrypto.RestrictFilePermissions); err != nil {
			return "", fmt.Errorf("não foi possível preservar o cofre anterior: %w", err)
		}
	}
	return backup, nil
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
