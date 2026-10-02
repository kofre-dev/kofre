//go:build windows

package installer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/sys/windows/registry"
)

func TestRegisterAndUninstallPreservesData(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Kofre 'teste' [isolado]")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(dir, "Kofre.exe")
	for _, name := range []string{"Kofre.exe", "vault.enc", "config.json", "backup.bak"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	root := `Software\KofreTeste-` + uuid.NewString()
	regPath, envPath := root+`\Uninstall`, root+`\Environment`
	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, regPath)
		_ = registry.DeleteKey(registry.CURRENT_USER, envPath)
		_ = registry.DeleteKey(registry.CURRENT_USER, root)
	})
	if err := registerAt(binaryPath, "teste", regPath); err != nil {
		t.Fatal(err)
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	command, _, err := key.GetStringValue("UninstallString")
	key.Close()
	if err != nil || command != `"`+binaryPath+`" uninstall` {
		t.Fatal("comando de desinstalacao incorreto")
	}
	env, _, err := registry.CreateKey(registry.CURRENT_USER, envPath, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	pathValue := `C:\Windows;` + dir + `;` + dir + `-outro;C:\Ferramentas`
	if err := env.SetExpandStringValue("Path", pathValue); err != nil {
		t.Fatal(err)
	}
	script := uninstallScript(binaryPath, regPath, `HKCU:\`+envPath, 2147483647)
	if strings.Contains(script, "-Recurse") {
		t.Fatal("desinstalador nao deve remover pastas recursivamente")
	}
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodedPowerShell(script))
	cmd.Env = append(os.Environ(), "TEMP="+t.TempDir())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("desinstalacao: %v %s", err, output)
	}
	if _, err := os.Stat(binaryPath); !os.IsNotExist(err) {
		t.Fatal("executavel permaneceu")
	}
	for _, name := range []string{"vault.enc", "config.json", "backup.bak"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(data) != "fixture" {
			t.Fatalf("dados alterados: %s", name)
		}
	}
	actual, kind, err := env.GetStringValue("Path")
	if err != nil || actual != `C:\Windows;`+dir+`-outro;C:\Ferramentas` || kind != registry.EXPAND_SZ {
		t.Fatalf("PATH divergente: %q tipo %d erro %v", actual, kind, err)
	}
	remaining, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.QUERY_VALUE)
	if err == nil {
		remaining.Close()
		t.Fatal("registro permaneceu")
	}
}
