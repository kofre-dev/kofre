package corporativo

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestCadastroRetomaComIdentidadeSalvaAntesDaRede(t *testing.T) {
	i, err := PrepararCadastro("Pessoa fictícia")
	if err != nil {
		t.Fatal(err)
	}
	defer i.Fechar()
	path := filepath.Join(t.TempDir(), "identidade.enc")
	senha := []byte("senha fictícia comprida")
	if err = SalvarIdentidade(path, i, senha); err != nil {
		t.Fatal(err)
	}
	tentativas := 0
	var primeira map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tentativas++
		if r.Method != "POST" || r.URL.Path != "/v1/corporativo/identidades" || r.Header.Get("Authorization") != "Bearer "+i.Token {
			t.Error("cadastro incorreto")
		}
		var entrada map[string]string
		if err := json.NewDecoder(r.Body).Decode(&entrada); err != nil {
			t.Error(err)
		}
		if entrada["recuperacao"] != "" || entrada["privada"] != "" {
			t.Error("segredo enviado no corpo")
		}
		if tentativas == 1 {
			primeira = entrada
			http.Error(w, "resposta indisponível", 503)
			return
		}
		for k, v := range primeira {
			if entrada[k] != v {
				t.Errorf("retry alterou %s", k)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": strings.Repeat("a", 32), "nome": i.Nome})
	}))
	defer srv.Close()
	c, err := NovoClient(srv.URL, i)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Cadastrar(context.Background()); err == nil {
		t.Fatal("falha de rede ignorada")
	}
	reaberta, err := AbrirIdentidade(path, senha)
	if err != nil {
		t.Fatal(err)
	}
	defer reaberta.Fechar()
	c, err = NovoClient(srv.URL, reaberta)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Cadastrar(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reaberta.Privada, i.Privada) || reaberta.Token != i.Token || reaberta.Recuperacao != i.Recuperacao || reaberta.ID == "" {
		t.Fatal("retomada perdeu identidade")
	}
}
