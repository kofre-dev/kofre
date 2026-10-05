package updater

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadRecusaChecksumAusenteAdulteracaoETamanho(t *testing.T) {
	binary := []byte("binário fictício")
	hash := sha256.Sum256(binary)
	p := PlatformRelease{SHA256: fmt.Sprintf("%x", hash), Size: int64(len(binary))}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(binary) }))
	defer server.Close()
	for _, tc := range []struct {
		name    string
		release PlatformRelease
		ok      bool
	}{
		{"válido", p, true}, {"checksum ausente", PlatformRelease{Size: p.Size}, false},
		{"checksum malformado", PlatformRelease{SHA256: "zz", Size: p.Size}, false},
		{"adulterado", PlatformRelease{SHA256: fmt.Sprintf("%064x", 1), Size: p.Size}, false},
		{"truncado", PlatformRelease{SHA256: p.SHA256, Size: p.Size + 1}, false},
		{"excedente", PlatformRelease{SHA256: p.SHA256, Size: p.Size - 1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "temporario")
			if err := os.WriteFile(path, nil, 0700); err != nil {
				t.Fatal(err)
			}
			err := downloadRelease(server.URL, path, tc.release)
			if (err == nil) != tc.ok {
				t.Fatalf("aceitação=%t, esperado=%t; %v", err == nil, tc.ok, err)
			}
		})
	}
	if secureURL("http://example.com/binario") == nil {
		t.Fatal("aceitou HTTP externo")
	}
}
