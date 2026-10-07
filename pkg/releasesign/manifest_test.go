package releasesign

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
	"time"
)

func TestRaizPublicaConfigurada(t *testing.T) {
	if len(TrustedKeys()[KeyID]) != ed25519.PublicKeySize {
		t.Fatal("raiz de publicação ausente ou inválida")
	}
}

func TestManifestoAutenticaTodosCamposEExigeValidade(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	p := PlatformRelease{Version: "1.0.20", Platform: "windows-amd64", URL: "/v1/download/windows-amd64?version=1.0.20", SHA256: strings.Repeat("a", 64), Size: 1234, KeyID: KeyID, ReleasedAt: now.Format(time.RFC3339), ExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339), Notes: "Correção de segurança"}
	p, err = Sign(p, private, now)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]ed25519.PublicKey{KeyID: public}
	if err := Verify(p, p.Platform, now, keys); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*PlatformRelease){
		"versão":              func(p *PlatformRelease) { p.Version = "1.0.21" },
		"plataforma":          func(p *PlatformRelease) { p.Platform = "linux-amd64" },
		"URL":                 func(p *PlatformRelease) { p.URL = "https://evil.invalid/app" },
		"hash":                func(p *PlatformRelease) { p.SHA256 = strings.Repeat("b", 64) },
		"tamanho":             func(p *PlatformRelease) { p.Size++ },
		"data":                func(p *PlatformRelease) { p.ReleasedAt = now.Add(-time.Hour).Format(time.RFC3339) },
		"expiração":           func(p *PlatformRelease) { p.ExpiresAt = now.Add(2 * time.Hour).Format(time.RFC3339) },
		"notas":               func(p *PlatformRelease) { p.Notes = "Adulteradas" },
		"sem assinatura":      func(p *PlatformRelease) { p.Signature = "" },
		"assinatura inválida": func(p *PlatformRelease) { p.Signature = strings.Repeat("0", 128) },
		"chave desconhecida":  func(p *PlatformRelease) { p.KeyID = "outra" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := p
			mutate(&changed)
			if Verify(changed, p.Platform, now, keys) == nil {
				t.Fatal("aceitou manifesto adulterado")
			}
		})
	}
	if Verify(p, p.Platform, now.Add(24*time.Hour), keys) == nil {
		t.Fatal("aceitou manifesto expirado")
	}
	if Verify(p, p.Platform, now.Add(-48*time.Hour), keys) == nil {
		t.Fatal("aceitou manifesto futuro")
	}
	if Verify(p, p.Platform, now, TrustedKeys()) == nil {
		t.Fatal("confiou em chave de fixture")
	}
}

func TestVersoesEstritas(t *testing.T) {
	for _, v := range []string{"1.0", "1.0.0.1", "1.0.x", "1.0.01", "-1.0.0", "9999999999999.0.0", "1.0.0-beta", "v1.0.0"} {
		if _, err := CompareVersions(v, "1.0.0"); err == nil {
			t.Errorf("aceitou %q", v)
		}
	}
	for _, tc := range []struct {
		a, b   string
		result int
	}{{"1.0.20", "1.0.19", 1}, {"1.0.19", "1.0.20", -1}, {"2.0.0", "1.99.99", 1}, {"1.0.0", "1.0.0", 0}} {
		if result, err := CompareVersions(tc.a, tc.b); err != nil || result != tc.result {
			t.Fatal(tc, result, err)
		}
	}
}
