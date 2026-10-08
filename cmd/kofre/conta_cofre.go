package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"kofre/pkg/corporativo"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/storage"
	"kofre/pkg/vault"
	"os"
	"path/filepath"
	"time"
)

// Só existe durante uma ação de conta. A senha não é retida nem enviada ao Cloud.
var contaEmUso *sessaoConta

type sessaoConta struct {
	cofre               *vault.ManagedVault
	chave               *mycrypto.SealedBuffer
	salt, base          []byte
	arquivo, identidade string
}

func abrirSessaoConta(identidade string) (*sessaoConta, error) {
	path := resolveVaultPath("")
	base, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("abra ou crie seu cofre antes de configurar a conta: %w", err)
	}
	v, key, salt, err := abrirCofreConta(path)
	if err != nil {
		return nil, err
	}
	defer mycrypto.ZeroBytes(key)
	atual, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(base, atual) {
		v.Close()
		return nil, errors.New("cofre alterado durante a abertura; tente novamente")
	}
	sealed, err := mycrypto.SealMemory(key)
	if err != nil {
		v.Close()
		return nil, err
	}
	return &sessaoConta{cofre: v, chave: sealed, salt: salt, base: base, arquivo: path, identidade: identidade}, nil
}

func (s *sessaoConta) fechar() { s.cofre.Close(); s.chave.Close() }

func abrirIdentidadeConta(path string, password []byte) (*corporativo.Identidade, error) {
	if contaEmUso == nil || path != contaEmUso.identidade {
		return corporativo.AbrirIdentidade(path, password)
	}
	if !contaEmUso.cofre.TemConta() {
		return nil, os.ErrNotExist
	}
	var i corporativo.Identidade
	err := contaEmUso.cofre.LerConta(func(data []byte) error { return json.Unmarshal(data, &i) })
	if err != nil || len(i.Privada) != 32 {
		i.Fechar()
		return nil, errors.New("identidade no cofre inválida")
	}
	if i.Pins == nil {
		i.Pins = map[string]string{}
	}
	if err = i.ProtegerPrivada(); err != nil {
		i.Fechar()
		return nil, err
	}
	return &i, nil
}

func salvarIdentidadeConta(path string, i *corporativo.Identidade, password []byte) error {
	if contaEmUso == nil {
		return corporativo.SalvarIdentidade(path, i, password)
	}
	s := contaEmUso
	if path != s.identidade {
		return exportarIdentidadeConta(path, i, password)
	}
	return s.salvar(i)
}

func exportarIdentidadeConta(path string, i *corporativo.Identidade, password []byte) error {
	if contaEmUso == nil {
		return corporativo.SalvarIdentidade(path, i, password)
	}
	s := contaEmUso
	// Exportação explícita, sem sobrescrever o cofre nem um arquivo existente.
	dest, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	orig, err := filepath.Abs(s.arquivo)
	if err != nil {
		return err
	}
	if dest == orig {
		return errors.New("o backup da conta não pode substituir seu cofre")
	}
	if _, err = os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return errors.New("escolha um arquivo novo para o backup da conta")
	}
	return s.chave.WithBytes(func(key []byte) error { return corporativo.SalvarIdentidadeComChave(path, i, key, s.salt) })
}

func (s *sessaoConta) salvar(i *corporativo.Identidade) error {
	atual, err := os.ReadFile(s.arquivo)
	if err != nil {
		return err
	}
	if !bytes.Equal(atual, s.base) {
		return errors.New("cofre alterado por outro processo; reabra a conta antes de continuar")
	}
	data, err := i.Serializar()
	if err != nil {
		return err
	}
	defer mycrypto.ZeroBytes(data)
	if err = s.cofre.DefinirConta(data); err != nil {
		return err
	}
	var packed []byte
	err = s.chave.WithBytes(func(key []byte) error { var e error; packed, e = s.cofre.Pack(key, s.salt); return e })
	if err != nil {
		return err
	}
	// LocalStorage preserva a base legada ao migrar para KOFRE003; gravações
	// posteriores não precisam gerar um novo backup de migração.
	store, err := resolveActiveStorageProvider(s.arquivo, "", "", "", "")
	if err != nil {
		return err
	}
	if syncer, ok := store.(*storage.SyncStorage); ok {
		defer syncer.Close()
	}
	if err = store.Save(context.Background(), packed); err != nil {
		return err
	}
	s.base = packed
	s.cofre.MarkClean()
	if syncer, ok := store.(*storage.SyncStorage); ok {
		if err = syncer.Flush(3 * time.Second); err != nil {
			informarConta("Conta salva no cofre local; sincronização pendente.")
		}
	}
	return nil
}

func migrarContaAnterior(path string) error {
	if contaEmUso.cofre.TemConta() {
		return nil
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	informarConta("Vamos vincular sua conta existente à senha mestra. O arquivo anterior será preservado como backup.")
	senha, err := segredoEmpresa("Senha antiga da conta (somente nesta migração)")
	if err != nil {
		return err
	}
	defer mycrypto.ZeroBytes(senha)
	return migrarIdentidadeProtegida(path, senha)
}

func migrarIdentidadeProtegida(path string, senha []byte) error {
	i, err := corporativo.AbrirIdentidade(path, senha)
	if err != nil {
		return err
	}
	defer i.Fechar()
	return salvarIdentidadeConta(path, i, nil)
}
