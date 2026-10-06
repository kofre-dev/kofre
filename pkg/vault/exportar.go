package vault

import "errors"

// ExportarEntrada é uma ação explícita para compartilhar um item. Retorna um
// buffer temporário serializado pelo mesmo caminho seguro usado no cofre, sem
// entregar strings de campos protegidos ao encoding/json. O chamador deve apagá-lo.
func (mv *ManagedVault) ExportarEntrada(id string) ([]byte, error) {
	mv.mu.RLock()
	defer mv.mu.RUnlock()
	if mv.closed {
		return nil, errors.New("cofre fechado")
	}
	for _, entry := range mv.data.Entries {
		if entry.ID == id {
			return marshalVault(&Vault{SchemaVersion: mv.data.SchemaVersion, CreatedAt: mv.data.CreatedAt, UpdatedAt: mv.data.UpdatedAt, Entries: []SecretEntry{entry}})
		}
	}
	return nil, errors.New("entrada pessoal não encontrada")
}
