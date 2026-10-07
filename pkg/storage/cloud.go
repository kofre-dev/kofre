package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"kofre/pkg/config"
)

// KofreCloudStorage implementa a sincronização gerenciada através da API oficial do Kofre Cloud
type KofreCloudStorage struct {
	mu                   sync.Mutex
	revisao              string
	revisaoConhecida     bool
	arquivoRevisao       string
	ultimaLeituraRevisao string
	errevisao            error
	endpoint             string
	token                string
	contaID              string
	httpClient           *http.Client
}

func NewKofreCloudStorage(endpoint, token string, contaID ...string) *KofreCloudStorage {
	if endpoint == "" {
		endpoint = config.GetCloudEndpoint()
	}
	endpoint = strings.TrimRight(endpoint, "/")
	c := &KofreCloudStorage{
		endpoint:   endpoint,
		token:      strings.TrimSpace(token),
		httpClient: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
	if len(contaID) > 0 {
		c.contaID = contaID[0]
	}
	return c
}

func (c *KofreCloudStorage) Load(ctx context.Context) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.carregar(ctx, true)
}

func (c *KofreCloudStorage) carregar(ctx context.Context, preservarBase bool) ([]byte, error) {
	if c.errevisao != nil {
		return nil, c.errevisao
	}
	url := fmt.Sprintf("%s/v1/vault", c.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar ao Kofre Cloud: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Uma exclusão remota não apaga a base de uma edição local existente.
		// O próximo PUT preserva If-Match e pede resolução explícita do conflito.
		if c.revisaoConhecida && c.revisao != "" {
			return nil, ErrNotFound
		}
		if err := c.guardarRevisao(""); err != nil {
			return nil, err
		}
		return nil, ErrNotFound
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("token de licença Kofre Cloud inválido ou expirado")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("erro do servidor Kofre Cloud (%d): %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 64*1024*1024 {
		return nil, fmt.Errorf("resposta da nuvem excede o limite de leitura")
	}
	if revisao := resp.Header.Get("ETag"); preservarBase && revisao != "" {
		if revisao != revisaoArquivoCloud(data) {
			return nil, fmt.Errorf("revisão não corresponde ao arquivo recebido")
		}
		// Nem mesmo uma leitura sem arquivo de revisão deve aprovar conteúdo
		// remoto antes da validação criptográfica feita pelo chamador.
		c.ultimaLeituraRevisao = revisao
	}
	return data, nil
}

func (c *KofreCloudStorage) Save(ctx context.Context, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.errevisao != nil {
		return c.errevisao
	}
	if !c.revisaoConhecida {
		atual, err := c.carregar(ctx, false)
		if err != nil && err != ErrNotFound {
			return err
		}
		if err == nil {
			if bytes.Equal(atual, data) {
				return c.guardarRevisao(revisaoArquivoCloud(data))
			}
			// Descobrir a revisão atual não prova que o arquivo local partiu dela.
			c.revisaoConhecida = false
			c.revisao = ""
			return fmt.Errorf("%w: há outro cofre na nuvem", ErrConflito)
		}
	}
	url := fmt.Sprintf("%s/v1/vault", c.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	req.Header.Set("Content-Type", "application/octet-stream")
	if c.revisao != "" {
		req.Header.Set("If-Match", c.revisao)
	} else {
		req.Header.Set("If-None-Match", "*")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("falha ao enviar dados para o Kofre Cloud: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("token de licença Kofre Cloud inválido ou expirado")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if resp.StatusCode == http.StatusConflict {
			// O PUT anterior pode ter sido aplicado e somente seu ACK local
			// falhado. Uma leitura de inspeção não adota outra base; só bytes
			// exatamente iguais aos enviados permitem concluir esse ACK.
			atual, leituraErr := c.carregar(ctx, false)
			if leituraErr == nil && bytes.Equal(atual, data) {
				return c.guardarRevisao(revisaoArquivoCloud(data))
			}
		}
		if resp.StatusCode == 409 || resp.StatusCode == 428 {
			return fmt.Errorf("%w: %s", ErrConflito, body)
		}
		return fmt.Errorf("erro ao salvar no Kofre Cloud (%d): %s", resp.StatusCode, string(body))
	}

	if revisao := resp.Header.Get("ETag"); revisao != "" {
		return c.guardarRevisao(revisao)
	}
	return nil
}

func (c *KofreCloudStorage) Exists(ctx context.Context) (bool, error) {
	_, err := c.Load(ctx)
	if err == nil {
		return true, nil
	}
	if strings.Contains(err.Error(), "404") || err == ErrNotFound {
		return false, nil
	}
	return false, err
}

func (c *KofreCloudStorage) Location() string {
	return fmt.Sprintf("cloud: %s", c.endpoint)
}
