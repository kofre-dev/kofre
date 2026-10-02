package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"kofre/pkg/config"
)

// CurrentVersion define a versão atual compilada do binário do Kofre
const CurrentVersion = "1.0.6"

// PlatformRelease armazena os metadados do binário para um sistema operacional e arquitetura
type PlatformRelease struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

// ReleaseMetadata define as informações da versão mais recente publicada
type ReleaseMetadata struct {
	Version     string                     `json:"version"`
	ReleaseDate string                     `json:"release_date"`
	Notes       string                     `json:"notes"`
	Platforms   map[string]PlatformRelease `json:"platforms"`
}

// CurrentPlatform retorna o identificador no formato os-arch (ex: windows-amd64)
func CurrentPlatform() string {
	return fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
}

// CheckForUpdate consulta a API do Kofre para verificar se há versão mais recente
func CheckForUpdate(endpoint string) (*ReleaseMetadata, bool, error) {
	if endpoint == "" {
		endpoint = config.GetCloudEndpoint()
	}
	endpoint = strings.TrimRight(endpoint, "/")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/v1/version/latest", endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("status inesperado da API: %d", resp.StatusCode)
	}

	var meta ReleaseMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, false, err
	}

	hasNew := IsNewerVersion(meta.Version, CurrentVersion)
	return &meta, hasNew, nil
}

// IsNewerVersion compara semanticamente duas versões (ex: 1.0.1 > 1.0.0)
func IsNewerVersion(remote, local string) bool {
	remote = strings.TrimPrefix(strings.TrimSpace(remote), "v")
	local = strings.TrimPrefix(strings.TrimSpace(local), "v")

	rParts := strings.Split(remote, ".")
	lParts := strings.Split(local, ".")

	for i := 0; i < len(rParts) && i < len(lParts); i++ {
		rNum, errR := strconv.Atoi(rParts[i])
		lNum, errL := strconv.Atoi(lParts[i])
		if errR == nil && errL == nil {
			if rNum > lNum {
				return true
			}
			if rNum < lNum {
				return false
			}
		}
	}
	return len(rParts) > len(lParts)
}

// AutoUpdate verifica e aplica a atualização automaticamente
func AutoUpdate(endpoint string, silent bool) (bool, error) {
	// 1. Limpa resíduos de atualizações anteriores (.old)
	CleanupOldBinaries()

	meta, hasNew, err := CheckForUpdate(endpoint)
	if err != nil {
		// Se offline ou timeout, não bloqueia o uso normal do Kofre
		return false, nil
	}

	if !hasNew {
		if !silent {
			fmt.Printf("✓ Kofre v%s já está na versão mais recente.\n", CurrentVersion)
		}
		return false, nil
	}

	platKey := CurrentPlatform()
	platInfo, ok := meta.Platforms[platKey]
	if !ok {
		// Sem binário para a plataforma atual
		return false, nil
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  Kofre 🚀 Nova versão detectada: v%-7s (Atual: v%-7s) ║\n", meta.Version, CurrentVersion)
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	if meta.Notes != "" {
		fmt.Printf("Notas: %s\n", meta.Notes)
	}
	fmt.Print("Baixando atualização e aplicando automaticamente... ")

	// Obtém o caminho do binário atual em execução
	execPath, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("não foi possível identificar o caminho do executável: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return false, err
	}

	dir := filepath.Dir(execPath)
	base := filepath.Base(execPath)
	tempPath := filepath.Join(dir, base+".new")
	oldPath := filepath.Join(dir, base+".old")

	downloadURL := platInfo.URL
	if !strings.HasPrefix(downloadURL, "http://") && !strings.HasPrefix(downloadURL, "https://") {
		if endpoint == "" {
			endpoint = config.GetCloudEndpoint()
		}
		downloadURL = strings.TrimRight(endpoint, "/") + "/" + strings.TrimLeft(downloadURL, "/")
	}

	// 2. Download do novo binário
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		fmt.Println("❌")
		return false, fmt.Errorf("falha ao baixar binário: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Println("❌")
		return false, fmt.Errorf("erro no download do binário, HTTP status: %d", resp.StatusCode)
	}

	out, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		fmt.Println("❌")
		return false, fmt.Errorf("falha ao criar arquivo temporário: %w", err)
	}

	hasher := sha256.New()
	multiWriter := io.MultiWriter(out, hasher)
	_, err = io.Copy(multiWriter, resp.Body)
	_ = out.Close()
	if err != nil {
		_ = os.Remove(tempPath)
		fmt.Println("❌")
		return false, fmt.Errorf("falha ao gravar binário: %w", err)
	}

	// 3. Validação do Hash SHA-256 se fornecido
	if platInfo.SHA256 != "" {
		calculatedSHA := hex.EncodeToString(hasher.Sum(nil))
		if !strings.EqualFold(calculatedSHA, platInfo.SHA256) {
			_ = os.Remove(tempPath)
			fmt.Println("❌")
			return false, fmt.Errorf("validação de integridade falhou: checksum não confere")
		}
	}

	// 4. Substituição atômica no Windows / Unix
	_ = os.Remove(oldPath) // remove se existir
	if err := os.Rename(execPath, oldPath); err != nil {
		_ = os.Remove(tempPath)
		fmt.Println("❌")
		return false, fmt.Errorf("falha ao substituir binário em execução: %w", err)
	}

	if err := os.Rename(tempPath, execPath); err != nil {
		// Tenta reverter
		_ = os.Rename(oldPath, execPath)
		fmt.Println("❌")
		return false, fmt.Errorf("falha ao promover novo binário: %w", err)
	}

	fmt.Println("✓ Concluído!")
	fmt.Printf("O Kofre foi atualizado com sucesso para v%s.\n\n", meta.Version)
	return true, nil
}

// CleanupOldBinaries remove silenciosamente arquivos .old gerados em updates anteriores
func CleanupOldBinaries() {
	execPath, err := os.Executable()
	if err != nil {
		return
	}
	execPath, _ = filepath.EvalSymlinks(execPath)
	oldPath := filepath.Join(filepath.Dir(execPath), filepath.Base(execPath)+".old")
	if _, err := os.Stat(oldPath); err == nil {
		_ = os.Remove(oldPath)
	}
}
