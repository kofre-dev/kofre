package crypto

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestTelegramEnvelopeSplitKey(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var capturedUnlockKey string

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/telegram/unlock-key" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			UnlockKey string `json:"unlock_key"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		capturedUnlockKey = req.UnlockKey

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}))
	defer mockServer.Close()

	// Garante que o arquivo de teste seja limpo ao final
	path, err := getTelegramEnvelopePath()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	dummyVaultKey := bytes.Repeat([]byte{7}, KeyLength)

	// 1. Salva o envelope split-key
	err = SaveTelegramUnlockEnvelope(dummyVaultKey, "mock-token", mockServer.URL)
	if err != nil {
		t.Fatalf("falha ao salvar envelope: %v", err)
	}

	if capturedUnlockKey == "" {
		t.Fatal("servidor mock nao recebeu a chave remota")
	}

	if !HasTelegramUnlockEnvelope() {
		t.Fatal("HasTelegramUnlockEnvelope retornou false")
	}

	// 2. Abre o envelope com o segredo correto
	recoveredKey, err := OpenTelegramUnlockEnvelope(capturedUnlockKey)
	if err != nil {
		t.Fatalf("falha ao abrir envelope: %v", err)
	}
	defer ZeroBytes(recoveredKey)

	if !bytes.Equal(recoveredKey, dummyVaultKey) {
		t.Fatal("chave recuperada diverge da chave original")
	}

	// 3. Testa segredo incorreto
	_, err = OpenTelegramUnlockEnvelope("0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("esperava erro com segredo remoto incorreto")
	}
}
