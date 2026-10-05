package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func secureURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.User != nil || u.Host == "" {
		return fmt.Errorf("URL de atualização inválida")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" {
		ip := net.ParseIP(u.Hostname())
		if ip != nil && ip.IsLoopback() {
			return nil
		}
	}
	return fmt.Errorf("atualização exige HTTPS; HTTP só é aceito em loopback para testes")
}
func updateClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("redirecionamentos excessivos")
		}
		return secureURL(req.URL.String())
	}}
}
func validateRelease(p PlatformRelease) error {
	hash, err := hex.DecodeString(p.SHA256)
	if err != nil || len(hash) != sha256.Size {
		return fmt.Errorf("checksum SHA-256 obrigatório e válido")
	}
	if p.Size <= 0 || p.Size > 100*1024*1024 {
		return fmt.Errorf("tamanho de atualização inválido")
	}
	return nil
}

// O arquivo temporário só pode ser promovido depois da verificação completa.
func downloadRelease(raw, path string, p PlatformRelease) error {
	if err := validateRelease(p); err != nil {
		return err
	}
	if err := secureURL(raw); err != nil {
		return err
	}
	resp, err := updateClient(60 * time.Second).Get(raw)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("erro no download: HTTP %d", resp.StatusCode)
	}
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, hash), io.LimitReader(resp.Body, p.Size+1))
	if err != nil {
		return err
	}
	if n != p.Size {
		return fmt.Errorf("tamanho do binário diverge dos metadados")
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), p.SHA256) {
		return fmt.Errorf("checksum não confere")
	}
	if err = out.Sync(); err != nil {
		return err
	}
	return out.Close()
}
