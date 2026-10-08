package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/term"
	"kofre/pkg/compra"
	"kofre/pkg/config"
	"kofre/pkg/conta"
	"kofre/pkg/corporativo"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/storage"
)

func nonceEmpresa() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
func textoEmpresa(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}
func perguntarEmpresa(label string) string {
	fmt.Print(label + ": ")
	input := bufio.NewReader(os.Stdin)
	line, _ := input.ReadString('\n')
	return strings.TrimSpace(line)
}
func segredoEmpresa(label string) ([]byte, error) {
	if interfaceConta != nil {
		texto := mensagensConta.String()
		mensagensConta.Reset()
		return interfaceConta.Entrada(label, strings.TrimSpace(texto), true)
	}
	fmt.Print(label + ": ")
	data, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	return data, err
}

func handleEmpresa(args []string) {
	if err := executarEmpresa(args); err != nil {
		fmt.Fprintln(os.Stderr, "Empresa:", err)
		os.Exit(1)
	}
}

var errContaParaContratar = errors.New("para contratar o Corporativo, crie uma conta gratuita ou entre por e-mail neste PC; não é necessário assinar o Pro pessoal")
var errContaParaEmpresas = errors.New("para acessar empresas e convites, crie uma conta gratuita ou entre por e-mail neste PC")

func executarEmpresa(args []string) error {
	if len(args) > 0 && (args[0] == "entrar" || args[0] == "recuperar-email" || args[0] == "recuperar") {
		return entrarContaEmail(args[0] != "entrar")
	}
	if len(args) == 0 || args[0] == "ajuda" {
		informarConta(`Kofre Empresa — seu cofre pessoal permanece independente.
  criar-conta                         Cria sua identidade sem licença pessoal Pro
  comprar                             Contrata assentos para sua empresa no navegador
  renovar <organização>               Renova a mesma empresa, preservando os espaços
  assentos <organização> <quantidade>  Consulta preço e confirma ajuste de assentos
  assinatura <organização>            Consulta contrato e ajustes de assentos
  cancelar-assinatura <organização>   Interrompe a renovação e mantém o período pago
  conectar                            Ativa a nuvem pessoal gratuita nesta conta
  configurar                         Configura identidade e arquivo de recuperação
  entrar                             Conecta por e-mail e senha mestra
  recuperar                          Recupera acesso por e-mail e identidade cifrada
  sessoes                            Lista e encerra sessões dos computadores
  sair-conta                         Encerra a sessão deste computador
  backup-conta                       Publica o backup cifrado da identidade
  listar                             Lista suas organizações
  aceitar <organização>              Aceita convite (código solicitado sem eco)
  abrir <organização>                Lista pessoas, equipes, workspaces e itens
  workspace <organização> <nome>      Cria workspace
  equipe <organização> <nome> <ids>    Cria equipe (IDs separados por vírgula)
  convidar <organização> <pessoa>      Gera convite individual
  enviar <org> <workspace> <entrada>  Compartilha uma entrada do cofre pessoal
  editar <org> <item> <entrada>       Atualiza conteúdo usando uma entrada pessoal
  compartilhar <org> <item> <ids>     Acrescenta pessoas com leitura
  compartilhar-equipe <org> <item> <equipe> Compartilha com a equipe
  retirar <org> <item> <pessoa>        Remove compartilhamento individual
  mostrar <organização> <item>        Abre um item cifrado explicitamente
  remover <organização> <pessoa>      Retira membro e seus acessos
  transferir <org> <pessoa>           Transfere gestão para membro ativo
  papel <org> <pessoa> <papel>        Delega administrador ou rebaixa a membro
  cancelar-convite <org> <id>         Cancela convite pendente
  excluir <org> <item>                Exclui item corporativo
  exportar <arquivo.enc>              Gera cópia protegida da identidade
  restaurar <arquivo.enc>             Restaura identidade em um novo computador
  publicar-chave                     Retoma configuração interrompida
  fingerprint                        Mostra sua identificação criptográfica
  atualizar-equipe <org> <id> <nome> <ids> Atualiza membros da equipe
  permissao <org> <item> <pessoa> <papel> Define leitor, editor ou gestor
  recompartilhar <org> <item>         Entrega cópias aos novos membros da equipe
A conta fica protegida pela senha mestra do cofre. Exporte um backup para usar em outro computador.`)
		return nil
	}
	dir, err := config.GetDefaultDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "identidade-empresa.enc")
	sessao, err := abrirSessaoConta(path)
	if err != nil {
		return err
	}
	contaEmUso = sessao
	defer func() { sessao.fechar(); contaEmUso = nil }()
	var password []byte // Compatibilidade dos helpers; a persistência usa a chave selada do cofre.
	if err = migrarContaAnterior(path); err != nil {
		return err
	}
	if args[0] == "restaurar" {
		if len(args) != 2 {
			return errors.New("informe o backup cifrado")
		}
		if sessao.cofre.TemConta() {
			return errors.New("restauração exige destino sem identidade existente")
		}
		senhaBackup, e := segredoEmpresa("Senha mestra usada ao exportar o backup")
		if e != nil {
			return e
		}
		defer mycrypto.ZeroBytes(senhaBackup)
		backup, e := corporativo.AbrirIdentidade(args[1], senhaBackup)
		if e != nil {
			return e
		}
		defer backup.Fechar()
		return salvarIdentidadeConta(path, backup, password)
	}
	var identidade *corporativo.Identidade
	if args[0] == "criar-conta" || (args[0] == "ativar-nuvem" && !sessao.cofre.TemConta()) {
		return cadastrarContaEmail(path)
	}
	if args[0] == "configurar" {
		if sessao.cofre.TemConta() {
			return errors.New("identidade já existe; use recuperar ou restaure seu backup")
		}
		identidade, err = corporativo.NovaIdentidade()
		if err != nil {
			return err
		}
		token, err := segredoEmpresa("Token da conta corporativa")
		if err != nil {
			return err
		}
		defer mycrypto.ZeroBytes(token)
		recovery, err := segredoEmpresa("Chave de recuperação entregue pelo administrador")
		if err != nil {
			return err
		}
		defer mycrypto.ZeroBytes(recovery)
		identidade.Token = string(token)
		identidade.Recuperacao = string(recovery)
	} else {
		identidade, err = abrirIdentidadeConta(path, password)
		if err != nil {
			if args[0] == "comprar" && errors.Is(err, os.ErrNotExist) {
				return errContaParaContratar
			}
			if args[0] == "painel" && errors.Is(err, os.ErrNotExist) {
				return errContaParaEmpresas
			}
			return err
		}
	}
	defer identidade.Fechar()
	if args[0] == "ativar-nuvem" {
		if identidade.ID == "" {
			return cadastrarContaEmail(path)
		}
		if identidade.Token == "" {
			return errors.New("sua conta já existe; use Entrar por e-mail neste PC para renovar o acesso antes de sincronizar")
		}
		sincronizar, e := confirmarSincronizacaoConta()
		if e != nil || !sincronizar {
			return e
		}
		args = []string{"conectar"}
	}
	if args[0] == "comprar" && (identidade.ID == "" || identidade.Token == "") {
		return errContaParaContratar
	}
	if args[0] == "painel" && (identidade.ID == "" || identidade.Token == "") {
		return errContaParaEmpresas
	}
	if err = identidade.ProtegerPrivada(); err != nil {
		return err
	}
	client, err := corporativo.NovoClient(config.GetCloudEndpoint(), identidade)
	if err != nil {
		return err
	}
	return executarAcaoEmpresa(args, path, password, identidade, client)
}

func executarAcaoEmpresa(args []string, path string, password []byte, identidade *corporativo.Identidade, client *corporativo.Client) error {
	var err error
	ctx := context.Background()
	confirmarChave := func(id string, pessoa corporativo.Pessoa, fingerprint string) bool {
		informarContaFormato("Identidade: %s (%s)\nFingerprint: %s\n", textoEmpresa(pessoa.Nome), id, fingerprint)
		return confirmarEmpresa("Confirmou essa chave com a pessoa por outro canal? [s/N]", "s")
	}
	client.ConfirmarAutor = confirmarChave
	if args[0] == "sessoes" {
		return administrarSessoesConta(identidade)
	}
	if args[0] == "backup-conta" {
		if err := publicarBackupConta(contaEmUso, identidade); err != nil {
			return err
		}
		informarConta("Backup protegido da conta atualizado.")
		return nil
	}
	if args[0] == "sair-conta" {
		c, e := conta.NovoClient(config.GetCloudEndpoint())
		if e != nil {
			return e
		}
		if e = c.Logout(ctx, identidade.Token); e != nil {
			return e
		}
		if err := encerrarContaLocal(identidade); err != nil {
			return err
		}
		informarConta("Acesso deste computador encerrado. Seu cofre local continua protegido.")
		return nil
	}
	if args[0] == "painel" {
		return painelEmpresa(path, password, identidade, client)
	}
	if args[0] == "convites" {
		return convitesRecebidosConta(client)
	}
	if args[0] == "comprar" || args[0] == "renovar" {
		hostname, _ := os.Hostname()
		var sessao *compra.Sessao
		if args[0] == "renovar" {
			if len(args) != 2 {
				return errors.New("informe a organização")
			}
			sessao, err = compra.RenovarEmpresa(ctx, config.GetCloudEndpoint(), hostname, identidade.Token, identidade.ID, args[1])
		} else {
			sessao, err = compra.IniciarComConta(ctx, config.GetCloudEndpoint(), hostname, identidade.Token, identidade.ID, "corporativo")
		}
		if err != nil {
			return err
		}
		defer sessao.Close()
		informarConta("Compra vinculada à sua conta. Mantenha o Kofre aberto.")
		if err = compra.AbrirNavegador(sessao.URL); err != nil {
			informarConta("Abra este endereço para continuar:", sessao.URL)
		}
		resultado, err := aguardarCompraConta(sessao)
		if err != nil {
			return err
		}
		if resultado.Sandbox {
			informarConta("Pagamento de homologação confirmado; não concede uma assinatura de produção.")
		} else {
			informarConta("Assinatura confirmada. Sua organização:", resultado.OrganizacaoID)
		}
		return sessao.Confirmar(ctx)
	}
	if args[0] == "conectar" {
		if err := config.AtivarLicencaComprada(config.GetCloudEndpoint(), identidade.Token); err != nil {
			return err
		}
		local, err := storage.NewLocalStorage(resolveVaultPath(""))
		if err != nil {
			return err
		}
		cloud := storage.NewKofreCloudStorage(config.GetCloudEndpoint(), identidade.Token, identidade.ID)
		sync := storage.NewSyncStorage(local, cloud)
		defer sync.Close()
		if data, err := local.Load(ctx); err == nil {
			if err := sync.Save(ctx, data); err != nil {
				return err
			}
			if err := sync.Flush(20 * time.Second); err != nil {
				return fmt.Errorf("conta conectada e cópia local preservada; sincronização pendente: %w", err)
			}
		} else if !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		informarConta("Conta conectada. Seu cofre local foi preservado; a sincronização usará esta conta.")
		return nil
	}
	if args[0] == "fingerprint" {
		pub, _ := identidade.Publica()
		sign, _ := identidade.PublicaAssinatura()
		informarConta("Conta:", identidade.ID)
		informarConta("Fingerprint:", corporativo.FingerprintIdentidade(pub, sign))
		return nil
	}
	if args[0] == "publicar-chave" {
		publica, e := identidade.Publica()
		if e != nil {
			return e
		}
		assinatura, e := identidade.PublicaAssinatura()
		if e != nil {
			return e
		}
		return client.Request(ctx, "PUT", "/v1/corporativo/identidade/chave", map[string]string{"chave_publica": publica, "chave_assinatura": assinatura}, nil)
	}
	if args[0] == "configurar" {
		var conta struct {
			ID      string `json:"id"`
			Nome    string `json:"nome"`
			Publica string `json:"chave_publica"`
		}
		if err = client.Request(ctx, "GET", "/v1/corporativo/identidade", nil, &conta); err != nil {
			return err
		}
		identidade.ID = conta.ID
		identidade.Nome = conta.Nome
		publica, _ := identidade.Publica()
		if conta.Publica != "" && conta.Publica != publica {
			return errors.New("conta já tem chave; restaure o arquivo de identidade original")
		}
		// Salva a chave privada antes de publicar sua contraparte; falha de rede
		// deixa um arquivo recuperável, sem destruir o cofre pessoal.
		if err = salvarIdentidadeConta(path, identidade, password); err != nil {
			return err
		}
		assinatura, _ := identidade.PublicaAssinatura()
		if err = client.Request(ctx, "PUT", "/v1/corporativo/identidade/chave", map[string]string{"chave_publica": publica, "chave_assinatura": assinatura}, nil); err != nil {
			return err
		}
		informarConta("Identidade configurada. Faça uma cópia protegida com kofre empresa exportar.")
		return nil
	}
	if args[0] == "exportar" {
		if len(args) != 2 {
			return errors.New("informe destino do backup cifrado")
		}
		if _, err := os.Stat(args[1]); err == nil {
			return errors.New("destino já existe; escolha outro arquivo")
		}
		if err := exportarIdentidadeConta(args[1], identidade, password); err != nil {
			return err
		}
		informarConta("Backup protegido exportado para:", args[1])
		return nil
	}
	if args[0] == "listar" {
		var resposta map[string]any
		err = client.Request(ctx, "GET", "/v1/corporativo/identidade", nil, &resposta)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(resposta, "", "  ")
		informarConta(string(b))
		return nil
	}
	if len(args) < 2 {
		return errors.New("informe o ID da organização; consulte empresa listar")
	}
	org := args[1]
	base := "/v1/corporativo/organizacoes/" + org
	switch args[0] {
	case "cancelar-assinatura":
		var contrato struct {
			PedidoID   string `json:"pedido_id"`
			Recorrente bool   `json:"recorrente"`
			Cancelado  bool   `json:"cancelado"`
		}
		if err := client.Request(ctx, "GET", base+"/assinatura", nil, &contrato); err != nil {
			return err
		}
		if contrato.Cancelado || !contrato.Recorrente {
			informarConta("Este contrato não possui renovação automática ativa.")
			return nil
		}
		if !confirmarEmpresa("O período pago será mantido. Digite CANCELAR para interromper a renovação", "CANCELAR") {
			return nil
		}
		if err := client.Request(ctx, "POST", "/v1/billing/orders/"+contrato.PedidoID+"/cancel", nil, nil); err != nil {
			return err
		}
		informarConta("Renovação cancelada. O acesso continua até o fim do período pago.")
		return nil
	case "assinatura":
		var contrato map[string]any
		if err := client.Request(ctx, "GET", base+"/assinatura", nil, &contrato); err != nil {
			return err
		}
		if interfaceConta != nil {
			informarConta(resumoAssinatura(contrato))
		} else {
			b, _ := json.MarshalIndent(contrato, "", "  ")
			informarConta(string(b))
		}
		return nil
	case "assentos":
		if len(args) != 3 {
			return errors.New("informe a quantidade desejada")
		}
		n, e := strconv.Atoi(args[2])
		if e != nil || n < 1 {
			return errors.New("quantidade inválida")
		}
		var resposta struct {
			Cotacao struct {
				ID        string `json:"id"`
				Acrescimo int    `json:"acrescimo"`
				Centavos  int64  `json:"centavos"`
				Fim       string `json:"fim"`
			} `json:"cotacao"`
			Proximo int64 `json:"proximo_ciclo_centavos"`
		}
		if err := client.Request(ctx, "GET", base+"/assinatura/assentos?quantidade="+strconv.Itoa(n), nil, &resposta); err != nil {
			return err
		}
		informarContaFormato("Quantidade desejada: %d. Próximo ciclo: R$ %.2f.\n", n, float64(resposta.Proximo)/100)
		if resposta.Cotacao.Acrescimo > 0 {
			informarContaFormato("Acréscimo proporcional: R$ %.2f, válido até %s. Liberação após pagamento.\n", float64(resposta.Cotacao.Centavos)/100, resposta.Cotacao.Fim)
		} else {
			informarConta("A quantidade paga continua disponível até a renovação.")
		}
		if !confirmarEmpresa("Digite CONFIRMAR para prosseguir", "CONFIRMAR") {
			return nil
		}
		var ajuste struct {
			Ajuste struct {
				CheckoutURL string `json:"checkout_url"`
				Status      string `json:"status"`
			} `json:"ajuste"`
		}
		if err := client.Request(ctx, "POST", base+"/assinatura/assentos", map[string]string{"cotacao_id": resposta.Cotacao.ID}, &ajuste); err != nil {
			return err
		}
		if ajuste.Ajuste.CheckoutURL != "" {
			if err := compra.AbrirNavegador(ajuste.Ajuste.CheckoutURL); err != nil {
				informarConta("Continue o pagamento:", ajuste.Ajuste.CheckoutURL)
			}
		}
		informarConta("Estado do ajuste:", textoEmpresa(ajuste.Ajuste.Status))
		return nil
	case "papel":
		if len(args) != 4 {
			return errors.New("informe pessoa e papel administrador ou membro")
		}
		err = client.Request(ctx, "PUT", base+"/membros/"+args[2]+"/papel", map[string]string{"papel": args[3]}, nil)
	case "cancelar-convite":
		if len(args) != 3 {
			return errors.New("informe ID do convite")
		}
		err = client.Request(ctx, "DELETE", base+"/convites/"+args[2], nil, nil)
	case "aceitar":
		codigo, e := segredoEmpresa("Código do convite")
		if e != nil {
			return e
		}
		defer mycrypto.ZeroBytes(codigo)
		err = client.Request(ctx, "POST", base+"/aceitar", map[string]string{"codigo": string(codigo)}, nil)
	case "abrir":
		recursos, e := client.Recursos(ctx, org)
		if e != nil {
			return e
		}
		informarConta("Pessoas:")
		for id, pessoa := range recursos.Pessoas {
			informarContaFormato("  %s  %s\n", id, textoEmpresa(pessoa.Nome))
		}
		informarConta("Workspaces:")
		for id, workspace := range recursos.Workspaces {
			informarContaFormato("  %s  %s\n", id, textoEmpresa(workspace.Nome))
		}
		informarConta("Equipes:")
		for id, equipe := range recursos.Equipes {
			informarContaFormato("  %s  %s (%d pessoas)\n", id, textoEmpresa(equipe.Nome), len(equipe.Membros))
		}
		informarConta("Itens disponíveis:")
		for id, resumo := range recursos.Segredos {
			informarContaFormato("  %s  workspace %s  %s\n", id, resumo.Workspace, resumo.Papel)
		}
		return nil
	case "workspace":
		if len(args) != 3 {
			return errors.New("informe o nome do workspace")
		}
		id := nonceEmpresa()
		err = client.Request(ctx, "PUT", base+"/workspaces/"+id, corporativo.Workspace{Nome: args[2]}, nil)
		if err == nil {
			informarConta("Workspace:", id)
		}
	case "equipe", "atualizar-equipe":
		if args[0] == "equipe" && len(args) != 4 || args[0] == "atualizar-equipe" && len(args) != 5 {
			return errors.New("informe nome e IDs dos membros separados por vírgula")
		}
		id := nonceEmpresa()
		nome, membros := args[2], args[3]
		if args[0] == "atualizar-equipe" {
			id, nome, membros = args[2], args[3], args[4]
		}
		err = client.Request(ctx, "PUT", base+"/equipes/"+id, corporativo.Equipe{Nome: nome, Membros: strings.Split(membros, ",")}, nil)
		if err == nil {
			informarConta("Equipe:", id)
		}
	case "convidar":
		if len(args) != 3 {
			return errors.New("informe o destinatário")
		}
		var convite map[string]string
		campo := "destinatario"
		if strings.Contains(args[2], "@") {
			campo = "email"
		}
		err = client.Request(ctx, "POST", base+"/convites", map[string]string{campo: args[2]}, &convite)
		if err == nil {
			if campo == "email" {
				informarConta("Convite criado. A pessoa será avisada por e-mail e poderá aceitar em Minha conta → Convites recebidos.")
			} else {
				informarConta("Código do convite (envie por canal seguro):", convite["codigo"])
			}
		}
	case "transferir":
		if len(args) != 3 || !confirmarEmpresa("Digite TRANSFERIR para confirmar", "TRANSFERIR") {
			return errors.New("transferência não confirmada")
		}
		err = client.Request(ctx, "PUT", base+"/proprietario", map[string]string{"novo_proprietario": args[2], "confirmacao": "TRANSFERIR"}, nil)
	case "remover":
		if len(args) != 3 || !confirmarEmpresa("Digite REMOVER para confirmar", "REMOVER") {
			return errors.New("remoção não confirmada")
		}
		err = client.Request(ctx, "DELETE", base+"/membros/"+args[2], nil, nil)
	case "excluir":
		if len(args) != 3 || !confirmarEmpresa("Digite EXCLUIR para confirmar", "EXCLUIR") {
			return errors.New("exclusão não confirmada")
		}
		err = client.Request(ctx, "DELETE", base+"/segredos/"+args[2], nil, nil)
	case "mostrar":
		if len(args) != 3 {
			return errors.New("informe o item")
		}
		if !confirmarEmpresa("Exibir conteúdo secreto no terminal? Digite MOSTRAR", "MOSTRAR") {
			return errors.New("exibição cancelada")
		}
		data, _, e := client.Ler(ctx, org, args[2])
		if e != nil {
			return e
		}
		defer mycrypto.ZeroBytes(data)
		if e = salvarIdentidadeConta(path, identidade, password); e != nil {
			return e
		}
		return escreverSegredoTerminal(data)
	case "enviar", "editar", "compartilhar", "compartilhar-equipe", "retirar", "permissao", "recompartilhar":
		var data []byte
		var item string
		var workspace string
		versao := uint64(1)
		acesso := corporativo.Acesso{Pessoas: map[string]string{identidade.ID: "gestor"}, Equipes: map[string]string{}}
		if args[0] == "enviar" {
			if len(args) != 4 {
				return errors.New("informe organização, workspace e ID da entrada pessoal")
			}
			v, e := abrirCofreSemChaveConta(resolveVaultPath(""))
			if e != nil {
				return e
			}
			defer v.Close()
			data, e = v.ExportarEntrada(args[3])
			if e != nil {
				return e
			}
			workspace = args[2]
			item = nonceEmpresa()
		} else {
			if args[0] == "recompartilhar" && len(args) != 3 || args[0] == "permissao" && len(args) != 5 || args[0] != "recompartilhar" && args[0] != "permissao" && len(args) != 4 {
				return errors.New("informe item e pessoa ou equipe")
			}
			var recebida *corporativo.ItemRecebido
			data, recebida, err = client.Ler(ctx, org, args[2])
			if err != nil {
				return err
			}
			item = args[2]
			workspace = recebida.Workspace
			versao = recebida.Versao + 1
			acesso = recebida.Acesso
			if args[0] == "editar" {
				mycrypto.ZeroBytes(data)
				v, e := abrirCofreSemChaveConta(resolveVaultPath(""))
				if e != nil {
					return e
				}
				defer v.Close()
				data, e = v.ExportarEntrada(args[3])
				if e != nil {
					return e
				}
			}
			if acesso.Pessoas == nil {
				acesso.Pessoas = map[string]string{}
			}
			if acesso.Equipes == nil {
				acesso.Equipes = map[string]string{}
			}
			if args[0] == "compartilhar" {
				for _, id := range strings.Split(args[3], ",") {
					if acesso.Pessoas[id] == "" {
						acesso.Pessoas[id] = "leitor"
					}
				}
			} else if args[0] == "compartilhar-equipe" {
				acesso.Equipes[args[3]] = "leitor"
			} else if args[0] == "permissao" {
				if args[4] != "leitor" && args[4] != "editor" && args[4] != "gestor" {
					return errors.New("papel inválido")
				}
				acesso.Pessoas[args[3]] = args[4]
			} else if args[0] == "retirar" {
				delete(acesso.Pessoas, args[3])
			}
		}
		defer mycrypto.ZeroBytes(data)
		protegido, e := mycrypto.SealMemory(data)
		mycrypto.ZeroBytes(data)
		if e != nil {
			return e
		}
		defer protegido.Close()
		err = client.GravarProtegido(ctx, org, item, workspace, versao, acesso, protegido, confirmarChave)
		if err == nil {
			err = salvarIdentidadeConta(path, identidade, password)
			if err == nil {
				informarConta("Item corporativo:", item)
			}
		}
	default:
		return errors.New("comando desconhecido; use kofre empresa ajuda")
	}
	if err == nil {
		informarConta("Operação confirmada pelo Cloud.")
	}
	return err
}
