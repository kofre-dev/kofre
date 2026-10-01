package installer

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"kofre/pkg/vault"
)

// InstallBinary instala o executavel atual no diretorio de programas do usuario e adiciona ao PATH
func InstallBinary() (string, error) {
	selfPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("falha ao identificar executavel atual: %w", err)
	}

	var installDir string
	var destBin string

	switch runtime.GOOS {
	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		installDir = filepath.Join(localAppData, "Programs", "Kofre")
		destBin = filepath.Join(installDir, "Kofre.exe")

	default: // Linux e macOS
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		installDir = filepath.Join(home, ".local", "bin")
		destBin = filepath.Join(installDir, "Kofre")
	}

	// Cria o diretorio de instalacao
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", fmt.Errorf("falha ao criar pasta de instalacao: %w", err)
	}

	// Copia o executavel (se nao for o mesmo destino)
	if !strings.EqualFold(selfPath, destBin) {
		if err := copyFile(selfPath, destBin); err != nil {
			return "", fmt.Errorf("falha ao copiar binario para %s: %w", destBin, err)
		}
		_ = os.Chmod(destBin, 0755)
	}

	// Adiciona ao PATH
	if err := addToUserPath(installDir); err != nil {
		return destBin, fmt.Errorf("binario copiado, mas houve aviso ao atualizar PATH: %w", err)
	}

	return destBin, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Se ja existir, remove ou sobrescreve
	_ = os.Remove(dst)

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func addToUserPath(dir string) error {
	if runtime.GOOS == "windows" {
		// Comando PowerShell seguro para adicionar ao PATH do Usuario sem precisar de admin
		psScript := fmt.Sprintf(`
$dir = '%s'
$path = [Environment]::GetEnvironmentVariable('Path', [EnvironmentVariableTarget]::User)
$items = $path -split ';' | Where-Object { $_ -ne '' }
if ($items -notcontains $dir) {
    $newPath = ($items + $dir) -join ';'
    [Environment]::SetEnvironmentVariable('Path', $newPath, [EnvironmentVariableTarget]::User)
}
`, dir)

		cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
		return cmd.Run()
	}

	// Linux / macOS: verifica ~/.bashrc ou ~/.zshrc
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	exportLine := fmt.Sprintf("\nexport PATH=\"%s:$PATH\"\n", dir)

	for _, rc := range []string{".zshrc", ".bashrc", ".profile"} {
		targetFile := filepath.Join(home, rc)
		if data, err := os.ReadFile(targetFile); err == nil {
			if !strings.Contains(string(data), dir) {
				f, err := os.OpenFile(targetFile, os.O_APPEND|os.O_WRONLY, 0644)
				if err == nil {
					_, _ = f.WriteString(exportLine)
					f.Close()
				}
			}
		}
	}

	return nil
}

// CreateBackup gera uma copia criptografada com timestamp de seguranca
func CreateBackup(srcVaultPath, destDir string) (string, error) {
	if _, err := os.Stat(srcVaultPath); os.IsNotExist(err) {
		return "", fmt.Errorf("arquivo do cofre nao encontrado em %s", srcVaultPath)
	}

	data, err := os.ReadFile(srcVaultPath)
	if err != nil {
		return "", err
	}

	// Valida integridade do cabecalho antes de gerar backup
	if _, _, err := vault.UnpackHeader(data); err != nil {
		return "", fmt.Errorf("arquivo de origem corrompido ou invalido: %w", err)
	}

	if destDir == "" {
		destDir = filepath.Dir(srcVaultPath)
	}

	timestamp := time.Now().Format("2006-01-02-150405")
	backupName := fmt.Sprintf("Kofre-backup-%s.enc", timestamp)
	backupPath := filepath.Join(destDir, backupName)

	if err := os.WriteFile(backupPath, data, 0600); err != nil {
		return "", fmt.Errorf("falha ao salvar backup: %w", err)
	}

	return backupPath, nil
}

// RestoreBackup restaura o cofre a partir de um backup validado
func RestoreBackup(backupPath, targetVaultPath string) error {
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("falha ao ler arquivo de backup: %w", err)
	}

	// Valida cabecalho
	if _, _, err := vault.UnpackHeader(data); err != nil {
		return fmt.Errorf("arquivo selecionado nao e um backup valido do Kofre: %w", err)
	}

	dir := filepath.Dir(targetVaultPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Gravacao com escrita segura
	tmpPath := targetVaultPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}

	_ = os.Remove(targetVaultPath)
	return os.Rename(tmpPath, targetVaultPath)
}
