// Package releasesign define o contrato público de releases autenticadas.
// Mantenha este arquivo idêntico no cliente e no publicador Cloud.
package releasesign

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const MaxMetadataBytes = 64 * 1024
const MaxBinaryBytes = 100 * 1024 * 1024
const KeyID = "kofre-release-2026-01"

// PublicKeyHex é a raiz de confiança compilada. Nunca carregue esta chave da API.
// O bootstrap de produção preenche esta constante antes da publicação.
const PublicKeyHex = "a08c941b86b17cbdf634c16ede1a07f7b307dc23e15262739a55d3c00de853b8"

type PlatformRelease struct {
	Version    string `json:"version"`
	URL        string `json:"url"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	Platform   string `json:"platform"`
	KeyID      string `json:"key_id"`
	ReleasedAt string `json:"released_at"`
	ExpiresAt  string `json:"expires_at"`
	Notes      string `json:"notes"`
	Signature  string `json:"signature"`
}

type Metadata struct {
	Version     string                     `json:"version"`
	ReleaseDate string                     `json:"release_date"`
	Notes       string                     `json:"notes"`
	Platforms   map[string]PlatformRelease `json:"platforms"`
}

func TrustedKeys() map[string]ed25519.PublicKey {
	keys := make(map[string]ed25519.PublicKey)
	if key, err := hex.DecodeString(PublicKeyHex); err == nil && len(key) == ed25519.PublicKeySize {
		keys[KeyID] = ed25519.PublicKey(key)
	}
	return keys
}

func ValidPlatform(platform string) bool {
	switch platform {
	case "windows-amd64", "linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64":
		return true
	}
	return false
}

func versionParts(version string) ([3]uint32, error) {
	var result [3]uint32
	parts := strings.Split(version, ".")
	if len(parts) != 3 || len(version) > 32 {
		return result, fmt.Errorf("versão inválida")
	}
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return result, fmt.Errorf("versão inválida")
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return result, fmt.Errorf("versão inválida")
			}
		}
		value, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return result, fmt.Errorf("versão inválida")
		}
		result[i] = uint32(value)
	}
	return result, nil
}

func CompareVersions(a, b string) (int, error) {
	pa, err := versionParts(a)
	if err != nil {
		return 0, err
	}
	pb, err := versionParts(b)
	if err != nil {
		return 0, err
	}
	for i := range pa {
		if pa[i] > pb[i] {
			return 1, nil
		}
		if pa[i] < pb[i] {
			return -1, nil
		}
	}
	return 0, nil
}

// signingBytes evita canonicalização ambígua de mapas JSON e separa o domínio.
// A assinatura cobre também a plataforma e o caminho imutável do download.
func signingBytes(p PlatformRelease) []byte {
	fields := []any{"kofre:release:v1", p.KeyID, p.Platform, p.Version, p.URL, p.SHA256, p.Size, p.ReleasedAt, p.ExpiresAt, p.Notes}
	data, _ := json.Marshal(fields)
	return data
}

func validate(p PlatformRelease, platform string, now time.Time) error {
	if !ValidPlatform(platform) || p.Platform != platform {
		return fmt.Errorf("plataforma do manifesto inválida")
	}
	if _, err := versionParts(p.Version); err != nil {
		return err
	}
	if p.URL != "/v1/download/"+platform+"?version="+p.Version {
		return fmt.Errorf("URL não corresponde à versão assinada")
	}
	hash, err := hex.DecodeString(p.SHA256)
	if err != nil || len(hash) != 32 || p.SHA256 != strings.ToLower(p.SHA256) {
		return fmt.Errorf("SHA-256 inválido")
	}
	if p.Size <= 0 || p.Size > MaxBinaryBytes {
		return fmt.Errorf("tamanho de release inválido")
	}
	if len(p.KeyID) > 64 || len(p.Notes) > 4096 {
		return fmt.Errorf("manifesto excessivo")
	}
	for _, r := range p.Notes {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return fmt.Errorf("notas contêm controle de terminal")
		}
	}
	released, err := time.Parse(time.RFC3339, p.ReleasedAt)
	if err != nil {
		return fmt.Errorf("data de publicação inválida")
	}
	expires, err := time.Parse(time.RFC3339, p.ExpiresAt)
	if err != nil || !expires.After(released) || expires.Sub(released) > 370*24*time.Hour {
		return fmt.Errorf("validade do manifesto inválida")
	}
	if released.After(now.Add(24*time.Hour)) || !expires.After(now) {
		return fmt.Errorf("manifesto expirado ou com data futura")
	}
	return nil
}

func Verify(p PlatformRelease, platform string, now time.Time, keys map[string]ed25519.PublicKey) error {
	if err := validate(p, platform, now); err != nil {
		return err
	}
	key, exists := keys[p.KeyID]
	if !exists || len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("chave de publicação desconhecida")
	}
	signature, err := hex.DecodeString(p.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize || !ed25519.Verify(key, signingBytes(p), signature) {
		return fmt.Errorf("assinatura da release ausente ou inválida")
	}
	return nil
}

func Sign(p PlatformRelease, private ed25519.PrivateKey, now time.Time) (PlatformRelease, error) {
	if len(private) != ed25519.PrivateKeySize {
		return p, fmt.Errorf("chave privada inválida")
	}
	if err := validate(p, p.Platform, now); err != nil {
		return p, err
	}
	p.Signature = hex.EncodeToString(ed25519.Sign(private, signingBytes(p)))
	return p, nil
}
