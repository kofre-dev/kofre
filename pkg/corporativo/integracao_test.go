package corporativo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestClientCloudIntegration(t *testing.T) {
	path := os.Getenv("KOFRE_TEST_CORPORATIVO")
	if path == "" {
		t.Skip("integração iniciada pelo teste do Cloud")
	}
	var cfg struct{ Endpoint, Admin string }
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	admin := func(path string, input any) map[string]any {
		t.Helper()
		b, _ := json.Marshal(input)
		r, _ := http.NewRequest("POST", cfg.Endpoint+path, bytes.NewReader(b))
		r.Header.Set("X-Admin-Secret", cfg.Admin)
		r.Header.Set("Content-Type", "application/json")
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("admin HTTP %d", resp.StatusCode)
		}
		var result map[string]any
		if e = json.NewDecoder(resp.Body).Decode(&result); e != nil {
			t.Fatal(e)
		}
		return result
	}
	criar := func(n string, nonce string) *Client {
		i, e := NovaIdentidade()
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(i.Fechar)
		pub, _ := i.Publica()
		sign, _ := i.PublicaAssinatura()
		r := admin("/v1/corporativo/piloto/identidades", map[string]any{"nome": n, "solicitacao_id": nonce, "chave_publica": pub, "chave_assinatura": sign})
		i.ID = r["id"].(string)
		i.Token = r["token"].(string)
		i.Recuperacao = r["recuperacao"].(string)
		client, e := NovoClient(cfg.Endpoint, i)
		if e != nil {
			t.Fatal(e)
		}
		client.ConfirmarAutor = func(string, Pessoa, string) bool { return true }
		return client
	}
	a := criar("Ana", strings.Repeat("a", 32))
	d := criar("Bruno", strings.Repeat("b", 32))
	terceiro := criar("Carlos", strings.Repeat("c", 32))
	orgDados := admin("/v1/corporativo/piloto/organizacoes", map[string]any{"nome": "Empresa fictícia", "plano": "corporate", "assentos": 3, "proprietario": a.Identidade.ID, "validade": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "solicitacao_id": strings.Repeat("d", 32)})
	org := orgDados["id"].(string)
	base := "/v1/corporativo/organizacoes/" + org
	for _, p := range []*Client{d, terceiro} {
		var invite map[string]string
		if err = a.Request(ctx, "POST", base+"/convites", map[string]string{"destinatario": p.Identidade.ID}, &invite); err != nil {
			t.Fatal(err)
		}
		if err = p.Request(ctx, "POST", base+"/aceitar", map[string]string{"codigo": invite["codigo"]}, nil); err != nil {
			t.Fatal(err)
		}
	}
	workspace, item, equipe := strings.Repeat("e", 32), strings.Repeat("f", 32), strings.Repeat("1", 32)
	if err = a.Request(ctx, "PUT", base+"/workspaces/"+workspace, Workspace{"Projeto fictício"}, nil); err != nil {
		t.Fatal(err)
	}
	if err = a.Request(ctx, "PUT", base+"/equipes/"+equipe, Equipe{"Desenvolvimento", []string{d.Identidade.ID}}, nil); err != nil {
		t.Fatal(err)
	}
	acl := Acesso{map[string]string{a.Identidade.ID: "gestor"}, map[string]string{equipe: "editor"}}
	plaintext := []byte(`{"senha":"ficticia-sem-valor"}`)
	confirmar := func(string, Pessoa, string) bool { return true }
	if err = a.Gravar(ctx, org, item, workspace, 1, acl, plaintext, confirmar); err != nil {
		t.Fatal(err)
	}
	aberto, recebido, err := d.Ler(ctx, org, item)
	if err != nil || !bytes.Equal(aberto, plaintext) {
		t.Fatal("destinatário não abriu", err)
	}
	clear(aberto)
	if _, _, err = terceiro.Ler(ctx, org, item); err == nil {
		t.Fatal("terceiro abriu segredo")
	}
	if _, err = Abrir(terceiro.Identidade.Privada, org, item, d.Identidade.ID, 1, recebido.Copia.Cifra); err == nil {
		t.Fatal("terceiro decifrou cópia alheia")
	}
	// Editor modifica conteúdo, mas não concede permissões por conta própria.
	alterada := Acesso{map[string]string{a.Identidade.ID: "gestor", terceiro.Identidade.ID: "leitor"}, map[string]string{equipe: "editor"}}
	if err = d.Gravar(ctx, org, item, workspace, 2, alterada, plaintext, confirmar); err == nil {
		t.Fatal("editor concedeu acesso")
	}
	if err = d.Gravar(ctx, org, item, workspace, 2, acl, []byte("nova credencial ficticia"), confirmar); err != nil {
		t.Fatal("editor não atualizou", err)
	}
	if err = a.Gravar(ctx, org, item, workspace, 2, acl, plaintext, confirmar); err == nil {
		t.Fatal("versão obsoleta aceita")
	}
	// Alteração de equipe não inventa uma cópia para o integrante novo.
	if err = a.Request(ctx, "PUT", base+"/equipes/"+equipe, Equipe{"Desenvolvimento", []string{d.Identidade.ID, terceiro.Identidade.ID}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err = terceiro.Ler(ctx, org, item); err == nil {
		t.Fatal("entrada na equipe criou acesso sem cifra")
	}
	if err = a.Gravar(ctx, org, item, workspace, 3, acl, plaintext, confirmar); err != nil {
		t.Fatal(err)
	}
	if _, _, err = terceiro.Ler(ctx, org, item); err != nil {
		t.Fatal("recompartilhamento não entregou cópia", err)
	}
	// Recuperação troca autenticação sem mudar chaves nem direitos.
	antigo := d.Identidade.Token
	var recovery struct {
		Token string `json:"token"`
		ID    string `json:"id"`
	}
	input := map[string]string{"id": d.Identidade.ID, "recuperacao": d.Identidade.Recuperacao, "solicitacao_id": strings.Repeat("2", 32)}
	if err = d.Request(ctx, "POST", "/v1/corporativo/identidades/recuperar", input, &recovery); err != nil {
		t.Fatal(err)
	}
	if recovery.ID != d.Identidade.ID || recovery.Token == antigo {
		t.Fatal("recuperação alterou ID ou não trocou token")
	}
	if _, _, err = d.Ler(ctx, org, item); err == nil {
		t.Fatal("token antigo continuou válido")
	}
	d.Identidade.Token = recovery.Token
	if _, _, err = d.Ler(ctx, org, item); err != nil {
		t.Fatal("recuperação perdeu cifra", err)
	}
	// Retirar membro remove suas cópias e direitos; token individual permanece.
	if err = a.Request(ctx, "DELETE", base+"/membros/"+d.Identidade.ID, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err = d.Ler(ctx, org, item); err == nil {
		t.Fatal("membro removido continua lendo")
	}
	var identidade map[string]any
	if err = d.Request(ctx, "GET", "/v1/corporativo/identidade", nil, &identidade); err != nil {
		t.Fatal("remoção apagou identidade", err)
	}
	// Troca de chave do diretório é recusada pelo pin local já confirmado.
	original := a.Identidade.Pins[terceiro.Identidade.ID]
	a.Identidade.Pins[terceiro.Identidade.ID] = strings.Repeat("0", 64)
	if err = a.Gravar(ctx, org, item, workspace, 4, acl, plaintext, confirmar); err == nil {
		t.Fatal("fingerprint divergente aceito")
	}
	a.Identidade.Pins[terceiro.Identidade.ID] = original
	// Confere que respostas administrativas e recursos não contêm texto secreto.
	recursos, err := a.Recursos(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(recursos)
	if bytes.Contains(encoded, plaintext) {
		t.Fatal("diretório expõe segredo")
	}
	if err = a.Request(ctx, "PUT", base+"/proprietario", map[string]string{"novo_proprietario": terceiro.Identidade.ID, "confirmacao": "TRANSFERIR"}, nil); err != nil {
		t.Fatal(err)
	}
	// A mesma pessoa pode participar de duas empresas sem misturar seus itens.
	// Reutilizar IDs de workspace e item expõe regressões na chave de isolamento.
	outra := admin("/v1/corporativo/piloto/organizacoes", map[string]any{"nome": "Outra empresa fictícia", "plano": "corporate", "assentos": 3, "proprietario": terceiro.Identidade.ID, "validade": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "solicitacao_id": strings.Repeat("3", 32)})
	org2 := outra["id"].(string)
	base2 := "/v1/corporativo/organizacoes/" + org2
	var convite2 map[string]string
	if err = terceiro.Request(ctx, "POST", base2+"/convites", map[string]string{"destinatario": a.Identidade.ID}, &convite2); err != nil {
		t.Fatal(err)
	}
	if err = a.Request(ctx, "POST", base2+"/aceitar", map[string]string{"codigo": convite2["codigo"]}, nil); err != nil {
		t.Fatal(err)
	}
	if err = terceiro.Request(ctx, "PUT", base2+"/workspaces/"+workspace, Workspace{"Outro projeto"}, nil); err != nil {
		t.Fatal(err)
	}
	segredo2 := []byte(`{"senha":"somente-segunda-empresa"}`)
	acesso2 := Acesso{Pessoas: map[string]string{terceiro.Identidade.ID: "gestor", a.Identidade.ID: "leitor"}, Equipes: map[string]string{}}
	if err = terceiro.Gravar(ctx, org2, item, workspace, 1, acesso2, segredo2, confirmar); err != nil {
		t.Fatal(err)
	}
	for _, caso := range []struct {
		org      string
		esperado []byte
	}{{org, plaintext}, {org2, segredo2}} {
		b, _, e := a.Ler(ctx, caso.org, item)
		if e != nil || !bytes.Equal(b, caso.esperado) {
			clear(b)
			t.Fatal("credenciais misturadas entre organizações", e)
		}
		clear(b)
	}
	if _, _, err = d.Ler(ctx, org2, item); err == nil {
		t.Fatal("usuário externo leu a segunda empresa")
	}
	if err = a.Gravar(ctx, org2, item, workspace, 2, acesso2, plaintext, confirmar); err == nil {
		t.Fatal("leitor editou item da segunda empresa")
	}
	if err = a.Request(ctx, "DELETE", base2+"/segredos/"+item, nil, nil); err == nil {
		t.Fatal("leitor excluiu item da segunda empresa")
	}
	if err = a.Request(ctx, "POST", base2+"/convites", map[string]string{"destinatario": d.Identidade.ID}, nil); err == nil {
		t.Fatal("membro convidou sem administração")
	}
	// Uma assinatura válida em uma empresa não valida a mesma cópia em outra.
	abertoOrg1, recebidoOrg1, err := a.Ler(ctx, org, item)
	clear(abertoOrg1)
	if err != nil {
		t.Fatal(err)
	}
	if verificarAssinatura(org2, item, a.Identidade.ID, recebidoOrg1.Workspace, recebidoOrg1.Versao, recebidoOrg1.Copia) {
		t.Fatal("assinatura reaproveitada em outra empresa")
	}
	if err = terceiro.Request(ctx, "DELETE", base2+"/membros/"+a.Identidade.ID, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err = a.Ler(ctx, org2, item); err == nil {
		t.Fatal("remoção da segunda empresa manteve acesso")
	}
	b1, _, err := a.Ler(ctx, org, item)
	clear(b1)
	if err != nil {
		t.Fatal("remoção da segunda empresa afetou a primeira", err)
	}
	t.Log(fmt.Sprintf("3 clientes reais; organização %s; compartilhamento, edição, recuperação e remoção validados", org))
}
