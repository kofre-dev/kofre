package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

type VersaoHistorico struct {
	ID       string    `json:"id"`
	CriadaEm time.Time `json:"criada_em"`
}

func (c *KofreCloudStorage) lerHistorico(ctx context.Context, sufixo string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.endpoint+"/v1/vault/historico"+sufixo, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return nil, fmt.Errorf("histórico indisponível (HTTP %d): %s", res.StatusCode, body)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 64*1024*1024+1))
	if len(data) > 64*1024*1024 {
		return nil, fmt.Errorf("resposta do histórico excede o limite de leitura")
	}
	return data, err
}

func (c *KofreCloudStorage) Historico(ctx context.Context) ([]VersaoHistorico, error) {
	data, err := c.lerHistorico(ctx, "")
	if err != nil {
		return nil, err
	}
	var resposta struct {
		Versoes []VersaoHistorico `json:"versoes"`
	}
	if err := json.Unmarshal(data, &resposta); err != nil {
		return nil, fmt.Errorf("lista de histórico inválida")
	}
	return resposta.Versoes, nil
}

func (c *KofreCloudStorage) BaixarVersao(ctx context.Context, id string) ([]byte, error) {
	if !regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-[a-f0-9]{16}$`).MatchString(id) {
		return nil, fmt.Errorf("versão inválida")
	}
	return c.lerHistorico(ctx, "/"+id)
}
