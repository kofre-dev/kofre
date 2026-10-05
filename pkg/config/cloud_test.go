package config

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAtivacaoPreservaCofreEContaExistente(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	if runtime.GOOS == "darwin" {
		t.Skip("os.UserConfigDir do macOS exige isolamento externo do usuário")
	}
	var status atomic.Int32
	status.Store(401)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/verify" {
			t.Error("rota inesperada")
		}
		w.WriteHeader(int(status.Load()))
	}))
	defer gateway.Close()
	cfg := DefaultConfig()
	cfg.VaultPath = filepath.Join(dir, "cofre-pessoal.enc")
	vault := []byte("arquivo criptografado ficticio")
	if err := os.WriteFile(cfg.VaultPath, vault, 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	configDir, _ := GetDefaultDir()
	path := filepath.Join(configDir, "config.json")
	before, _ := os.ReadFile(path)
	token := "kfr_pro_" + strings.Repeat("a", 64)
	if err := AtivarLicencaComprada(gateway.URL, token); err == nil {
		t.Fatal("pagamento não autorizado ativou")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("configuração alterada sem licença válida")
	}
	status.Store(200)
	if err := AtivarLicencaComprada(gateway.URL, token); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig()
	if err != nil || loaded.KofreToken != token || !loaded.CloudEnabled || loaded.VaultPath != cfg.VaultPath {
		t.Fatal("licença não salva corretamente")
	}
	data, _ := os.ReadFile(cfg.VaultPath)
	if !bytes.Equal(data, vault) {
		t.Fatal("arquivo local mudou")
	}
	before, _ = os.ReadFile(path)
	if runtime.GOOS == "windows" && bytes.Contains(before, []byte(token)) {
		t.Fatal("token salvo sem DPAPI")
	}
	if err = AtivarLicencaComprada(gateway.URL, "kfr_pro_"+strings.Repeat("b", 64)); err == nil {
		t.Fatal("compra substituiu outra conta")
	}
	after, _ = os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("conta existente alterada")
	}
	if err = os.WriteFile(path, []byte("config quebrada"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = AtivarLicencaComprada(gateway.URL, token); err == nil {
		t.Fatal("configuração corrompida sobrescrita")
	}
	after, _ = os.ReadFile(path)
	if string(after) != "config quebrada" {
		t.Fatal("configuração inválida não preservada")
	}
}
