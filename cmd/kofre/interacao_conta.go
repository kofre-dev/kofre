package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"kofre/pkg/compra"
	"kofre/pkg/config"
	"kofre/pkg/corporativo"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/runner"
	"kofre/pkg/tui"
	"kofre/pkg/vault"
)

// Usado exclusivamente pelo fluxo sequencial do painel no processo de conta.
// Comandos explícitos da CLI continuam com sua saída textual habitual.
type interacaoConta interface {
	Escolher(string, []string) (int, error)
	EscolherComDescricao(string, []string, []string) (int, error)
	Entrada(string, string, bool) ([]byte, error)
	Mensagem(string, string) error
	Esperar(string, string, func(context.Context) error) error
	Revelar(string, func(func([]byte) error) error) error
}

var interfaceConta interacaoConta
var mensagensConta strings.Builder

func informarConta(a ...any) {
	if interfaceConta == nil {
		fmt.Println(a...)
		return
	}
	mensagensConta.WriteString(fmt.Sprintln(a...))
}
func informarContaFormato(formato string, a ...any) {
	if interfaceConta == nil {
		fmt.Printf(formato, a...)
		return
	}
	fmt.Fprintf(&mensagensConta, formato, a...)
}
func exibirMensagensConta(titulo string) error {
	if interfaceConta == nil || mensagensConta.Len() == 0 {
		return nil
	}
	texto := mensagensConta.String()
	mensagensConta.Reset()
	err := interfaceConta.Mensagem(titulo, strings.TrimSpace(texto))
	if errors.Is(err, tui.ErrCancelado) {
		return nil
	}
	return err
}
func escolherOpcaoConta(titulo string, opcoes []string) (int, error) {
	if interfaceConta == nil {
		return tui.EscolherOpcao(titulo, opcoes)
	}
	if err := exibirMensagensConta("Conta e empresas"); err != nil {
		return -1, err
	}
	return interfaceConta.Escolher(titulo, opcoes)
}

func escolherOpcaoContaExplicada(titulo string, opcoes, descricoes []string) (int, error) {
	if interfaceConta == nil {
		return tui.EscolherOpcao(titulo, opcoes)
	}
	if err := exibirMensagensConta("Conta e empresas"); err != nil {
		return -1, err
	}
	return interfaceConta.EscolherComDescricao(titulo, opcoes, descricoes)
}
func solicitarEmpresa(label string) (string, error) {
	if interfaceConta == nil {
		return perguntarEmpresa(label), nil
	}
	texto := mensagensConta.String()
	mensagensConta.Reset()
	b, err := interfaceConta.Entrada(label, strings.TrimSpace(texto), false)
	defer mycrypto.ZeroBytes(b)
	return strings.TrimSpace(string(b)), err
}
func confirmarEmpresa(label, esperado string) bool {
	valor, err := solicitarEmpresa(label)
	return err == nil && valor == esperado
}

func confirmarSincronizacaoConta() (bool, error) {
	if interfaceConta == nil {
		return confirmarEmpresa("Conectar e sincronizar este cofre agora? [s/N]", "s"), nil
	}
	opcao, err := escolherOpcaoConta("Sincronizar este cofre com a nuvem gratuita?", []string{
		"Sim, sincronizar agora",
		"Não, manter offline",
	})
	return err == nil && opcao == 0, err
}
func concluirAcaoConta(err error) error {
	if errors.Is(err, tui.ErrCancelado) || errors.Is(err, context.Canceled) {
		mensagensConta.Reset()
		return nil
	}
	if err != nil {
		informarConta("Não foi possível concluir:", err)
	}
	if mensagensConta.Len() == 0 {
		return nil
	}
	return exibirMensagensConta("Resultado da operação")
}
func abrirCofreConta(path string) (*vault.ManagedVault, []byte, []byte, error) {
	if interfaceConta == nil {
		return runner.UnlockVaultWithKey(path)
	}
	return runner.UnlockVaultComEntrada(path, func() ([]byte, error) { return segredoEmpresa("Senha mestra do seu Kofre") })
}
func abrirCofreSemChaveConta(path string) (*vault.ManagedVault, error) {
	v, key, _, err := abrirCofreConta(path)
	mycrypto.ZeroBytes(key)
	return v, err
}

func aguardarCompraConta(sessao *compra.Sessao) (compra.Resultado, error) {
	var resultado compra.Resultado
	if interfaceConta == nil {
		return sessao.Aguardar(context.Background())
	}
	mensagensConta.Reset()
	texto := "Continue no navegador. Após o pagamento, o Kofre recebe a confirmação automaticamente.\n\n" + sessao.URL
	err := interfaceConta.Esperar("Aguardando pagamento", texto, func(ctx context.Context) error {
		var e error
		resultado, e = sessao.Aguardar(ctx)
		return e
	})
	return resultado, err
}

func executarProConta() error {
	if interfaceConta == nil {
		handlePro()
		return nil
	}
	i, err := escolherOpcaoConta("Pro pessoal", []string{"Assinar Pro no navegador", "Voltar"})
	if err != nil || i < 0 || i == 1 {
		return err
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}
	naConta := cfg.CloudEnabled && cfg.ContaID != "" && corporativo.CredencialContaValida(cfg.KofreToken)
	if cfg.CloudEnabled && !naConta {
		return errors.New("já existe outra nuvem configurada; consulte os planos em https://kofre.dev/comprar")
	}
	hostname, _ := os.Hostname()
	ctx := context.Background()
	var sessao *compra.Sessao
	if naConta {
		sessao, err = compra.IniciarComConta(ctx, config.GetCloudEndpoint(), hostname, cfg.KofreToken, cfg.ContaID, "pro")
	} else {
		sessao, err = compra.Iniciar(ctx, config.GetCloudEndpoint(), hostname)
	}
	if err != nil {
		return err
	}
	defer sessao.Close()
	_ = compra.AbrirNavegador(sessao.URL)
	r, err := aguardarCompraConta(sessao)
	if err != nil {
		return err
	}
	if r.Sandbox {
		informarConta("Pagamento de teste confirmado. Sua conta e o cofre foram preservados.")
		return sessao.Confirmar(ctx)
	}
	if r.Status != "active" || r.OrganizacaoID != "" {
		return errors.New("resposta de ativação do Pro inválida")
	}
	if r.ContaID != "" {
		err = config.ConfirmarProNaConta(r.ContaID)
	} else {
		err = config.AtivarLicencaComprada(sessao.Endpoint(), r.Token)
	}
	if err != nil {
		return fmt.Errorf("pagamento confirmado; ativação pendente. Consulte seu pedido: %w", err)
	}
	informarConta("Pro ativado. Seu cofre e sua senha mestra foram preservados.")
	return sessao.Confirmar(ctx)
}
