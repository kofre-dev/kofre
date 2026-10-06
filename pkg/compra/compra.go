// Package compra coordena o pagamento no navegador e o retorno temporário local.
package compra

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var idValido = regexp.MustCompile(`^[a-f0-9]{32}$`)
var segredoValido = regexp.MustCompile(`^[a-f0-9]{64}$`)

type falhaHTTP struct{ status int }

func (e *falhaHTTP) Error() string {
	return fmt.Sprintf("não foi possível consultar a compra (HTTP %d)", e.status)
}

type Resultado struct {
	ContaID       string `json:"conta_id,omitempty"`
	OrganizacaoID string `json:"organizacao_id,omitempty"`
	Status        string `json:"status"`
	Token         string `json:"token"`
	OrderID       string `json:"order_id"`
	Sandbox       bool   `json:"sandbox"`
}

type Sessao struct {
	contaID     string
	contaToken  string
	produto     string
	ID          string
	URL         string
	ExpiresAt   time.Time
	endpoint    string
	verificador string
	codigo      string
	callback    string
	client      *http.Client
	server      *http.Server
	wake        chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
	closeOnce   sync.Once
}

func aleatorio(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func hash(v string) string { b := sha256.Sum256([]byte(v)); return hex.EncodeToString(b[:]) }

func endpointSeguro(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Hostname() == "" {
		return false
	}
	return u.Scheme == "https" || (u.Scheme == "http" && u.Hostname() == "127.0.0.1")
}

func Iniciar(ctx context.Context, endpoint, computador string) (*Sessao, error) {
	return iniciarCompra(ctx, endpoint, computador, "", "", "")
}

// Vincula pagamento à identidade existente; o recibo não recebe sua credencial.
func IniciarComConta(ctx context.Context, endpoint, computador, token, contaID, produto string) (*Sessao, error) {
	if !idValido.MatchString(contaID) || !strings.HasPrefix(token, "kfr_conta_") || len(token) > 128 || (produto != "pro" && produto != "corporativo") {
		return nil, errors.New("conta ou produto inválido para compra")
	}
	return iniciarCompra(ctx, endpoint, computador, token, contaID, produto)
}

func RenovarEmpresa(ctx context.Context, endpoint, computador, token, contaID, org string) (*Sessao, error) {
	if !idValido.MatchString(org) || !idValido.MatchString(contaID) || !strings.HasPrefix(token, "kfr_conta_") {
		return nil, errors.New("conta ou organização inválida")
	}
	return iniciarCompra(ctx, endpoint, computador, token, contaID, "corporativo", org)
}

func iniciarCompra(ctx context.Context, endpoint, computador, token, contaID, produto string, organizacao ...string) (*Sessao, error) {
	if !endpointSeguro(endpoint) {
		return nil, errors.New("a compra exige um endpoint HTTPS confiável")
	}
	verificador, err := aleatorio(32)
	if err != nil {
		return nil, err
	}
	state, err := aleatorio(16)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, errors.New("não foi possível preparar o retorno local da compra")
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Minute)
	s := &Sessao{endpoint: strings.TrimRight(endpoint, "/"), verificador: verificador, callback: "http://" + listener.Addr().String() + "/retorno/" + state, wake: make(chan struct{}, 1), ctx: ctx, cancel: cancel, client: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	s.contaID, s.contaToken, s.produto = contaID, token, produto
	var reply struct {
		SessionID    string    `json:"session_id"`
		BrowserURL   string    `json:"browser_url"`
		CallbackCode string    `json:"callback_code"`
		ExpiresAt    time.Time `json:"expires_at"`
	}
	entrada := map[string]string{"verifier_hash": hash(verificador), "callback_url": s.callback, "device_name": computador, "produto": produto}
	if len(organizacao) > 0 {
		entrada["organizacao_id"] = organizacao[0]
	}
	err = s.request(ctx, "POST", "/v1/billing/activations", entrada, &reply)
	s.contaToken = ""
	u, parseErr := url.Parse(reply.BrowserURL)
	if err == nil && (parseErr != nil || u.Scheme != "https" || u.Host != "kofre.dev" || u.User != nil || u.Path != "/comprar" || u.Query().Get("ativacao") != reply.SessionID || !idValido.MatchString(reply.SessionID) || !segredoValido.MatchString(reply.CallbackCode) || !segredoValido.MatchString(urlValuesFragment(u).Get("vinculo")) || !reply.ExpiresAt.After(time.Now()) || reply.ExpiresAt.After(time.Now().Add(91*time.Minute))) {
		err = errors.New("resposta de compra inválida")
	}
	if err != nil {
		listener.Close()
		cancel()
		return nil, err
	}
	s.ID = reply.SessionID
	s.URL = reply.BrowserURL
	s.codigo = reply.CallbackCode
	s.ExpiresAt = reply.ExpiresAt
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+uPath(s.callback), s.receberRetorno)
	s.server = &http.Server{Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8 * 1024}
	go func() { _ = s.server.Serve(listener) }()
	go func() { <-ctx.Done(); s.Close() }()
	return s, nil
}

func urlValuesFragment(u *url.URL) url.Values { v, _ := url.ParseQuery(u.Fragment); return v }
func (s *Sessao) Endpoint() string            { return s.endpoint }

func uPath(raw string) string { u, _ := url.Parse(raw); return u.Path }

func (s *Sessao) request(ctx context.Context, method, path string, body, result any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.endpoint+path, reader)
	if err != nil {
		return errors.New("requisição de compra inválida")
	}
	if s.ID != "" {
		req.Header.Set("Authorization", "Bearer "+s.verificador)
	} else if s.contaToken != "" {
		req.Header.Set("Authorization", "Bearer "+s.contaToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return errors.New("serviço de compra indisponível")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return &falhaHTTP{status: resp.StatusCode}
	}
	if result != nil && json.NewDecoder(io.LimitReader(resp.Body, 16*1024)).Decode(result) != nil {
		return errors.New("resposta de compra inválida")
	}
	return nil
}

func (s *Sessao) receberRetorno(w http.ResponseWriter, r *http.Request) {
	u, _ := url.Parse(s.callback)
	// null pode ser enviado em formulário com Referrer-Policy: no-referrer.
	origin := r.Header.Get("Origin")
	if r.Host != u.Host || (origin != "https://kofre.dev" && origin != "null") {
		// Consome somente o corpo limitado antes de responder. Deixar dados
		// pendentes pode encerrar TCP com reset no Windows e ocultar o HTTP 403.
		_, _ = io.Copy(io.Discard, http.MaxBytesReader(w, r.Body, 2048))
		http.Error(w, "retorno não autorizado", 403)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	if r.ParseForm() != nil || subtle.ConstantTimeCompare([]byte(r.PostForm.Get("code")), []byte(s.codigo)) != 1 {
		http.Error(w, "código de retorno inválido", 403)
		return
	}
	if s.ctx.Err() != nil {
		http.Error(w, "compra encerrada", 410)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors https://kofre.dev")
	w.Write([]byte(`<!doctype html><html lang="pt-BR"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Volte ao Kofre</title><body style="background:#030712;color:#e2e8f0;font:18px/1.6 system-ui;padding:10vh 8vw"><h1>Pode voltar ao Kofre.</h1><p>O aplicativo recebeu o retorno e está conferindo a ativação com o servidor. Você não precisa copiar a licença.</p><p>Você pode fechar esta aba.</p></body></html>`))
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Sessao) Consultar(ctx context.Context) (Resultado, error) {
	var result Resultado
	err := s.request(ctx, "GET", "/v1/billing/activations/"+s.ID, nil, &result)
	if err == nil && result.Status == "active" {
		valido := !result.Sandbox
		if s.contaID != "" {
			valido = valido && result.ContaID == s.contaID && result.Token == ""
			if s.produto == "corporativo" {
				valido = valido && idValido.MatchString(result.OrganizacaoID)
			} else {
				valido = valido && result.OrganizacaoID == ""
			}
		} else {
			valido = valido && strings.HasPrefix(result.Token, "kfr_pro_") && len(result.Token) <= 128 && result.ContaID == "" && result.OrganizacaoID == ""
		}
		if !valido {
			return Resultado{}, errors.New("ativação de compra inválida")
		}
	}
	return result, err
}

func (s *Sessao) Aguardar(ctx context.Context) (Resultado, error) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		result, err := s.Consultar(ctx)
		var httpErr *falhaHTTP
		if errors.As(err, &httpErr) && (httpErr.status == 401 || httpErr.status == 403 || httpErr.status == 404 || httpErr.status == 410) {
			return Resultado{}, err
		}
		if err == nil {
			switch result.Status {
			case "active", "sandbox_confirmed":
				return result, nil
			case "canceled", "expired", "refunded", "completed":
				return Resultado{}, errors.New("compra encerrada; consulte o pedido no site")
			}
		}
		select {
		case <-ctx.Done():
			return Resultado{}, ctx.Err()
		case <-s.ctx.Done():
			return Resultado{}, errors.New("espera de compra encerrada; a licença pode ser recuperada no site")
		case <-s.wake:
		case <-ticker.C:
		}
	}
}

func (s *Sessao) Confirmar(ctx context.Context) error {
	return s.request(ctx, "POST", "/v1/billing/activations/"+s.ID+"/complete", nil, nil)
}
func (s *Sessao) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		if s.server != nil {
			_ = s.server.Close()
		}
	})
}
