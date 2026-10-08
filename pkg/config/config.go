package config

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"kofre/internal/arquivo"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	mycrypto "kofre/pkg/crypto"
)

// Valores padrões da infraestrutura pública
var (
	DefaultCloudEndpoint = "https://api.kofre.dev"
	DefaultTelegramBot   = "kofredev_bot"
)

// AppConfig armazena as configuracoes do usuario
type AppConfig struct {
	ContaConfigurada bool   `json:"conta_configurada,omitempty"` // Apenas apresentação; não autoriza acesso ao Cloud.
	ContaID          string `json:"conta_id,omitempty"`
	PlanoCloud       string `json:"plano_cloud,omitempty"` // Somente apresentação; autorização pertence ao servidor.
	Tema             string `json:"tema,omitempty"`
	Mode             string `json:"mode"`          // "local" ou "cloud"
	VaultPath        string `json:"vault_path"`    // caminho do arquivo local
	CloudEnabled     bool   `json:"cloud_enabled"` // se sincronizacao com S3 esta ativa
	S3Bucket         string `json:"s3_bucket"`
	S3Key            string `json:"s3_key"`
	S3Region         string `json:"s3_region"`
	S3Endpoint       string `json:"s3_endpoint"` // para Cloudflare R2 ou MinIO
	S3AccessKey      string `json:"s3_access_key,omitempty"`
	S3SecretKey      string `json:"s3_secret_key,omitempty"`
	CloudEndpoint    string `json:"cloud_endpoint,omitempty"` // URL da API (ex: https://api.kofre.dev)
	KofreToken       string `json:"kofre_token,omitempty"`    // Token da licença Pro
	TelegramAuth     bool   `json:"telegram_auth"`            // se usa autenticacao remota do Telegram
	TelegramBot      string `json:"telegram_bot_token,omitempty"`
	TelegramChat     string `json:"telegram_chat_id,omitempty"`
}

// GetCloudEndpoint retorna a URL da API da Nuvem com prioridade: ENV > config.json > Default
func GetCloudEndpoint() string {
	if env := os.Getenv("KOFRE_CLOUD_ENDPOINT"); env != "" {
		return strings.TrimRight(env, "/")
	}
	cfg, err := LoadConfig()
	if err == nil && cfg != nil && cfg.CloudEndpoint != "" {
		return strings.TrimRight(cfg.CloudEndpoint, "/")
	}
	return strings.TrimRight(DefaultCloudEndpoint, "/")
}

// GetTelegramBot retorna o username do bot do Telegram oficial
func GetTelegramBot() string {
	if env := os.Getenv("KOFRE_TELEGRAM_BOT"); env != "" {
		return strings.TrimPrefix(env, "@")
	}
	return DefaultTelegramBot
}

// GetDefaultDir retorna o diretorio padrao do Kofre baseado no sistema operacional
// Windows: %APPDATA%/Kofre
// Linux: ~/.config/Kofre
// macOS: ~/Library/Application Support/Kofre
func GetDefaultDir() (string, error) {
	baseDir, err := os.UserConfigDir()
	if err != nil {
		home, errHome := os.UserHomeDir()
		if errHome != nil {
			return ".", nil
		}
		baseDir = filepath.Join(home, ".config")
	}

	appDir := filepath.Join(baseDir, "Kofre")
	if err := os.MkdirAll(appDir, 0700); err != nil {
		return "", fmt.Errorf("falha ao criar pasta de configuracao: %w", err)
	}

	return appDir, nil
}

// GetDefaultVaultPath retorna o caminho padrao do arquivo vault.enc
func GetDefaultVaultPath() (string, error) {
	dir, err := GetDefaultDir()
	if err != nil {
		return "vault.enc", err
	}
	return filepath.Join(dir, "vault.enc"), nil
}

// LoadConfig carrega o config.json ou cria uma configuracao padrao
func LoadConfig() (*AppConfig, error) {
	dir, err := GetDefaultDir()
	if err != nil {
		return DefaultConfig(), nil
	}

	cfgPath := filepath.Join(dir, "config.json")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		cfg := DefaultConfig()
		_ = SaveConfig(cfg)
		return cfg, nil
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return DefaultConfig(), nil
	}

	var cfg AppConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), nil
	}

	// Se o token estiver protegido com DPAPI, descriptografa para uso em memória
	if strings.HasPrefix(cfg.KofreToken, "enc:dpapi:") {
		encHex := strings.TrimPrefix(cfg.KofreToken, "enc:dpapi:")
		if encBytes, errHex := hex.DecodeString(encHex); errHex == nil {
			if decBytes, errDec := mycrypto.DecryptWithDPAPI(encBytes); errDec == nil {
				cfg.KofreToken = string(decBytes)
			} else {
				// Chave não pertence a este hardware ou usuário: token invalidado
				cfg.KofreToken = ""
			}
		}
	}

	if cfg.VaultPath == "" {
		cfg.VaultPath, _ = GetDefaultVaultPath()
	}

	// Migração transparente de endpoints legados do Railway
	if strings.Contains(cfg.CloudEndpoint, "kofre-api-production.up.railway.app") {
		cfg.CloudEndpoint = DefaultCloudEndpoint
		_ = SaveConfig(&cfg)
	}

	return &cfg, nil
}

// SaveConfig grava as configuracoes no arquivo config.json protegendo o token com DPAPI (UTF-8 sem BOM)
func SaveConfig(cfg *AppConfig) error {
	dir, err := GetDefaultDir()
	if err != nil {
		return err
	}

	// Cria cópia para persistência no disco protegendo o token com DPAPI
	persisted := *cfg
	if persisted.KofreToken != "" && !strings.HasPrefix(persisted.KofreToken, "enc:dpapi:") {
		if encBytes, err := mycrypto.EncryptWithDPAPI([]byte(persisted.KofreToken)); err == nil {
			persisted.KofreToken = "enc:dpapi:" + hex.EncodeToString(encBytes)
		} else if runtime.GOOS == "windows" {
			return fmt.Errorf("falha ao proteger token: %w", err)
		}
	}

	cfgPath := filepath.Join(dir, "config.json")
	data, err := json.MarshalIndent(&persisted, "", "  ")
	if err != nil {
		return err
	}

	return arquivo.Gravar(cfgPath, data, mycrypto.RestrictFilePermissions)
}

// DefaultConfig gera a configuracao padrao inicial
func DefaultConfig() *AppConfig {
	defaultVault, _ := GetDefaultVaultPath()
	return &AppConfig{
		Mode:         "local",
		VaultPath:    defaultVault,
		CloudEnabled: false,
		S3Key:        "vault.enc",
		S3Region:     "us-east-1",
	}
}
