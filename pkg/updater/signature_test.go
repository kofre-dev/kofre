package updater

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kofre/pkg/releasesign"
)

func TestAtualizadorRecusaAdulteracaoRollbackEJSONExcessivo(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	p, err := releasesign.Sign(PlatformRelease{Version: "1.0.20", Platform: "windows-amd64", URL: "/v1/download/windows-amd64?version=1.0.20", SHA256: strings.Repeat("a", 64), Size: 1234, KeyID: releasesign.KeyID, ReleasedAt: now.Format(time.RFC3339), ExpiresAt: now.Add(time.Hour).Format(time.RFC3339), Notes: "Notas autenticadas"}, private, now)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]ed25519.PublicKey{releasesign.KeyID: public}
	meta := ReleaseMetadata{Version: "999.0.0", Notes: "Não autenticadas", Platforms: map[string]PlatformRelease{"windows-amd64": p}}
	body, _ := json.Marshal(meta)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer server.Close()
	got, newer, err := checkForUpdate(server.URL, p.Platform, "1.0.19", now, keys)
	if err != nil || !newer || got.Version != p.Version || got.Notes != p.Notes {
		t.Fatal("usou metadados globais sem assinatura", got, newer, err)
	}
	if _, newer, err := checkForUpdate(server.URL, p.Platform, p.Version, now, keys); err != nil || newer {
		t.Fatal("versão igual", newer, err)
	}
	if _, _, err := checkForUpdate(server.URL, p.Platform, "1.0.21", now, keys); err == nil {
		t.Fatal("aceitou downgrade")
	}
	p.Signature = ""
	meta.Platforms[p.Platform] = p
	body, _ = json.Marshal(meta)
	if _, _, err := checkForUpdate(server.URL, p.Platform, "1.0.19", now, keys); err == nil {
		t.Fatal("aceitou release sem assinatura")
	}
	body = []byte(strings.Repeat(" ", releasesign.MaxMetadataBytes+1))
	if _, _, err := checkForUpdate(server.URL, p.Platform, "1.0.19", now, keys); err == nil {
		t.Fatal("aceitou metadados excessivos")
	}
	body = []byte(`{} {}`)
	if _, _, err := checkForUpdate(server.URL, p.Platform, "1.0.19", now, keys); err == nil {
		t.Fatal("aceitou JSON com dados extras")
	}
}
