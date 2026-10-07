package arquivo

import (
	"context"
	"fmt"
	"os"
	"time"
)

// ComExclusao serializa escritores cooperantes, inclusive de outros processos.
// O arquivo de lock fica estável: removê-lo criaria locks em arquivos distintos.
func ComExclusao(ctx context.Context, path string, executar func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("não foi possível abrir controle de escrita: %w", err)
	}
	defer f.Close()
	for {
		obtido, err := tentarExclusao(f)
		if err != nil {
			return fmt.Errorf("não foi possível bloquear escrita: %w", err)
		}
		if obtido {
			if err := ctx.Err(); err != nil {
				return err
			}
			return executar()
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
