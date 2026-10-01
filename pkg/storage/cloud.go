package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// KofreCloudStorage implementa a sincronização gerenciada através da API oficial do Kofre Cloud
type KofreCloudStorage struct {
	endpoint   string
	token      string
	httpClient *http.Client
}

func NewKofreCloudStorage(endpoint, token string) *KofreCloudStorage {
	if endpoint == "" {
		endpoint = "https://api.kofre.dev"
	}
	endpoint = strings.TrimRight(endpoint, "/")
	return &KofreCloudStorage{
		endpoint:   endpoint,
		token:      strings.TrimSpace(token),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *KofreCloudStorage) Load(ctx context.Context) ([]byte, error) {
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
		return nil, ErrNotFound
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("token de licença Kofre Cloud inválido ou expirado")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("erro do servidor Kofre Cloud (%d): %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

func (c *KofreCloudStorage) Save(ctx context.Context, data []byte) error {
	url := fmt.Sprintf("%s/v1/vault", c.endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("falha ao enviar dados para o Kofre Cloud: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("token de licença Kofre Cloud inválido ou expirado")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("erro ao salvar no Kofre Cloud (%d): %s", resp.StatusCode, string(body))
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
