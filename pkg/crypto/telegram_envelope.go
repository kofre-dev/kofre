package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TelegramEnvelope armazena a chave dividida para desbloqueio seguro via Telegram
type TelegramEnvelope struct {
	EncryptedVaultKey []byte `json:"encrypted_vault_key"`
	LocalSecretBlob   []byte `json:"local_secret_blob"`
}

func getTelegramEnvelopePath() (string, error) {
	appData := os.Getenv("APPDATA")
	if appData != "" {
		dir := filepath.Join(appData, "Kofre")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", err
		}
		return filepath.Join(dir, "telegram_envelope.json"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	dir := filepath.Join(home, ".config", "Kofre")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "telegram_envelope.json"), nil
}

// HasTelegramUnlockEnvelope verifica se este computador já possui o envelope configurado
func HasTelegramUnlockEnvelope() bool {
	path, err := getTelegramEnvelopePath()
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

// SaveTelegramUnlockEnvelope cria o envelope split-key e envia a metade remota para o Kofre Cloud
func SaveTelegramUnlockEnvelope(vaultKey []byte, kofreToken, endpoint string) error {
	if len(vaultKey) == 0 || kofreToken == "" {
		return errors.New("chave do cofre ou token ausente")
	}

	// 1. Gera segredo local (32 bytes) e segredo remoto (32 bytes)
	localSecret := make([]byte, 32)
	serverSecret := make([]byte, 32)
	if _, err := rand.Read(localSecret); err != nil {
		return err
	}
	defer ZeroBytes(localSecret)
	if _, err := rand.Read(serverSecret); err != nil {
		return err
	}
	defer ZeroBytes(serverSecret)

	serverSecretHex := hex.EncodeToString(serverSecret)

	// 2. Envia a metade remota para o servidor associado ao perfil do Telegram
	url := fmt.Sprintf("%s/v1/telegram/unlock-key", strings.TrimRight(endpoint, "/"))
	body, _ := json.Marshal(map[string]string{
		"unlock_key": serverSecretHex,
	})
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", kofreToken))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("falha ao registrar chave no Kofre Cloud: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("servidor recusou chave (HTTP %d)", resp.StatusCode)
	}

	// 3. Deriva chave combinada do envelope: SHA256(localSecret + serverSecret)
	combined := append(append([]byte(nil), localSecret...), serverSecret...)
	envelopeKeyHash := sha256.Sum256(combined)
	ZeroBytes(combined)
	defer ZeroBytes(envelopeKeyHash[:])

	// 4. Criptografa o vaultKey com a chave do envelope
	encryptedVaultKey, err := Encrypt(vaultKey, envelopeKeyHash[:])
	if err != nil {
		return fmt.Errorf("falha ao cifrar envelope: %w", err)
	}

	// 5. Cifra localSecret com DPAPI no Windows para proteção contra leitura estática
	var localSecretBlob []byte
	dpapiBytes, err := EncryptWithDPAPI(localSecret)
	if err == nil {
		localSecretBlob = dpapiBytes
	} else {
		localSecretBlob = append([]byte(nil), localSecret...)
	}

	env := TelegramEnvelope{
		EncryptedVaultKey: encryptedVaultKey,
		LocalSecretBlob:   localSecretBlob,
	}

	envData, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return err
	}

	path, err := getTelegramEnvelopePath()
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, envData, 0600); err != nil {
		return err
	}
	_ = RestrictFilePermissions(path)

	return nil
}

// OpenTelegramUnlockEnvelope reconstrói a chave do cofre usando o segredo devolvido pelo Telegram
func OpenTelegramUnlockEnvelope(serverSecretHex string) ([]byte, error) {
	if serverSecretHex == "" {
		return nil, errors.New("segredo remoto do Telegram ausente")
	}

	serverSecret, err := hex.DecodeString(serverSecretHex)
	if err != nil {
		return nil, fmt.Errorf("segredo do Telegram inválido: %w", err)
	}
	defer ZeroBytes(serverSecret)

	path, err := getTelegramEnvelopePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("envelope de desbloqueio do Telegram não encontrado neste dispositivo: %w", err)
	}

	var env TelegramEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}

	var localSecret []byte
	decBytes, err := DecryptWithDPAPI(env.LocalSecretBlob)
	if err == nil {
		localSecret = decBytes
	} else {
		localSecret = append([]byte(nil), env.LocalSecretBlob...)
	}
	defer ZeroBytes(localSecret)

	// Reconstrói chave combinada: SHA256(localSecret + serverSecret)
	combined := append(append([]byte(nil), localSecret...), serverSecret...)
	envelopeKeyHash := sha256.Sum256(combined)
	ZeroBytes(combined)
	defer ZeroBytes(envelopeKeyHash[:])

	// Decifra o vaultKey
	vaultKey, err := Decrypt(env.EncryptedVaultKey, envelopeKeyHash[:])
	if err != nil {
		return nil, fmt.Errorf("falha ao decifrar cofre com autorização do Telegram: %w", err)
	}

	return vaultKey, nil
}
