// Package conta implementa cadastro e sessões por e-mail. Senha mestra só é usada localmente.
package conta

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"kofre/pkg/corporativo"
)

type Client struct {
	endpoint string
	http     *http.Client
}
type Desafio struct {
	ID          string `json:"desafio_id"`
	Verificador string `json:"-"`
	Finalidade  string `json:"-"`
}
type Backup struct {
	ID       string `json:"id"`
	Cifra    []byte `json:"identidade_cifrada"`
	Mensagem string `json:"mensagem_assinatura"`
}
type Sessao struct {
	ID       string    `json:"id"`
	Token    string    `json:"token"`
	SessaoID string    `json:"sessao_id"`
	ExpiraEm time.Time `json:"expira_em"`
}
type Perfil struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	Nome       string `json:"nome"`
	Revisao    string `json:"backup_revisao"`
	BackupHash string `json:"backup_hash"`
}
type Dispositivo struct {
	ID       string    `json:"id"`
	Nome     string    `json:"dispositivo"`
	Atual    bool      `json:"atual"`
	ExpiraEm time.Time `json:"expira_em"`
}
type ErroHTTP struct{ Status int }

func (e *ErroHTTP) Error() string {
	return fmt.Sprintf("conta: solicitação recusada (HTTP %d)", e.Status)
}

func NovoClient(endpoint string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"))) {
		return nil, errors.New("a conta exige HTTPS")
	}
	return &Client{strings.TrimRight(endpoint, "/"), &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func EmailValido(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	a, err := mail.ParseAddress(email)
	if err != nil || a.Address != email || len(email) > 254 || strings.ContainsAny(email, "\r\n\x00") {
		return "", errors.New("informe um endereço de e-mail válido")
	}
	return email, nil
}
func hash(v []byte) string { h := sha256.Sum256(v); return hex.EncodeToString(h[:]) }
func (c *Client) request(ctx context.Context, metodo, path, token string, entrada, saida any) error {
	var body []byte
	var err error
	if entrada != nil {
		body, err = json.Marshal(entrada)
		if err != nil {
			return err
		}
		defer clear(body)
	}
	r, err := http.NewRequestWithContext(ctx, metodo, c.endpoint+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	r.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(r)
	if err != nil {
		return errors.New("não foi possível conectar à conta")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ErroHTTP{resp.StatusCode}
	}
	if saida == nil {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 360*1024+1))
	if err != nil {
		return err
	}
	defer clear(data)
	if len(data) > 360*1024 {
		return errors.New("resposta de conta excede limite")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if err = d.Decode(saida); err != nil {
		return errors.New("resposta de conta inválida")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("resposta de conta contém dados excedentes")
	}
	return nil
}
func (c *Client) iniciar(ctx context.Context, finalidade, email, token string, cadastro map[string]any) (Desafio, error) {
	var d Desafio
	normal, err := EmailValido(email)
	if err != nil {
		return d, err
	}
	var raw [32]byte
	if _, err = rand.Read(raw[:]); err != nil {
		return d, err
	}
	defer clear(raw[:])
	d.Verificador = hex.EncodeToString(raw[:])
	d.Finalidade = finalidade
	in := map[string]any{"email": normal, "verificador_hash": hash([]byte(d.Verificador))}
	for k, v := range cadastro {
		in[k] = v
	}
	err = c.request(ctx, "POST", "/v1/conta/"+finalidade, token, in, &d)
	if err == nil && !corporativo.IDValido(d.ID) {
		err = errors.New("desafio de conta inválido")
	}
	return d, err
}
func (c *Client) Cadastrar(ctx context.Context, i *corporativo.Identidade, cifra []byte) (Desafio, error) {
	pub, err := i.Publica()
	if err != nil {
		return Desafio{}, err
	}
	sig, err := i.PublicaAssinatura()
	if err != nil {
		return Desafio{}, err
	}
	return c.iniciar(ctx, "cadastro", i.Email, i.Token, map[string]any{"nome": i.Nome, "recuperacao_hash": hash([]byte(i.Recuperacao)), "chave_publica": pub, "chave_assinatura": sig, "identidade_cifrada": cifra})
}
func prova(d Desafio, codigo []byte) (map[string]string, error) {
	if !corporativo.IDValido(d.ID) || len(d.Verificador) != 64 || len(codigo) != 8 {
		return nil, errors.New("código de confirmação inválido")
	}
	for _, b := range codigo {
		if b < '0' || b > '9' {
			return nil, errors.New("o código deve conter oito números")
		}
	}
	return map[string]string{"desafio_id": d.ID, "codigo": string(codigo), "verificador": d.Verificador}, nil
}
func (c *Client) ConfirmarCadastro(ctx context.Context, d Desafio, codigo []byte, i *corporativo.Identidade, dispositivo string) (Sessao, error) {
	var sessao Sessao
	if d.Finalidade != "cadastro" {
		return sessao, errors.New("finalidade de desafio inválida")
	}
	in, err := prova(d, codigo)
	if err != nil {
		return sessao, err
	}
	sig, err := i.AssinarMensagem([]byte("kofre:cadastro:v1:" + d.ID + ":" + hash([]byte(d.Verificador))))
	if err != nil {
		return sessao, err
	}
	in["assinatura"] = sig
	in["dispositivo"] = dispositivo
	var out struct {
		Sessao
		Confirmado bool `json:"email_confirmado"`
	}
	err = c.request(ctx, "POST", "/v1/conta/email/confirmar", "", in, &out)
	if err == nil && (!out.Confirmado || !TokenSessaoValido(out.Token, out.ID, out.SessaoID) || !out.ExpiraEm.After(time.Now()) || out.ExpiraEm.After(time.Now().Add(31*24*time.Hour))) {
		err = errors.New("cadastro não confirmado")
	}
	return out.Sessao, err
}
func (c *Client) IniciarAcesso(ctx context.Context, email string, recuperar bool) (Desafio, error) {
	kind := "login"
	if recuperar {
		kind = "recuperacao"
	}
	return c.iniciar(ctx, kind, email, "", nil)
}
func (c *Client) ConfirmarAcesso(ctx context.Context, d Desafio, codigo []byte) (Backup, error) {
	var b Backup
	if d.Finalidade != "login" && d.Finalidade != "recuperacao" {
		return b, errors.New("finalidade de desafio inválida")
	}
	in, err := prova(d, codigo)
	if err != nil {
		return b, err
	}
	err = c.request(ctx, "POST", "/v1/conta/"+d.Finalidade+"/confirmar", "", in, &b)
	esperada := "kofre:" + d.Finalidade + ":v1:" + d.ID + ":" + hash([]byte(d.Verificador))
	if err == nil && (!corporativo.IDValido(b.ID) || b.Mensagem != esperada || len(b.Cifra) < 52 || !bytes.HasPrefix(b.Cifra, []byte("KFRID001"))) {
		err = errors.New("backup ou desafio inconsistente")
	}
	return b, err
}
func (c *Client) Concluir(ctx context.Context, d Desafio, b Backup, i *corporativo.Identidade, dispositivo string) (Sessao, error) {
	var s Sessao
	if i.ID != "" && i.ID != b.ID {
		return s, errors.New("backup pertence a outra conta")
	}
	in := map[string]string{"desafio_id": d.ID, "verificador": d.Verificador, "dispositivo": dispositivo}
	if d.Finalidade == "login" {
		sig, err := i.AssinarMensagem([]byte(b.Mensagem))
		if err != nil {
			return s, err
		}
		in["assinatura"] = sig
	} else if d.Finalidade == "recuperacao" {
		in["recuperacao"] = i.Recuperacao
	} else {
		return s, errors.New("finalidade de desafio inválida")
	}
	err := c.request(ctx, "POST", "/v1/conta/"+d.Finalidade+"/concluir", "", in, &s)
	if err == nil && (s.ID != b.ID || !TokenSessaoValido(s.Token, s.ID, s.SessaoID) || !s.ExpiraEm.After(time.Now()) || s.ExpiraEm.After(time.Now().Add(31*24*time.Hour))) {
		err = errors.New("sessão de conta inconsistente")
	}
	return s, err
}
func TokenSessaoValido(token, id, sid string) bool {
	p := strings.Split(strings.TrimPrefix(token, "kfr_sessao_"), "_")
	if !strings.HasPrefix(token, "kfr_sessao_") || len(p) != 3 || p[0] != id || p[1] != sid || !corporativo.IDValido(id) || !corporativo.IDValido(sid) {
		return false
	}
	b, e := hex.DecodeString(p[2])
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == p[2]
}
func (c *Client) Perfil(ctx context.Context, token string) (Perfil, error) {
	var p Perfil
	err := c.request(ctx, "GET", "/v1/conta/me", token, nil, &p)
	return p, err
}
func (c *Client) PublicarBackup(ctx context.Context, i *corporativo.Identidade, cifra []byte) error {
	p, err := c.Perfil(ctx, i.Token)
	if err != nil {
		return err
	}
	if p.ID != i.ID {
		return errors.New("backup pertence a outra conta")
	}
	sig, err := i.AssinarMensagem([]byte("kofre:backup:v1:" + i.ID + ":" + p.Revisao + ":" + hash(cifra)))
	if err != nil {
		return err
	}
	return c.request(ctx, "PUT", "/v1/conta/backup", i.Token, map[string]any{"identidade_cifrada": cifra, "revisao": p.Revisao, "assinatura": sig}, nil)
}

// PrepararBackup captura a revisão antes da gravação local. A fila não pode
// obter uma revisão nova para sobrescrever outra troca de senha no futuro.
func (c *Client) PrepararBackup(ctx context.Context, i *corporativo.Identidade, cifra []byte) ([]byte, error) {
	p, err := c.Perfil(ctx, i.Token)
	if err != nil {
		return nil, err
	}
	if p.ID != i.ID {
		return nil, errors.New("backup de outra conta")
	}
	sig, err := i.AssinarMensagem([]byte("kofre:backup:v1:" + i.ID + ":" + p.Revisao + ":" + hash(cifra)))
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		ID         string `json:"conta_id"`
		Cifra      []byte `json:"identidade_cifrada"`
		Revisao    string `json:"revisao"`
		Assinatura string `json:"assinatura"`
	}{i.ID, cifra, p.Revisao, sig})
}
func (c *Client) EnviarBackupPendente(ctx context.Context, i *corporativo.Identidade, payload []byte) error {
	var in struct {
		ID         string `json:"conta_id"`
		Cifra      []byte `json:"identidade_cifrada"`
		Revisao    string `json:"revisao"`
		Assinatura string `json:"assinatura"`
	}
	if json.Unmarshal(payload, &in) != nil || in.ID != i.ID {
		return errors.New("backup pendente inválido")
	}
	err := c.request(ctx, "PUT", "/v1/conta/backup", i.Token, map[string]any{"identidade_cifrada": in.Cifra, "revisao": in.Revisao, "assinatura": in.Assinatura}, nil)
	var recusado *ErroHTTP
	if errors.As(err, &recusado) && recusado.Status == http.StatusConflict {
		// Resposta perdida ou falha local após PUT: só confirma uma cifra
		// já aplicada. Nunca muda a revisão para sobrescrever outro backup.
		p, e := c.Perfil(ctx, i.Token)
		if e == nil && p.ID == i.ID && p.BackupHash == hash(in.Cifra) {
			return nil
		}
	}
	return err
}
func (c *Client) Sessoes(ctx context.Context, token string) ([]Dispositivo, error) {
	var out struct {
		Sessoes []Dispositivo `json:"sessoes"`
	}
	err := c.request(ctx, "GET", "/v1/conta/sessoes", token, nil, &out)
	return out.Sessoes, err
}
func (c *Client) Revogar(ctx context.Context, token, id string) error {
	if !corporativo.IDValido(id) {
		return errors.New("sessão inválida")
	}
	return c.request(ctx, "DELETE", "/v1/conta/sessoes/"+id, token, nil, nil)
}
func (c *Client) Logout(ctx context.Context, token string) error {
	return c.request(ctx, "POST", "/v1/conta/logout", token, nil, nil)
}
