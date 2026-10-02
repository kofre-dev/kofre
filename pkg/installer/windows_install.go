//go:build windows

package installer

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\Kofre`

func installedPath() (string, error) {
	root := os.Getenv("LOCALAPPDATA")
	if root == "" || !filepath.IsAbs(root) {
		return "", fmt.Errorf("LOCALAPPDATA indisponivel ou invalido")
	}
	return filepath.Join(root, "Programs", "Kofre", "Kofre.exe"), nil
}

func RegisterInstallation(binaryPath, version string) error {
	return registerAt(binaryPath, version, uninstallKey)
}

func registerAt(binaryPath, version, keyPath string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	for name, value := range map[string]string{
		"DisplayName": "Kofre", "DisplayVersion": version, "Publisher": "Kofre",
		"InstallLocation": filepath.Dir(binaryPath), "DisplayIcon": `"` + binaryPath + `",0`,
		"UninstallString": `"` + binaryPath + `" uninstall`, "URLInfoAbout": "https://kofre.dev",
	} {
		if err := key.SetStringValue(name, value); err != nil {
			return err
		}
	}
	for _, name := range []string{"NoModify", "NoRepair"} {
		if err := key.SetDWordValue(name, 1); err != nil {
			return err
		}
	}
	return nil
}

// Inclui instalações existentes quando o binário atualizado é iniciado.
func RefreshRegistration(version string) error {
	expected, err := installedPath()
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(self), expected) {
		return nil
	}
	return RegisterInstallation(expected, version)
}

func updateWindowsPath(dir string, add bool) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	value, kind, err := key.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return err
	}
	parts := strings.Split(value, ";")
	kept := make([]string, 0, len(parts)+1)
	for _, part := range parts {
		normalized := strings.TrimRight(strings.Trim(strings.TrimSpace(part), `"`), `\/`)
		if !strings.EqualFold(normalized, strings.TrimRight(dir, `\/`)) {
			kept = append(kept, part)
		}
	}
	if add {
		kept = append(kept, dir)
	}
	next := strings.Join(kept, ";")
	if next == value {
		return nil
	}
	if kind == registry.EXPAND_SZ {
		err = key.SetExpandStringValue("Path", next)
	} else {
		err = key.SetStringValue("Path", next)
	}
	if err != nil {
		return err
	}
	// Avisa o Explorer para que novos terminais herdem o PATH atualizado.
	name, _ := windows.UTF16PtrFromString("Environment")
	var result uintptr
	_, _, _ = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW").Call(0xffff, 0x001a, 0, uintptr(unsafe.Pointer(name)), 2, 2000, uintptr(unsafe.Pointer(&result)))
	return nil
}

func psLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

// Remove somente o executável nomeado, sem remoção recursiva de diretórios.
func uninstallScript(target, keyPath, environmentKey string, parentPID int) string {
	dir := filepath.Dir(target)
	return fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$target = %s
$installDir = %s
$log = Join-Path $env:TEMP 'Kofre-desinstalacao.log'
try {
    if ([IO.Path]::GetFullPath($target) -ne (Join-Path $installDir 'Kofre.exe')) { throw 'Destino invalido' }
    Wait-Process -Id %d -Timeout 30 -ErrorAction SilentlyContinue
    for ($attempt=0; $attempt -lt 30; $attempt++) {
        try { if (Test-Path -LiteralPath $target) { Remove-Item -LiteralPath $target -Force }; break }
        catch { if ($attempt -eq 29) { throw }; Start-Sleep -Milliseconds 500 }
    }
    $environmentKey = %s
    $path = (Get-ItemProperty -LiteralPath $environmentKey -Name Path -ErrorAction SilentlyContinue).Path
    $items = @($path -split ';' | Where-Object { $_.Trim().Trim('"').TrimEnd('\','/') -ine $installDir.TrimEnd('\','/') })
    Set-ItemProperty -LiteralPath $environmentKey -Name Path -Value ($items -join ';')
    $registration = %s
    if (Test-Path -LiteralPath $registration) { Remove-Item -LiteralPath $registration }
    'Kofre desinstalado. Vault, configuracoes e backups preservados.' | Set-Content -LiteralPath $log
} catch {
    ('Falha ao desinstalar Kofre: ' + $_.Exception.Message) | Set-Content -LiteralPath $log
    exit 1
}
`, psLiteral(target), psLiteral(dir), parentPID, psLiteral(environmentKey), psLiteral(`HKCU:\`+keyPath))
}

func encodedPowerShell(script string) string {
	units := utf16.Encode([]rune(script))
	data := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(data[i*2:], u)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func Uninstall() error {
	target, err := installedPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("instalacao nao encontrada: %w", err)
	}
	script := uninstallScript(target, uninstallKey, `HKCU:\Environment`, os.Getpid())
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.Command(powershell, "-NoProfile", "-NonInteractive", "-EncodedCommand", encodedPowerShell(script))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
