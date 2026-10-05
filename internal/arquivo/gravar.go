package arquivo

import (
	"fmt"
	"os"
	"path/filepath"
)

// Gravar prepara e sincroniza o temporário antes de substituir o destino.
// Falhas de preparação ou substituição preservam o arquivo anterior.
func Gravar(path string, data []byte, proteger func(string) error) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".kofre-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	defer f.Close()
	if proteger != nil {
		if err = proteger(name); err != nil {
			return fmt.Errorf("falha ao proteger temporário: %w", err)
		}
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = substituir(name, path); err != nil {
		return fmt.Errorf("falha ao substituir arquivo: %w", err)
	}
	return nil
}
