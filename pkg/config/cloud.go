package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func ValidateCloudLicense(endpoint, token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/v1/auth/verify", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	resp, err := (&http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("licença não autorizada (HTTP %d)", resp.StatusCode)
	}
	return nil
}

// Preserva o cofre e impede que uma compra substitua outra conta configurada.
func AtivarLicencaComprada(endpoint, token string) error {
	dir, err := GetDefaultDir()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil && !os.IsNotExist(err) {
		return errors.New("não foi possível ler a configuração existente")
	}
	if err == nil {
		var existente AppConfig
		if json.Unmarshal(data, &existente) != nil {
			return errors.New("configuração existente inválida; ela foi preservada")
		}
	}
	if err := ValidateCloudLicense(endpoint, token); err != nil {
		return err
	}
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	if cfg.CloudEnabled && cfg.KofreToken != token {
		return errors.New("já existe outra conta de nuvem configurada; a compra não alterou sua conta")
	}
	cfg.Mode = "kofre_cloud"
	cfg.CloudEnabled = true
	cfg.KofreToken = token
	cfg.CloudEndpoint = endpoint
	return SaveConfig(cfg)
}
