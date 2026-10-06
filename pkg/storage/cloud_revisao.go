package storage

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"kofre/internal/arquivo"
	"os"
	"regexp"
)

var revisaoCloudValida = regexp.MustCompile(`^"[a-f0-9]{64}"$`)

type estadoRevisaoCloud struct {
	Destino string `json:"destino"`
	Revisao string `json:"revisao"`
}

// O arquivo é vinculado ao cofre local e ao destino. Não contém o token nem
// segredos do cofre; preserva a base da edição durante reinícios e uso offline.
func (c *KofreCloudStorage) DefinirArquivoRevisao(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.arquivoRevisao = path
	c.revisao, c.revisaoConhecida, c.errevisao = "", false, nil
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err == nil {
		var estado estadoRevisaoCloud
		if json.Unmarshal(b, &estado) != nil || estado.Destino != syncDestination(c) || estado.Revisao != "" && !revisaoCloudValida.MatchString(estado.Revisao) {
			err = fmt.Errorf("estado de sincronização inválido ou pertencente a outra conta; arquivo preservado")
		} else {
			c.revisao, c.revisaoConhecida = estado.Revisao, true
		}
	}
	c.errevisao = err
	return err
}

func revisaoArquivoCloud(data []byte) string { return fmt.Sprintf(`"%x"`, sha256.Sum256(data)) }

// Chamar somente depois de instalar a cópia recebida no armazenamento local.
// Uma leitura de inspeção ou download que falhou no disco não muda a base editada.
func (c *KofreCloudStorage) ConfirmarLeitura(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ultimaLeituraRevisao == "" {
		return nil
	}
	if c.ultimaLeituraRevisao != revisaoArquivoCloud(data) {
		return fmt.Errorf("cópia local diverge da última leitura")
	}
	if err := c.guardarRevisao(c.ultimaLeituraRevisao); err != nil {
		return err
	}
	c.ultimaLeituraRevisao = ""
	return nil
}

func (c *KofreCloudStorage) coincideComBase(data []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.errevisao == nil && c.revisaoConhecida && c.revisao == revisaoArquivoCloud(data)
}

func (c *KofreCloudStorage) guardarRevisao(revisao string) error {
	if revisao != "" && !revisaoCloudValida.MatchString(revisao) {
		return fmt.Errorf("revisão inválida recebida da nuvem")
	}
	if c.arquivoRevisao != "" {
		b, err := json.Marshal(estadoRevisaoCloud{syncDestination(c), revisao})
		if err != nil {
			return err
		}
		if err = arquivo.Gravar(c.arquivoRevisao, b, nil); err != nil {
			return fmt.Errorf("não foi possível preservar a revisão: %w", err)
		}
	}
	c.revisao, c.revisaoConhecida = revisao, true
	return nil
}
