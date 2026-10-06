package vault

import "errors"

// A identidade não participa da lista, busca, exportação de itens nem do ambiente.
func (mv *ManagedVault) TemConta() bool {
	mv.mu.RLock()
	defer mv.mu.RUnlock()
	return !mv.closed && mv.data.Conta != nil
}

func (mv *ManagedVault) LerConta(use func([]byte) error) error {
	mv.mu.RLock()
	defer mv.mu.RUnlock()
	if mv.closed || mv.data.Conta == nil {
		return ErrNotFound
	}
	return mv.data.Conta.WithValue(use)
}

func (mv *ManagedVault) DefinirConta(data []byte) error {
	mv.mu.Lock()
	defer mv.mu.Unlock()
	if mv.closed {
		return errors.New("cofre encerrado")
	}
	if len(data) == 0 || len(data) > 4*1024*1024-128 {
		return errors.New("identidade inválida ou muito grande")
	}
	f, err := NewProtectedField("Identidade da conta", data)
	if err != nil {
		return err
	}
	if mv.data.Conta != nil {
		mv.data.Conta.Close()
	}
	mv.data.Conta = &f
	mv.dirty = true
	return nil
}

func (mv *ManagedVault) BackupContaPendente() []byte {
	mv.mu.RLock()
	defer mv.mu.RUnlock()
	if mv.closed {
		return nil
	}
	return append([]byte(nil), mv.data.ContaBackupPendente...)
}
func (mv *ManagedVault) DefinirBackupContaPendente(data []byte) {
	mv.mu.Lock()
	defer mv.mu.Unlock()
	if !mv.closed {
		mv.data.ContaBackupPendente = append([]byte(nil), data...)
		mv.dirty = true
	}
}
