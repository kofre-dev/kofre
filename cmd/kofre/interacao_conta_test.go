package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"kofre/pkg/config"
	"kofre/pkg/corporativo"
	"kofre/pkg/tui"
)

func TestCorporativoCadastroOuEntradaRetomaCompra(t *testing.T) {
	for _, caso := range []struct {
		nome     string
		escolha  int
		esperado []string
	}{
		{"cadastro", 0, []string{"comprar", "criar-conta", "comprar"}},
		{"entrada", 1, []string{"comprar", "entrar", "comprar"}},
		{"voltar", 2, []string{"comprar"}},
		{"escape", -1, []string{"comprar"}},
	} {
		t.Run(caso.nome, func(t *testing.T) {
			usarInterfaceFicticia(t, &interfaceContaFicticia{escolhas: []int{caso.escolha}})
			var chamadas []string
			err := contratarCorporativoConta(func(args []string) error {
				chamadas = append(chamadas, args[0])
				if len(chamadas) == 1 {
					return errContaParaContratar
				}
				return nil
			})
			if err != nil || !reflect.DeepEqual(chamadas, caso.esperado) {
				t.Fatalf("fluxo incorreto: %v, erro %v", chamadas, err)
			}
		})
	}
}

func TestCorporativoNaoCompraDepoisDeCadastroCanceladoOuFalha(t *testing.T) {
	for _, falha := range []error{tui.ErrCancelado, errors.New("confirmação por e-mail falhou")} {
		usarInterfaceFicticia(t, &interfaceContaFicticia{escolhas: []int{0}})
		var chamadas []string
		err := contratarCorporativoConta(func(args []string) error {
			chamadas = append(chamadas, args[0])
			if len(chamadas) == 1 {
				return errContaParaContratar
			}
			return falha
		})
		if !errors.Is(err, falha) || !reflect.DeepEqual(chamadas, []string{"comprar", "criar-conta"}) {
			t.Fatalf("iniciou compra após falha: %v %v", chamadas, err)
		}
	}
	usarInterfaceFicticia(t, &interfaceContaFicticia{})
	falha := errors.New("identidade corrompida")
	if err := contratarCorporativoConta(func([]string) error { return falha }); !errors.Is(err, falha) {
		t.Fatal("erro real de identidade foi ocultado")
	}
}

type interfaceContaFicticia struct {
	escolhas   []int
	entrada    []byte
	err        error
	segredo    bool
	mensagem   string
	opcoes     []string
	descricoes []string
}

func (f *interfaceContaFicticia) Escolher(_ string, opcoes []string) (int, error) {
	f.opcoes = append([]string(nil), opcoes...)
	i := f.escolhas[0]
	f.escolhas = f.escolhas[1:]
	return i, nil
}
func (f *interfaceContaFicticia) EscolherComDescricao(titulo string, opcoes, descricoes []string) (int, error) {
	f.descricoes = append([]string(nil), descricoes...)
	return f.Escolher(titulo, opcoes)
}

func TestPainelContaInicialTemCincoOpcoesExplicadas(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	f := &interfaceContaFicticia{escolhas: []int{4}}
	usarInterfaceFicticia(t, f)
	if err := painelContaLoop(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.opcoes, []string{"Ativar nuvem gratuita", "Entrar na minha conta", "Empresas e equipes", "Conhecer planos", "Voltar ao cofre"}) {
		t.Fatalf("menu inesperado: %v", f.opcoes)
	}
	if len(f.descricoes) != len(f.opcoes) {
		t.Fatal("há opção sem explicação")
	}
	for _, texto := range f.descricoes {
		if texto == "" {
			t.Fatal("explicação vazia")
		}
	}
}

func TestPainelContaCriadaOfflineOfereceSincronizar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := config.DefaultConfig()
	cfg.ContaConfigurada = true
	cfg.CloudEnabled = false
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	f := &interfaceContaFicticia{escolhas: []int{4}}
	usarInterfaceFicticia(t, f)
	if err := painelContaLoop(); err != nil {
		t.Fatal(err)
	}
	if f.opcoes[0] != "Sincronizar cofre" || f.opcoes[1] != "Minha conta" {
		t.Fatalf("conta criada foi confundida com nova: %v", f.opcoes)
	}
	atual, err := config.LoadConfig()
	if err != nil || atual.CloudEnabled {
		t.Fatal("abrir menu ativou sincronização")
	}
}

func TestSincronizacaoContaExigeSelecaoExplicita(t *testing.T) {
	for _, caso := range []struct {
		nome        string
		opcao       int
		sincronizar bool
	}{
		{"sim", 0, true},
		{"nao", 1, false},
		{"escape", -1, false},
	} {
		t.Run(caso.nome, func(t *testing.T) {
			f := &interfaceContaFicticia{escolhas: []int{caso.opcao}}
			usarInterfaceFicticia(t, f)
			resultado, err := confirmarSincronizacaoConta()
			if err != nil || resultado != caso.sincronizar {
				t.Fatalf("seleção inesperada: %v, erro %v", resultado, err)
			}
			if !reflect.DeepEqual(f.opcoes, []string{"Sim, sincronizar agora", "Não, manter offline"}) {
				t.Fatalf("opções inesperadas: %v", f.opcoes)
			}
		})
	}
}

func TestAtivarNuvemContaExistenteNaoRepeteCadastro(t *testing.T) {
	for _, escolha := range []int{1, -1} {
		t.Run(fmt.Sprint(escolha), func(t *testing.T) {
			s := prepararSessaoContaTeste(t)
			i, err := corporativo.PrepararCadastro("Pessoa fictícia")
			if err != nil {
				t.Fatal(err)
			}
			defer i.Fechar()
			i.ID = strings.Repeat("a", 32)
			i.Email = "fixture@example.com"
			i.Token = "credencial-ficticia"
			if err = s.salvar(i); err != nil {
				t.Fatal(err)
			}
			antes, err := os.ReadFile(s.arquivo)
			if err != nil {
				t.Fatal(err)
			}
			usarInterfaceFicticia(t, &interfaceContaFicticia{escolhas: []int{escolha}, entrada: []byte("senha-mestra-teste")})
			if err = executarEmpresa([]string{"ativar-nuvem"}); err != nil {
				t.Fatal(err)
			}
			depois, err := os.ReadFile(s.arquivo)
			if err != nil || !bytes.Equal(antes, depois) {
				t.Fatal("cancelar sincronização alterou o cofre")
			}
			cfg, err := config.LoadConfig()
			if err != nil || cfg.CloudEnabled {
				t.Fatal("recusar sincronização ativou a nuvem")
			}
		})
	}
}
func (f *interfaceContaFicticia) Entrada(_, texto string, segredo bool) ([]byte, error) {
	f.segredo = segredo
	f.mensagem = texto
	return f.entrada, f.err
}
func (f *interfaceContaFicticia) Mensagem(_, texto string) error { f.mensagem = texto; return nil }
func (f *interfaceContaFicticia) Esperar(_, _ string, trabalho func(context.Context) error) error {
	return trabalho(context.Background())
}
func (f *interfaceContaFicticia) Revelar(string, func(func([]byte) error) error) error { return nil }
func usarInterfaceFicticia(t *testing.T, f *interfaceContaFicticia) {
	t.Helper()
	anterior := interfaceConta
	interfaceConta = f
	mensagensConta.Reset()
	t.Cleanup(func() { interfaceConta = anterior; mensagensConta.Reset() })
}

func TestCancelarCriacaoWorkspaceNaoProduzComandoNemGravacao(t *testing.T) {
	f := &interfaceContaFicticia{escolhas: []int{0, 1, 0}, err: tui.ErrCancelado}
	usarInterfaceFicticia(t, f)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v1/corporativo/identidade" {
			t.Errorf("chamada inesperada %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"organizacoes": []map[string]any{{"id": strings.Repeat("a", 32), "nome": "Empresa fictícia", "papel": "proprietario", "validade": time.Now().Add(time.Hour)}}})
	}))
	defer srv.Close()
	c := &corporativo.Client{Endpoint: srv.URL, HTTP: srv.Client()}
	args, err := selecionarAcaoOrganizacao(c)
	if !errors.Is(err, tui.ErrCancelado) || len(args) != 0 {
		t.Fatalf("cancelamento gerou comando: %v %v", args, err)
	}
}

func TestCorporativoVoltarDosGruposNaoExecutaAcao(t *testing.T) {
	usarInterfaceFicticia(t, &interfaceContaFicticia{escolhas: []int{1, 5, 3}})
	acao, err := escolherAcaoOrganizacao("Empresa")
	if err != nil || acao != -1 {
		t.Fatalf("voltar produziu ação: %d, %v", acao, err)
	}
}

func TestCorporativoConviteRecebidoVoltarNaoAceita(t *testing.T) {
	usarInterfaceFicticia(t, &interfaceContaFicticia{escolhas: []int{0, 2}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v1/corporativo/convites" {
			t.Errorf("navegar alterou convite: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"convites": []map[string]any{{"id": strings.Repeat("b", 64), "organizacao": strings.Repeat("a", 32), "empresa": "Empresa fictícia", "validade": time.Now().Add(time.Hour)}}})
	}))
	defer srv.Close()
	if err := convitesRecebidosConta(&corporativo.Client{Endpoint: srv.URL, HTTP: srv.Client()}); err != nil {
		t.Fatal(err)
	}
}

func TestCorporativoSemEmpresasNaoEErro(t *testing.T) {
	f := &interfaceContaFicticia{}
	usarInterfaceFicticia(t, f)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v1/corporativo/identidade" {
			t.Error("chamada inesperada")
		}
		w.Write([]byte(`{"organizacoes":[]}`))
	}))
	defer srv.Close()
	args, err := selecionarAcaoOrganizacao(&corporativo.Client{Endpoint: srv.URL, HTTP: srv.Client()})
	if err != nil || len(args) != 0 || !strings.Contains(mensagensConta.String(), "ainda não participa") {
		t.Fatalf("estado vazio incorreto: %v %v", args, err)
	}
}

func TestCancelarConviteOuRenovacaoNaoEnviaAlteracao(t *testing.T) {
	for _, acao := range []string{"aceitar", "cancelar-assinatura", "assentos"} {
		t.Run(acao, func(t *testing.T) {
			f := &interfaceContaFicticia{err: tui.ErrCancelado}
			usarInterfaceFicticia(t, f)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("cancelamento enviou %s", r.Method)
					w.WriteHeader(500)
					return
				}
				if acao == "assentos" {
					json.NewEncoder(w).Encode(map[string]any{"cotacao": map[string]any{"id": "cotacao", "acrescimo": 1, "centavos": 12345, "fim": "2026-12-31"}, "proximo_ciclo_centavos": 56789})
				} else {
					json.NewEncoder(w).Encode(map[string]any{"pedido_id": strings.Repeat("b", 32), "recorrente": true, "cancelado": false})
				}
			}))
			defer srv.Close()
			c := &corporativo.Client{Endpoint: srv.URL, HTTP: srv.Client()}
			args := []string{acao, strings.Repeat("a", 32)}
			if acao == "assentos" {
				args = append(args, "4")
			}
			err := executarAcaoEmpresa(args, "", nil, &corporativo.Identidade{}, c)
			if acao == "aceitar" {
				if !errors.Is(err, tui.ErrCancelado) || !f.segredo {
					t.Fatal("convite não cancelou entrada segura")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if acao == "assentos" && (!strings.Contains(f.mensagem, "123.45") || !strings.Contains(f.mensagem, "567.89")) {
				t.Fatal("cotação não usa valores do Cloud")
			}
		})
	}
}

func TestResumoAssinaturaEFinalizacaoPermanecemNoPainel(t *testing.T) {
	f := &interfaceContaFicticia{}
	usarInterfaceFicticia(t, f)
	texto := resumoAssinatura(map[string]any{"assentos": 8, "ocupados": 5, "validade": "2027-10-08T12:00:00Z", "recorrente": true, "cancelado": false, "pedido_id": "pedido-ficticio"})
	informarConta(texto)
	if err := concluirAcaoConta(nil); err != nil {
		t.Fatal(err)
	}
	for _, esperado := range []string{"contratados: 8", "ocupados: 5", "08/10/2027", "Automática"} {
		if !strings.Contains(f.mensagem, esperado) {
			t.Fatalf("resumo sem %q", esperado)
		}
	}
	if strings.Contains(f.mensagem, "{\"") || mensagensConta.Len() != 0 {
		t.Fatal("JSON bruto ou mensagens residuais")
	}
}
