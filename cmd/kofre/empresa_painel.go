package main

import (
	"context"
	"encoding/json"
	"errors"
	"kofre/pkg/config"
	"kofre/pkg/corporativo"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/tui"
	"sort"
	"strings"
	"time"
)

func painelConta() error {
	return tui.ExecutarPainel(func(p *tui.PainelInterativo) error {
		interfaceConta = p
		defer func() { interfaceConta = nil; mensagensConta.Reset() }()
		return painelContaLoop()
	})
}

func painelContaLoop() error {
	for {
		opcoes := []string{"Ativar nuvem gratuita", "Entrar na minha conta", "Empresas e equipes", "Conhecer planos", "Voltar ao cofre"}
		descricoes := []string{
			"Sincronize entre PCs gratuitamente. Cadastre seu e-mail e use a mesma senha mestra.",
			"Já tem conta? Entre neste PC. Aqui também ficam sessões, backups e recuperação de acesso.",
			"Acesse suas empresas ou aceite um convite. Seu cofre pessoal permanece privado e separado.",
			"Conheça o Pro pessoal e o Corporativo para equipes. Você pode contratar usando uma conta Free.",
			"Volte às suas credenciais pessoais. O uso offline continua gratuito e não exige cadastro.",
		}
		if cfg, e := config.LoadConfig(); e == nil {
			if cfg.ContaConfigurada || cfg.ContaID != "" {
				opcoes[0], opcoes[1] = "Sincronizar cofre", "Minha conta"
				descricoes[0] = "Sua conta já existe. Sincronize este cofre entre PCs; os dados são cifrados antes do envio."
				descricoes[1] = "Consulte sua identidade e gerencie sessões, backups ou recuperação de acesso."
			}
			if cfg.CloudEnabled {
				opcoes[0] = "Sincronização"
			}
		}
		i, err := escolherOpcaoContaExplicada("Kofre · Conta e empresas", opcoes, descricoes)
		if err != nil {
			return err
		}
		if i < 0 || i == 4 {
			return nil
		}
		switch i {
		case 0:
			err = executarEmpresa([]string{"ativar-nuvem"})
		case 1:
			err = menuContaPessoal()
		case 2:
			err = acessarEmpresasConta("painel", executarEmpresa)
		case 3:
			err = menuPlanosConta()
		}
		if e := concluirAcaoConta(err); e != nil {
			return e
		}
	}
}

func menuContaPessoal() error {
	op, err := escolherOpcaoContaExplicada("Minha conta", []string{"Entrar por e-mail neste PC", "Minha identidade e empresas", "Sessões e dispositivos", "Backup e recuperação", "Convites recebidos", "Encerrar acesso deste PC", "Voltar"}, []string{
		"Conecte uma conta existente usando e-mail e senha mestra. Não cria uma nova conta.",
		"Veja sua identificação para convites e acesse as empresas das quais participa.",
		"Consulte os computadores conectados e encerre sessões que não reconhecer.",
		"Recupere acesso, consulte o histórico ou gerencie backups protegidos da conta.",
		"Veja as empresas que convidaram seu e-mail e aceite ou recuse no aplicativo.",
		"Encerre a sessão desta conta neste computador. As credenciais locais não são apagadas.",
		"Volte ao menu de conta e empresas.",
	})
	if err != nil || op < 0 || op == 6 {
		return err
	}
	if op == 3 {
		return menuBackupConta()
	}
	if op == 1 {
		return acessarEmpresasConta("painel", executarEmpresa)
	}
	acoes := []string{"entrar", "painel", "sessoes", "", "convites", "sair-conta"}
	return executarEmpresa([]string{acoes[op]})
}

func menuBackupConta() error {
	op, err := escolherOpcaoContaExplicada("Backup e recuperação", []string{"Recuperar acesso por e-mail", "Histórico do cofre pessoal", "Atualizar backup da conta na nuvem", "Restaurar backup manual", "Voltar"}, []string{
		"Recupere o acesso à conta por e-mail. Para abrir os dados cifrados, você ainda precisa da senha mestra.",
		"Consulte as versões disponíveis do cofre pessoal. A disponibilidade depende do seu plano.",
		"Envie uma cópia cifrada da identidade da conta para facilitar a recuperação de acesso.",
		"Restaure sua identidade a partir de um arquivo de backup protegido, usando a senha da exportação.",
		"Volte sem alterar seu cofre ou sua conta.",
	})
	if err != nil || op < 0 || op == 4 {
		return err
	}
	switch op {
	case 0:
		return executarEmpresa([]string{"recuperar-email"})
	case 1:
		return executarHistorico([]string{"painel"})
	case 2:
		return executarEmpresa([]string{"backup-conta"})
	case 3:
		arquivo, e := solicitarEmpresa("Arquivo de identidade .enc")
		if e != nil {
			return e
		}
		return executarEmpresa([]string{"restaurar", arquivo})
	}
	return nil
}

func menuPlanosConta() error {
	op, err := escolherOpcaoContaExplicada("Planos e assinatura", []string{"Pro pessoal", "Corporativo para minha empresa", "Voltar"}, []string{
		"Recursos extras para seu cofre pessoal. Não é necessário para participar de uma empresa.",
		"Contrate assentos para compartilhar credenciais com sua equipe. Cada pessoa mantém seu cofre pessoal separado.",
		"Volte ao menu sem iniciar uma contratação.",
	})
	if err != nil || op < 0 || op == 2 {
		return err
	}
	if op == 0 {
		return executarProConta()
	}
	return contratarCorporativoConta(executarEmpresa)
}

func contratarCorporativoConta(executar func([]string) error) error {
	return acessarEmpresasConta("comprar", executar)
}

func acessarEmpresasConta(acaoDesejada string, executar func([]string) error) error {
	for {
		err := executar([]string{acaoDesejada})
		if !errors.Is(err, errContaParaContratar) && !errors.Is(err, errContaParaEmpresas) {
			return err
		}
		titulo := "Empresas · Primeiro conecte sua conta gratuita"
		if acaoDesejada == "comprar" {
			titulo = "Corporativo · Primeiro conecte sua conta gratuita"
		}
		op, err := escolherOpcaoContaExplicada(titulo, []string{"Criar conta gratuita por e-mail", "Entrar por e-mail neste PC", "Voltar"}, []string{
			"Crie sua identidade para participar de empresas. Não exige assinatura pessoal Pro nem sincronizar seu cofre.",
			"Use uma conta já cadastrada. Depois de entrar, você volta ao que estava fazendo.",
			"Volte sem criar conta ou iniciar uma contratação.",
		})
		if err != nil || op < 0 || op == 2 {
			return err
		}
		acao := "criar-conta"
		if op == 1 {
			acao = "entrar"
		}
		if err := executar([]string{acao}); err != nil {
			return err
		}
		if err := exibirMensagensConta("Conta · Continuar para empresas"); err != nil {
			return err
		}
	}
}

func escolherRecurso(titulo string, opcoes map[string]string) (string, error) {
	ids := make([]string, 0, len(opcoes))
	for id := range opcoes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return opcoes[ids[i]] < opcoes[ids[j]] })
	if len(ids) == 0 {
		return "", errors.New("nenhum item disponível nesta seleção")
	}
	nomes := make([]string, len(ids))
	for i, id := range ids {
		nomes[i] = opcoes[id] + "  · " + id[:min(8, len(id))]
	}
	i, err := escolherOpcaoConta(titulo, nomes)
	if err != nil {
		return "", err
	}
	if i < 0 {
		return "", nil
	}
	return ids[i], nil
}

func painelEmpresa(path string, senha []byte, identidade *corporativo.Identidade, client *corporativo.Client) error {
	for {
		op, err := escolherOpcaoContaExplicada("Empresas e equipes · "+identidade.Nome,
			[]string{"Minhas empresas", "Criar minha empresa", "Identificação e backup", "Voltar"},
			[]string{"Abra credenciais compartilhadas e gerencie equipe e assinatura conforme suas permissões.", "Contrate o Corporativo para sua empresa. Seu cofre pessoal continua separado.", "Consulte sua identificação ou exporte um backup protegido da identidade.", "Volte para Conta e nuvem."})
		if err != nil {
			return err
		}
		if op < 0 || op == 3 {
			return nil
		}
		var args []string
		switch op {
		case 0:
			args, err = selecionarAcaoOrganizacao(client)
			if err == nil {
				err = salvarIdentidadeConta(path, identidade, senha)
			}
		case 1:
			args = []string{"comprar"}
		case 2:
			detalhe, e := escolherOpcaoConta("Identificação e backup", []string{"Minha identificação para convites", "Exportar backup protegido", "Voltar"})
			err = e
			if err == nil && detalhe == 0 {
				args = []string{"fingerprint"}
			}
			if err == nil && detalhe == 1 {
				arquivo, e := solicitarEmpresa("Caminho do arquivo de backup .enc")
				err = e
				if err == nil {
					args = []string{"exportar", arquivo}
				}
			}
		}
		if err == nil && len(args) > 0 {
			err = executarAcaoEmpresa(args, path, senha, identidade, client)
		}
		if e := concluirAcaoConta(err); e != nil {
			return e
		}
	}
}

// Mantém os comandos existentes; navegar pelos grupos não executa alterações.
func escolherAcaoOrganizacao(titulo string) (int, error) {
	grupos := []struct {
		nome, descricao string
		opcoes          []string
		acoes           []int
	}{
		{"Credenciais", "Abra credenciais compartilhadas ou compartilhe um item com a empresa.",
			[]string{"Abrir credencial", "Compartilhar credencial pessoal", "Dar acesso a uma pessoa", "Dar acesso a uma equipe"}, []int{0, 1, 4, 6}},
		{"Equipe e workspaces", "Organize pessoas, equipes e espaços de trabalho. Ações dependem do seu papel.",
			[]string{"Criar workspace", "Convidar pessoa", "Criar equipe", "Remover membro", "Ver pessoas, equipes e workspaces"}, []int{2, 3, 5, 7, 11}},
		{"Assinatura", "Consulte o contrato e gerencie assentos e renovação conforme suas permissões.",
			[]string{"Ver assinatura e assentos", "Alterar assentos", "Renovar contrato", "Cancelar renovação automática"}, []int{8, 9, 10, 12}},
	}
	for {
		grupo, err := escolherOpcaoContaExplicada(titulo,
			[]string{grupos[0].nome, grupos[1].nome, grupos[2].nome, "Voltar"},
			[]string{grupos[0].descricao, grupos[1].descricao, grupos[2].descricao, "Volte para suas empresas."})
		if err != nil || grupo < 0 || grupo >= len(grupos) {
			return -1, err
		}
		g := grupos[grupo]
		opcoes := append(append([]string(nil), g.opcoes...), "Voltar")
		acao, err := escolherOpcaoConta(titulo+" · "+g.nome, opcoes)
		if err != nil {
			return -1, err
		}
		if acao >= 0 && acao < len(g.acoes) {
			return g.acoes[acao], nil
		}
	}
}

func selecionarAcaoOrganizacao(client *corporativo.Client) ([]string, error) {
	ctx := context.Background()
	var conta struct {
		Organizacoes []struct {
			ID       string    `json:"id"`
			Nome     string    `json:"nome"`
			Papel    string    `json:"papel"`
			Validade time.Time `json:"validade"`
		} `json:"organizacoes"`
	}
	if err := client.Request(ctx, "GET", "/v1/corporativo/identidade", nil, &conta); err != nil {
		return nil, err
	}
	if len(conta.Organizacoes) == 0 {
		informarConta("Você ainda não participa de nenhuma empresa. Use Criar minha empresa para contratar o Corporativo ou aguarde um convite enviado pela empresa ao seu e-mail.")
		return nil, nil
	}
	opcoes := map[string]string{}
	for _, org := range conta.Organizacoes {
		opcoes[org.ID] = org.Nome + " · " + org.Papel
		if !org.Validade.After(time.Now()) {
			opcoes[org.ID] += " · contrato vencido"
		}
	}
	org, err := escolherRecurso("Suas empresas", opcoes)
	if err != nil || org == "" {
		return nil, err
	}
	op, err := escolherAcaoOrganizacao(opcoes[org])
	if err != nil || op < 0 {
		return nil, err
	}
	switch op {
	case 2:
		valor, e := solicitarEmpresa("Nome do workspace")
		if e != nil {
			return nil, e
		}
		return []string{"workspace", org, valor}, nil
	case 3:
		valor, e := solicitarEmpresa("E-mail da pessoa a convidar")
		if e != nil {
			return nil, e
		}
		return []string{"convidar", org, valor}, nil
	case 8:
		return []string{"assinatura", org}, nil
	case 9:
		valor, e := solicitarEmpresa("Quantidade total desejada")
		if e != nil {
			return nil, e
		}
		return []string{"assentos", org, valor}, nil
	case 10:
		return []string{"renovar", org}, nil
	case 11:
		return []string{"abrir", org}, nil
	case 12:
		return []string{"cancelar-assinatura", org}, nil
	}
	if op == 7 {
		// O proprietário pode resolver excesso de assentos mesmo quando
		// o compartilhamento está suspenso por estorno ou redução de capacidade.
		var cadastro struct {
			Membros map[string]string `json:"membros"`
		}
		if err := client.Request(ctx, "GET", "/v1/corporativo/organizacoes/"+org, nil, &cadastro); err != nil {
			return nil, err
		}
		pessoas := map[string]string{}
		for id, papel := range cadastro.Membros {
			if papel != "proprietario" {
				pessoas[id] = papel + " · " + id
			}
		}
		p, err := escolherRecurso("Membro a remover", pessoas)
		if err != nil || p == "" {
			return nil, err
		}
		return []string{"remover", org, p}, nil
	}
	r, err := client.Recursos(ctx, org)
	if err != nil {
		return nil, err
	}
	pessoas := map[string]string{}
	for id, p := range r.Pessoas {
		pessoas[id] = p.Nome
	}
	workspaces := map[string]string{}
	for id, w := range r.Workspaces {
		workspaces[id] = w.Nome
	}
	equipes := map[string]string{}
	for id, e := range r.Equipes {
		equipes[id] = e.Nome
	}
	if op == 5 {
		nome, err := solicitarEmpresa("Nome da equipe")
		if err != nil {
			return nil, err
		}
		var membros []string
		for len(pessoas) > 0 {
			p, err := escolherRecurso("Selecione membros; Esc conclui a equipe", pessoas)
			if err != nil {
				return nil, err
			}
			if p == "" {
				break
			}
			membros = append(membros, p)
			delete(pessoas, p)
		}
		if len(membros) == 0 {
			return nil, nil
		}
		return []string{"equipe", org, nome, strings.Join(membros, ",")}, nil
	}
	if op == 1 {
		w, err := escolherRecurso("Workspace de destino", workspaces)
		if err != nil || w == "" {
			return nil, err
		}
		v, err := abrirCofreSemChaveConta(resolveVaultPath(""))
		if err != nil {
			return nil, err
		}
		defer v.Close()
		entradas := map[string]string{}
		for _, e := range v.Entries() {
			entradas[e.ID] = e.Title
		}
		id, err := escolherRecurso("Credencial pessoal para compartilhar", entradas)
		if err != nil || id == "" {
			return nil, err
		}
		return []string{"enviar", org, w, id}, nil
	}
	itens := map[string]string{}
	for id, i := range r.Segredos {
		itens[id] = r.Workspaces[i.Workspace].Nome + " · " + i.Papel
		data, _, err := client.Ler(ctx, org, id)
		if err != nil {
			return nil, err
		}
		var resumo struct {
			Titulo string `json:"title"`
		}
		err = json.Unmarshal(data, &resumo)
		mycrypto.ZeroBytes(data)
		if err != nil {
			return nil, errors.New("item compartilhado não contém uma credencial válida")
		}
		itens[id] = textoEmpresa(resumo.Titulo) + " · " + itens[id]
	}
	item, err := escolherRecurso("Itens compartilhados (identificados pelo workspace)", itens)
	if err != nil || item == "" {
		return nil, err
	}
	if op == 0 {
		return []string{"mostrar", org, item}, nil
	}
	if op == 4 {
		p, err := escolherRecurso("Pessoa que receberá acesso", pessoas)
		if err != nil || p == "" {
			return nil, err
		}
		return []string{"compartilhar", org, item, p}, nil
	}
	e, err := escolherRecurso("Equipe que receberá acesso", equipes)
	if err != nil || e == "" {
		return nil, err
	}
	return []string{"compartilhar-equipe", org, item, e}, nil
}
