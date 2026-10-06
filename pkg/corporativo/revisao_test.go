package corporativo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRevisaoCancelamentoUsaRotaExataSemAbrirOutrasRotas(t *testing.T) {
	chamadas := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chamadas++
		if r.Method != "POST" || r.URL.Path != "/v1/billing/orders/"+strings.Repeat("a", 32)+"/cancel" || r.Header.Get("Authorization") != "Bearer token-ficticio" {
			t.Error("pedido divergente")
		}
		w.Write([]byte(`{"canceled":true}`))
	}))
	defer s.Close()
	i, _ := NovaIdentidade()
	defer i.Fechar()
	i.Token = "token-ficticio"
	c, err := NovoClient(s.URL, i)
	if err != nil {
		t.Fatal(err)
	}
	alvo := "/v1/billing/orders/" + strings.Repeat("a", 32) + "/cancel"
	if err := c.Request(context.Background(), "POST", alvo, nil, nil); err != nil {
		t.Fatal("cancelamento não chegou ao Cloud", err)
	}
	for _, path := range []string{"/v1/admin/login", alvo + "?x=1", "/v1/corporativo/../../v1/admin/login", "/v1/billing/orders/erro/cancel"} {
		if c.Request(context.Background(), "POST", path, nil, nil) == nil {
			t.Fatal("rota fora do contrato aceita", path)
		}
	}
	if chamadas != 1 {
		t.Fatal("rota recusada atingiu a rede", chamadas)
	}
}
