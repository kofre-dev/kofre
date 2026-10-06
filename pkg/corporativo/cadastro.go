package corporativo

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
)

// PrepararCadastro não faz requisições. Salve a identidade cifrada antes de
// Cadastrar; se a resposta se perder, o mesmo arquivo permite repetir o cadastro.
func PrepararCadastro(nome string) (*Identidade, error) {
	nome = strings.TrimSpace(nome)
	if nome == "" || len(nome) > 120 || strings.IndexFunc(nome, unicode.IsControl) >= 0 {
		return nil, fmt.Errorf("informe um nome com até 120 bytes, sem controles")
	}
	i, err := NovaIdentidade()
	if err != nil {
		return nil, err
	}
	var segredos [64]byte
	if _, err = rand.Read(segredos[:]); err != nil {
		i.Fechar()
		return nil, err
	}
	defer clear(segredos[:])
	i.Nome = nome
	i.Token = "kfr_conta_" + hex.EncodeToString(segredos[:32])
	i.Recuperacao = hex.EncodeToString(segredos[32:])
	return i, nil
}

func (c *Client) Cadastrar(ctx context.Context) error {
	i := c.Identidade
	if i == nil || i.Nome == "" || !strings.HasPrefix(i.Token, "kfr_conta_") {
		return fmt.Errorf("prepare e salve sua identidade antes de cadastrar")
	}
	publica, err := i.Publica()
	if err != nil {
		return err
	}
	assinatura, err := i.PublicaAssinatura()
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(i.Recuperacao))
	var resposta struct {
		ID   string `json:"id"`
		Nome string `json:"nome"`
	}
	err = c.Request(ctx, "POST", "/v1/corporativo/identidades", map[string]string{
		"nome": i.Nome, "recuperacao_hash": hex.EncodeToString(hash[:]),
		"chave_publica": publica, "chave_assinatura": assinatura,
	}, &resposta)
	if err != nil {
		return err
	}
	if !IDValido(resposta.ID) || resposta.Nome != i.Nome || i.ID != "" && i.ID != resposta.ID {
		return fmt.Errorf("resposta de cadastro inconsistente")
	}
	i.ID = resposta.ID
	return nil
}
