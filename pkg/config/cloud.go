package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"kofre/pkg/corporativo"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func ValidateCloudLicense(endpoint, token string) error {
	_, err := ConsultarPlanoCloud(endpoint, token)
	return err
}

var ErrContaRevogada = errors.New("conta não autorizada")

type PlanoCloud struct {
	Valido     bool   `json:"valid"`
	ContaID    string `json:"user_id"`
	Plano      string `json:"plan"`
	QuotaBytes int64  `json:"quota_bytes"`
}

func ConsultarPlanoCloud(endpoint, token string) (PlanoCloud, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/v1/auth/verify", nil)
	if err != nil {
		return PlanoCloud{}, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	resp, err := (&http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		return PlanoCloud{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return PlanoCloud{}, fmt.Errorf("%w (HTTP %d)", ErrContaRevogada, resp.StatusCode)
		}
		return PlanoCloud{}, fmt.Errorf("conta não autorizada (HTTP %d)", resp.StatusCode)
	}
	var p PlanoCloud
	if json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&p) != nil || !p.Valido || p.ContaID == "" || (p.Plano != "free" && p.Plano != "pro") {
		return PlanoCloud{}, errors.New("resposta de conta inválida")
	}
	return p, nil
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
	plano, err := ConsultarPlanoCloud(endpoint, token)
	if err != nil {
		return err
	}
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	mesmaConta := corporativo.CredencialContaValida(cfg.KofreToken) && corporativo.CredencialContaValida(token) && cfg.ContaID != "" && cfg.ContaID == plano.ContaID && strings.TrimRight(cfg.CloudEndpoint, "/") == strings.TrimRight(endpoint, "/")
	if cfg.CloudEnabled && cfg.KofreToken != token && !mesmaConta {
		return errors.New("já existe outra conta de nuvem configurada; a compra não alterou sua conta")
	}
	cfg.Mode = "kofre_cloud"
	cfg.CloudEnabled = true
	cfg.KofreToken = token
	cfg.CloudEndpoint = endpoint
	cfg.ContaID, cfg.PlanoCloud = plano.ContaID, plano.Plano
	return SaveConfig(cfg)
}

func AtualizarPlanoCloud() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	if !cfg.CloudEnabled || cfg.KofreToken == "" {
		return nil
	}
	plano, err := ConsultarPlanoCloud(GetCloudEndpoint(), cfg.KofreToken)
	revogada := errors.Is(err, ErrContaRevogada)
	if err != nil && !revogada {
		return err
	}
	atual, err := LoadConfig()
	if err != nil {
		return err
	}
	if atual.KofreToken != cfg.KofreToken || atual.CloudEndpoint != cfg.CloudEndpoint {
		return errors.New("configuração mudou durante a consulta")
	}
	cfg = atual
	if revogada {
		cfg.PlanoCloud = "free"
		return SaveConfig(cfg)
	}
	if cfg.ContaID != "" && cfg.ContaID != plano.ContaID {
		return errors.New("a conta da nuvem mudou; configuração preservada")
	}
	cfg.ContaID, cfg.PlanoCloud = plano.ContaID, plano.Plano
	return SaveConfig(cfg)
}

func ConfirmarProNaConta(contaID string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	if !cfg.CloudEnabled || cfg.ContaID != contaID || !corporativo.CredencialContaValida(cfg.KofreToken) {
		return errors.New("a compra pertence a outra conta; configuração preservada")
	}
	p, err := ConsultarPlanoCloud(GetCloudEndpoint(), cfg.KofreToken)
	if err != nil {
		return err
	}
	if p.ContaID != contaID || p.Plano != "pro" {
		return errors.New("Pro ainda não confirmado para esta conta")
	}
	cfg.PlanoCloud = p.Plano
	return SaveConfig(cfg)
}
