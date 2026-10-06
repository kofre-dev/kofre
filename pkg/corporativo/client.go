package corporativo

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

type Client struct {
	Endpoint       string
	Identidade     *Identidade
	HTTP           *http.Client
	ConfirmarAutor func(string, Pessoa, string) bool
}

func IDValido(id string) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 16 && hex.EncodeToString(b) == id
}

func NovoClient(endpoint string, i *Identidade) (*Client, error) {
	if i == nil {
		return nil, fmt.Errorf("identidade corporativa obrigatória")
	}
	if _, err := i.Publica(); err != nil {
		return nil, fmt.Errorf("chave de identidade inválida")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Host == "" || u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")) {
		return nil, fmt.Errorf("endpoint corporativo deve usar HTTPS")
	}
	return &Client{Endpoint: strings.TrimRight(endpoint, "/"), Identidade: i, HTTP: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *Client) Request(ctx context.Context, method, path string, input, output any) error {
	u, parseErr := url.ParseRequestURI(path)
	valida := parseErr == nil && u.Scheme == "" && u.Host == "" && !strings.Contains(u.Path, "\\") && !strings.Contains(u.Path, "//")
	if valida {
		for _, segmento := range strings.Split(u.Path, "/") {
			if segmento == "." || segmento == ".." {
				valida = false
			}
		}
	}
	financeiro := strings.Split(path, "/")
	cancelar := method == http.MethodPost && len(financeiro) == 6 && financeiro[1] == "v1" && financeiro[2] == "billing" && financeiro[3] == "orders" && IDValido(financeiro[4]) && financeiro[5] == "cancel"
	if !valida || (!strings.HasPrefix(path, "/v1/corporativo/") && !cancelar) {
		return fmt.Errorf("rota corporativa inválida")
	}
	var body []byte
	var err error
	if input != nil {
		body, err = json.Marshal(input)
		if err != nil {
			return err
		}
		defer clear(body)
	}
	r, err := http.NewRequestWithContext(ctx, method, c.Endpoint+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/json")
	if c.Identidade != nil && c.Identidade.Token != "" {
		r.Header.Set("Authorization", "Bearer "+c.Identidade.Token)
	}
	resp, err := c.HTTP.Do(r)
	if err != nil {
		return fmt.Errorf("falha na conexão corporativa: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("Cloud HTTP %d: %s", resp.StatusCode, strings.TrimSpace(strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, string(b))))
	}
	if output == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 4*1024*1024))
		return err
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024+1))
	if err != nil {
		return err
	}
	defer clear(data)
	if len(data) > 4*1024*1024 {
		return fmt.Errorf("resposta corporativa excede limite")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if err = d.Decode(output); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("resposta corporativa contém dados excedentes")
	}
	return nil
}
