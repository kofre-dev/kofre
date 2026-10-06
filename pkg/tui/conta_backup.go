package tui

import (
	"context"
	"encoding/json"
	"errors"
	"kofre/pkg/config"
	"kofre/pkg/conta"
	"kofre/pkg/corporativo"
)

func (m *Model) identidadeConta() (*corporativo.Identidade, error) {
	var i corporativo.Identidade
	if err := m.vault.LerConta(func(data []byte) error { return json.Unmarshal(data, &i) }); err != nil {
		return nil, err
	}
	if err := i.ProtegerPrivada(); err != nil {
		i.Fechar()
		return nil, err
	}
	return &i, nil
}
func (m *Model) prepararBackupNovaSenha(key, salt []byte) ([]byte, error) {
	if !m.vault.TemConta() {
		return nil, nil
	}
	var meta struct {
		Email string `json:"email"`
	}
	if err := m.vault.LerConta(func(b []byte) error { return json.Unmarshal(b, &meta) }); err != nil {
		return nil, err
	}
	if meta.Email == "" {
		return nil, nil
	}
	i, err := m.identidadeConta()
	if err != nil {
		return nil, err
	}
	defer i.Fechar()
	if i.Email == "" {
		return nil, nil
	}
	if len(m.vault.BackupContaPendente()) != 0 {
		return nil, errors.New("conclua o backup pendente da conta antes de trocar a senha novamente")
	}
	cifra, err := corporativo.CifrarIdentidadeComChave(i, key, salt)
	if err != nil {
		return nil, err
	}
	c, err := conta.NovoClient(config.GetCloudEndpoint())
	if err != nil {
		return nil, err
	}
	payload, err := c.PrepararBackup(context.Background(), i, cifra)
	if err != nil {
		return nil, errors.New("conecte sua conta por e-mail antes de alterar a senha; o cofre foi preservado")
	}
	return payload, nil
}
func (m *Model) enviarBackupContaPendente() error {
	payload := m.vault.BackupContaPendente()
	if len(payload) == 0 {
		return nil
	}
	i, err := m.identidadeConta()
	if err != nil {
		return err
	}
	defer i.Fechar()
	c, err := conta.NovoClient(config.GetCloudEndpoint())
	if err != nil {
		return err
	}
	if err = c.EnviarBackupPendente(context.Background(), i, payload); err != nil {
		return err
	}
	m.vault.DefinirBackupContaPendente(nil)
	packed, err := m.pack()
	if err == nil {
		err = m.storage.Save(context.Background(), packed)
	}
	if err != nil {
		m.vault.DefinirBackupContaPendente(payload)
		return err
	}
	m.vault.MarkClean()
	return nil
}
