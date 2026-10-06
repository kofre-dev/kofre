package conta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContaRecusaRedirectEBackupDeDesafioDiferente(t *testing.T) {
	chamadas := 0
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { chamadas++ }))
	defer dest.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, dest.URL, 307) }))
	defer s.Close()
	c, err := NovoClient(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Perfil(context.Background(), "token-ficticio"); err == nil || chamadas != 0 {
		t.Fatal("redirect recebeu credencial")
	}
	if _, err = NovoClient("http://fora.example"); err == nil {
		t.Fatal("HTTP externo aceito")
	}
	if _, err = EmailValido("teste@example.test\r\nBcc: outro@example.test"); err == nil {
		t.Fatal("injeção no e-mail aceita")
	}
	for _, token := range []string{"kfr_sessao_", "kfr_sessao_" + strings.Repeat("a", 32) + "_" + strings.Repeat("b", 32) + "_" + strings.Repeat("C", 64)} {
		if TokenSessaoValido(token, strings.Repeat("a", 32), strings.Repeat("b", 32)) {
			t.Fatal("token malformado aceito")
		}
	}
}
