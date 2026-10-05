package compra

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func sessaoTeste(t *testing.T) (*Sessao, *atomic.Bool, *atomic.Int32) {
	t.Helper()
	paid := &atomic.Bool{}
	status := &atomic.Int32{}
	status.Store(200)
	var challenge string
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/v1/billing/activations" {
			var request map[string]string
			json.NewDecoder(r.Body).Decode(&request)
			challenge = request["verifier_hash"]
			if !segredoValido.MatchString(challenge) || !strings.HasPrefix(request["callback_url"], "http://127.0.0.1:") {
				t.Error("inicialização indevida")
			}
			json.NewEncoder(w).Encode(map[string]any{"session_id": strings.Repeat("b", 32), "browser_url": "https://kofre.dev/comprar?ativacao=" + strings.Repeat("b", 32) + "#vinculo=" + strings.Repeat("c", 64), "callback_code": strings.Repeat("d", 64), "expires_at": time.Now().Add(time.Hour)})
			return
		}
		if hash(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) != challenge {
			http.Error(w, "auth", 401)
			return
		}
		if status.Load() != 200 {
			w.WriteHeader(int(status.Load()))
			return
		}
		if r.Method == "POST" {
			w.Write([]byte(`{"completed":true}`))
			return
		}
		if paid.Load() {
			w.Write([]byte(`{"status":"active","token":"kfr_pro_` + strings.Repeat("e", 64) + `"}`))
		} else {
			w.Write([]byte(`{"status":"pending"}`))
		}
	}))
	t.Cleanup(gateway.Close)
	s, err := Iniciar(context.Background(), gateway.URL, "PC fictício")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, paid, status
}

func postRetorno(t *testing.T, s *Sessao, origin, code, host string, want int) {
	t.Helper()
	r, _ := http.NewRequest("POST", s.callback, strings.NewReader(url.Values{"code": {code}, "token": {"kfr_pro_falso"}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", origin)
	if host != "" {
		r.Host = host
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("retorno HTTP %d, esperado %d", resp.StatusCode, want)
	}
}

func TestRetornoLocalNaoConcedeLicenca(t *testing.T) {
	s, paid, _ := sessaoTeste(t)
	if strings.Contains(s.URL, s.verificador) || strings.Contains(s.URL, "kfr_pro_") {
		t.Fatal("URL expôs credencial")
	}
	postRetorno(t, s, "https://intruso.example", s.codigo, "", 403)
	postRetorno(t, s, "https://kofre.dev", "errado", "", 403)
	postRetorno(t, s, "https://kofre.dev", s.codigo, "intruso.example", 403)
	postRetorno(t, s, "", s.codigo, "", 403)
	postRetorno(t, s, "https://kofre.dev", s.codigo, "", 200)
	r, err := s.Consultar(context.Background())
	if err != nil || r.Status != "pending" || r.Token != "" {
		t.Fatal("callback local virou prova de pagamento")
	}
	paid.Store(true)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, err = s.Aguardar(ctx)
	if err != nil || r.Status != "active" || r.Token == "" {
		t.Fatal("pagamento do servidor não entregue")
	}
	if err = s.Confirmar(ctx); err != nil {
		t.Fatal(err)
	}
	s.Close()
	client := &http.Client{Timeout: time.Second}
	if resp, err := client.Get(s.callback); err == nil {
		resp.Body.Close()
		t.Fatal("listener continuou aberto")
	}
}

func TestSessaoExpiradaEncerraSemEsperar(t *testing.T) {
	s, _, status := sessaoTeste(t)
	status.Store(410)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := s.Aguardar(ctx)
	if err == nil || ctx.Err() != nil {
		t.Fatal("expiração ficou em repetição")
	}
}

func TestEndpointInseguroNaoIniciaCompra(t *testing.T) {
	for _, endpoint := range []string{"http://example.com", "http://192.168.1.2", "https://user:senha@example.com", "https://example.com?token=x"} {
		if _, err := Iniciar(context.Background(), endpoint, "teste"); err == nil {
			t.Fatal("endpoint indevido aceito")
		}
	}
}
