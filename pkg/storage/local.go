package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	dir := filepath.Dir(l.filePath)
	tmpFile, err := os.CreateTemp(dir, "Kofre-*.tmp")
	if err != nil {
		return fmt.Errorf("falha ao criar arquivo temporario para gravacao: %w", err)
	}
	tmpName := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpName)
		return fmt.Errorf("falha ao escrever no arquivo temporario: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpName)
		return fmt.Errorf("falha ao sincronizar arquivo com disco: %w", err)
	}
	tmpFile.Close()

	// Substituicao atomica (ou sobrescrita segura no Windows)
	if err := os.Rename(tmpName, l.filePath); err != nil {
		// No Windows se o arquivo de destino ja existir o rename simples pode falhar, usamos remove antes
		_ = os.Remove(l.filePath)
		if err := os.Rename(tmpName, l.filePath); err != nil {
			os.Remove(tmpName)
			return fmt.Errorf("falha ao substituir arquivo do cofre: %w", err)
		}
	}

	return nil
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
