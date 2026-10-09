package updater

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"kofre/pkg/config"
	"kofre/pkg/releasesign"
)

// CurrentVersion define a versão atual compilada do binário do Kofre
const CurrentVersion = "1.0.23"

// PlatformRelease armazena os metadados do binário para um sistema operacional e arquitetura
type PlatformRelease = releasesign.PlatformRelease

// ReleaseMetadata define as informações da versão mais recente publicada
type ReleaseMetadata = releasesign.Metadata

// CurrentPlatform retorna o identificador no formato os-arch (ex: windows-amd64)
func CurrentPlatform() string {
	return fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
}

// CheckForUpdate consulta a API do Kofre para verificar se há versão mais recente
func CheckForUpdate(endpoint string) (*ReleaseMetadata, bool, error) {
	return checkForUpdate(endpoint, CurrentPlatform(), CurrentVersion, time.Now(), releasesign.TrustedKeys())
}

func checkForUpdate(endpoint, platform, current string, now time.Time, keys map[string]ed25519.PublicKey) (*ReleaseMetadata, bool, error) {
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

	if err := secureURL(url); err != nil {
		return nil, false, err
	}
	client := updateClient(3 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("status inesperado da API: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, releasesign.MaxMetadataBytes+1))
	if err != nil {
		return nil, false, err
	}
	if len(data) > releasesign.MaxMetadataBytes {
		return nil, false, fmt.Errorf("metadados de atualização excessivos")
	}
	var meta ReleaseMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, false, err
	}
	p, ok := meta.Platforms[platform]
	if !ok {
		return nil, false, fmt.Errorf("release assinada indisponível para esta plataforma")
	}
	if err := releasesign.Verify(p, platform, now, keys); err != nil {
		return nil, false, err
	}
	comparison, err := releasesign.CompareVersions(p.Version, current)
	if err != nil {
		return nil, false, err
	}
	if comparison < 0 {
		return nil, false, fmt.Errorf("release anterior à versão instalada foi recusada")
	}
	// Campos globais antigos não são autenticados. Somente o descriptor assinado
	// da plataforma atual decide versão, notas e o binário a instalar.
	meta.Version, meta.Notes, meta.ReleaseDate = p.Version, p.Notes, p.ReleasedAt
	meta.Platforms = map[string]PlatformRelease{platform: p}
	return &meta, comparison > 0, nil
}

// IsNewerVersion compara semanticamente duas versões (ex: 1.0.1 > 1.0.0)
func IsNewerVersion(remote, local string) bool {
	remote = strings.TrimPrefix(strings.TrimSpace(remote), "v")
	local = strings.TrimPrefix(strings.TrimSpace(local), "v")

	comparison, err := releasesign.CompareVersions(remote, local)
	return err == nil && comparison > 0
}

// AutoUpdate verifica e aplica a atualização automaticamente
func AutoUpdate(endpoint string, silent bool) (bool, error) {
	// 1. Limpa resíduos de atualizações anteriores (.old)
	CleanupOldBinaries()

	meta, hasNew, err := CheckForUpdate(endpoint)
	if err != nil {
		// Se offline ou timeout, não bloqueia o uso normal do Kofre
		if silent {
			return false, nil
		}
		return false, fmt.Errorf("não foi possível verificar uma atualização autenticada: %w", err)
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
	tmp, err := os.CreateTemp(dir, ".kofre-update-*.new")
	if err != nil {
		return false, err
	}
	tempPath := tmp.Name()
	if err = tmp.Chmod(0755); err != nil {
		tmp.Close()
		os.Remove(tempPath)
		return false, err
	}
	if err = tmp.Close(); err != nil {
		os.Remove(tempPath)
		return false, err
	}
	defer os.Remove(tempPath)
	oldPath := filepath.Join(dir, base+".old")

	downloadURL := platInfo.URL
	if !strings.HasPrefix(downloadURL, "http://") && !strings.HasPrefix(downloadURL, "https://") {
		if endpoint == "" {
			endpoint = config.GetCloudEndpoint()
		}
		downloadURL = strings.TrimRight(endpoint, "/") + "/" + strings.TrimLeft(downloadURL, "/")
	}

	if err := downloadRelease(downloadURL, tempPath, platInfo); err != nil {
		return false, fmt.Errorf("atualização recusada: %w", err)
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
