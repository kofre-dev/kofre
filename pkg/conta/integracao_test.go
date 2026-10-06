package conta

import (
	"bytes"
	"context"
	"encoding/json"
	"kofre/pkg/corporativo"
	mycrypto "kofre/pkg/crypto"
	"net/http"
	"os"
	"strings"
	"testing"
)

// O Cloud inicia este teste com SQLite real e uma caixa de e-mail local simulada.
func TestContaCloudIntegracao(t *testing.T) {
	path := os.Getenv("KOFRE_TEST_CONTA_EMAIL")
	if path == "" {
		t.Skip("integração iniciada pelo Cloud")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct{ Endpoint string }
	if json.Unmarshal(raw, &cfg) != nil {
		t.Fatal("configuração inválida")
	}
	c, err := NovoClient(cfg.Endpoint)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	codigo := func() []byte {
		t.Helper()
		r, e := http.Get(cfg.Endpoint + "/__teste/codigo")
		if e != nil {
			t.Fatal(e)
		}
		defer r.Body.Close()
		var msg struct{ Codigo string }
		if e = json.NewDecoder(r.Body).Decode(&msg); e != nil {
			t.Fatal(e)
		}
		return []byte(msg.Codigo)
	}
	i, err := corporativo.PrepararCadastro("Pessoa de teste")
	if err != nil {
		t.Fatal(err)
	}
	defer i.Fechar()
	i.Email = "cli@example.test"
	key, salt, err := mycrypto.DeriveKeyBytes([]byte("senha-mestra-ficticia"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(key)
	cifra, err := corporativo.CifrarIdentidadeComChave(i, key, salt)
	if err != nil {
		t.Fatal(err)
	}
	d, err := c.Cadastrar(ctx, i, cifra)
	if err != nil {
		t.Fatal(err)
	}
	code := codigo()
	root := i.Token
	sessao, err := c.ConfirmarCadastro(ctx, d, code, i, "PC cadastro")
	if err != nil {
		t.Fatal(err)
	}
	i.ID = sessao.ID
	i.Token = sessao.Token
	if _, err = c.Perfil(ctx, root); err == nil {
		t.Fatal("raiz de cadastro permaneceu autorizada")
	}
	if _, err = c.ConfirmarCadastro(ctx, d, code, i, "PC cadastro"); err == nil {
		t.Fatal("replay aceito")
	}
	if err = c.PublicarBackup(ctx, i, cifra); err != nil {
		t.Fatal(err)
	}
	login := func(recuperar bool) (*corporativo.Identidade, Sessao) {
		t.Helper()
		d, e := c.IniciarAcesso(ctx, i.Email, recuperar)
		if e != nil {
			t.Fatal(e)
		}
		b, e := c.ConfirmarAcesso(ctx, d, codigo())
		if e != nil {
			t.Fatal(e)
		}
		if errado, e := corporativo.AbrirIdentidadeBytes(b.Cifra, []byte("senha-errada")); e == nil {
			errado.Fechar()
			t.Fatal("senha incorreta abriu identidade")
		}
		restored, e := corporativo.AbrirIdentidadeBytes(b.Cifra, []byte("senha-mestra-ficticia"))
		if e != nil {
			t.Fatal(e)
		}
		restored.ID = b.ID
		if e = restored.ProtegerPrivada(); e != nil {
			t.Fatal(e)
		}
		s, e := c.Concluir(ctx, d, b, restored, "PC de teste")
		if e != nil {
			restored.Fechar()
			t.Fatal(e)
		}
		restored.Token = s.Token
		if _, e = c.Concluir(ctx, d, b, restored, "PC de teste"); e == nil {
			t.Fatal("conclusão repetida autorizada")
		}
		return restored, s
	}
	first, a := login(false)
	defer first.Fechar()
	second, b := login(false)
	defer second.Fechar()
	devices, err := c.Sessoes(ctx, a.Token)
	if err != nil || len(devices) != 3 {
		t.Fatal("sessões não listadas", err)
	}
	if err = c.Revogar(ctx, a.Token, b.SessaoID); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Perfil(ctx, b.Token); err == nil {
		t.Fatal("sessão revogada foi autorizada")
	}
	if _, err = c.Perfil(ctx, a.Token); err != nil {
		t.Fatal("revogação atingiu sessão diferente", err)
	}
	pendente, err := c.PrepararBackup(ctx, first, cifra)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.EnviarBackupPendente(ctx, first, pendente); err != nil {
		t.Fatal(err)
	}
	if err = c.EnviarBackupPendente(ctx, first, pendente); err != nil {
		t.Fatal("repetição de backup já aplicado não foi reconciliada", err)
	}
	cifraOutra, err := corporativo.CifrarIdentidadeComChave(first, key, salt)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.PublicarBackup(ctx, first, cifraOutra); err != nil {
		t.Fatal(err)
	}
	if err = c.EnviarBackupPendente(ctx, first, pendente); err == nil {
		t.Fatal("fila antiga sobrescreveu uma cifra mais recente")
	}
	if bytes.Contains(pendente, []byte(first.Token)) || bytes.Contains(pendente, []byte(first.Recuperacao)) {
		t.Fatal("fila contém credencial aberta")
	}
	if err = c.Logout(ctx, a.Token); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Perfil(ctx, a.Token); err == nil {
		t.Fatal("logout não revogou")
	}
	recovered, r := login(true)
	defer recovered.Fechar()
	if _, err = c.Perfil(ctx, i.Token); err == nil {
		t.Fatal("recuperação preservou raiz anterior")
	}
	if _, err = c.Perfil(ctx, r.Token); err != nil {
		t.Fatal(err)
	}
	if !corporativo.CredencialContaValida(r.Token) || corporativo.CredencialContaValida(strings.Replace(r.Token, "_", "/", 1)) {
		t.Fatal("formato de sessão inconsistente")
	}
}
