package crypto_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/storage"
	"kofre/pkg/vault"
)

// Executado pelo script smoke_local.py contra o binário real do gateway.
// A URL deve ser loopback; os dados e credenciais são exclusivamente fictícios.
func TestSmokeGatewayReal(t *testing.T) {
	endpoint := os.Getenv("KOFRE_SMOKE_ENDPOINT")
	if endpoint == "" {
		t.Skip("execute kofre-cloud/scripts/smoke_local.py")
	}
	u, err := url.Parse(endpoint)
	if err != nil || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() {
		t.Fatal("smoke aceita somente loopback")
	}
	if runtime.GOOS == "darwin" {
		t.Skip("smoke com envelopes requer configuração isolada por APPDATA ou XDG_CONFIG_HOME")
	}
	admin, webhook := os.Getenv("KOFRE_SMOKE_ADMIN"), os.Getenv("KOFRE_SMOKE_WEBHOOK")
	client := &http.Client{Timeout: 10 * time.Second}
	call := func(status int, method, path, body, token string, isAdmin, isWebhook bool) map[string]any {
		t.Helper()
		req, err := http.NewRequest(method, endpoint+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if isAdmin {
			req.Header.Set("X-Admin-Secret", admin)
		}
		if isWebhook {
			req.Header.Set("X-Telegram-Bot-Api-Secret-Token", webhook)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != status {
			t.Fatalf("%s %s: HTTP %d, esperado %d", method, path, resp.StatusCode, status)
		}
		var result map[string]any
		_ = json.Unmarshal(data, &result)
		return result
	}
	token := call(200, "POST", "/v1/auth/provision", "", "", true, false)["token"].(string)
	call(401, "POST", "/v1/auth/verify", "", "kfr_falso_canario", false, false)
	link := call(200, "POST", "/v1/telegram/link-request", "", token, false, false)["code"].(string)
	message := func(text string) string {
		body, _ := json.Marshal(map[string]any{"message": map[string]any{"chat": map[string]any{"id": 123}, "from": map[string]any{"id": 123}, "text": text}})
		return string(body)
	}
	call(200, "POST", "/v1/telegram/webhook", message("/start "+link), "", false, true)
	key, salt, err := mycrypto.DeriveKeyBytes([]byte("senha-mestre-ficticia-do-smoke"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(key)
	v := vault.NewManaged()
	defer v.Close()
	_, err = v.AddEntry(vault.SecretEntry{Title: "Canário smoke", Fields: []vault.Field{{Name: "Senha", Value: "segredo-exclusivamente-ficticio", Protected: true}}})
	if err != nil {
		t.Fatal(err)
	}
	packed, err := v.Pack(key, salt)
	if err != nil {
		t.Fatal(err)
	}
	cloud := storage.NewKofreCloudStorage(endpoint, token)
	local, err := storage.NewLocalStorage(filepath.Join(t.TempDir(), "vault.enc"))
	if err != nil {
		t.Fatal(err)
	}
	syncing := storage.NewSyncStorage(local, cloud)
	defer syncing.Close()
	if err = syncing.Save(context.Background(), packed); err != nil {
		t.Fatal(err)
	}
	if err = syncing.Flush(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	loaded, err := cloud.Load(context.Background())
	if err != nil || !bytes.Equal(loaded, packed) {
		t.Fatal("cloud não confirmou bytes criptografados")
	}
	deviceDirs := []string{t.TempDir(), t.TempDir()}
	deviceIDs := make([]string, 2)
	selectDevice := func(dir string) { t.Setenv("APPDATA", dir); t.Setenv("XDG_CONFIG_HOME", dir) }
	for index, dir := range deviceDirs {
		selectDevice(dir)
		if err = mycrypto.SaveTelegramUnlockEnvelope(key, token, endpoint); err != nil {
			t.Fatal(err)
		}
		deviceIDs[index] = mycrypto.TelegramEnvelopeDeviceID()
	}
	for index, dir := range deviceDirs {
		selectDevice(dir)
		id := call(200, "POST", "/v1/auth/telegram-challenge", fmt.Sprintf(`{"device_id":"%s"}`, deviceIDs[index]), token, false, false)["challenge_id"].(string)
		approval := fmt.Sprintf(`{"callback_query":{"id":"ficticio","from":{"id":123},"message":{"chat":{"id":123}},"data":"auth_approve:%s"}}`, id)
		call(401, "POST", "/v1/telegram/webhook", approval, "", false, false)
		call(200, "POST", "/v1/telegram/webhook", approval, "", false, true)
		response := call(200, "GET", "/v1/auth/telegram-challenge/status?id="+id, "", token, false, false)
		recovered, err := mycrypto.OpenTelegramUnlockEnvelope(response["unlock_secret"].(string))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(recovered, key) {
			t.Fatal("chave de outro dispositivo sobrescrita")
		}
		recoveredSalt, payload, err := vault.UnpackHeader(loaded)
		if err != nil {
			t.Fatal(err)
		}
		opened, err := vault.DecryptAndLoad(payload, recovered, recoveredSalt)
		mycrypto.ZeroBytes(recovered)
		if err != nil {
			t.Fatal(err)
		}
		if opened.Count() != 1 {
			t.Fatal("conteúdo do cofre não preservado")
		}
		opened.Close()
		replay := call(200, "GET", "/v1/auth/telegram-challenge/status?id="+id, "", token, false, false)
		if replay["unlock_secret"] != nil {
			t.Fatal("replay liberou segredo")
		}
	}
	// Exercita concorrência do cliente real e do gateway sobre bytes cifrados.
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 16 {
				data, err := v.Pack(key, salt)
				if err != nil {
					t.Error(err)
					return
				}
				if err = syncing.Save(context.Background(), data); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if err = syncing.Flush(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	localBytes, _ := local.Load(context.Background())
	remoteBytes, err := cloud.Load(context.Background())
	if err != nil || !bytes.Equal(localBytes, remoteBytes) {
		t.Fatal("128 gravações concorrentes não convergiram")
	}
	call(200, "POST", "/v1/telegram/webhook", message("/panic"), "", false, true)
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		call(423, method, "/v1/vault", string(packed), token, false, false)
	}
	call(200, "POST", "/v1/telegram/webhook", message("/unlock"), "", false, true)
	call(200, "POST", "/v1/auth/provision", fmt.Sprintf(`{"token":"%s","status":"revoked"}`, token), "", true, false)
	call(403, "GET", "/v1/vault", "", token, false, false)
	t.Log("gateway compilado + storage/Argon2id/AES-GCM/DPAPI reais: dois dispositivos, replay, pânico, revogação e 128 gravações concorrentes passaram")
}
