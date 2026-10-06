package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"kofre/pkg/corporativo"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/runner"
	"kofre/pkg/tui"
	"sort"
	"strings"
	"time"
)

func painelConta() error {
	for {
		i, err := tui.EscolherOpcao("Kofre · Conta e empresas", []string{"Criar conta gratuita por e-mail", "Minha conta / administrar empresas", "Entrar por e-mail neste PC", "Recuperar acesso por e-mail", "Conhecer Pro pessoal", "Histórico do cofre pessoal", "Sessões e dispositivos", "Atualizar backup da conta na nuvem", "Encerrar acesso deste PC", "Restaurar backup manual", "Voltar ao cofre"})
		if err != nil {
			return err
		}
		if i < 0 || i == 10 {
			return nil
		}
		switch i {
		case 0:
			err = executarEmpresa([]string{"criar-conta"})
		case 1:
			err = executarEmpresa([]string{"painel"})
		case 2:
			err = executarEmpresa([]string{"entrar"})
		case 3:
			err = executarEmpresa([]string{"recuperar-email"})
		case 4:
			handlePro()
		case 5:
			err = executarHistorico([]string{"painel"})
		case 6:
			err = executarEmpresa([]string{"sessoes"})
		case 7:
			err = executarEmpresa([]string{"backup-conta"})
		case 8:
			err = executarEmpresa([]string{"sair-conta"})
		case 9:
			err = executarEmpresa([]string{"restaurar", perguntarEmpresa("Arquivo de identidade .enc")})
		}
		if err != nil {
			fmt.Println("Não foi possível concluir:", err)
		}
		perguntarEmpresa("Enter para voltar ao menu")
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
	i, err := tui.EscolherOpcao(titulo, nomes)
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
		op, err := tui.EscolherOpcao("Conta e empresas · "+identidade.Nome, []string{"Minha identidade / código para receber convites", "Conectar nuvem pessoal Free", "Minhas empresas e credenciais compartilhadas", "Contratar para uma nova empresa", "Aceitar convite de uma empresa", "Exportar backup protegido da identidade", "Voltar"})
		if err != nil {
			return err
		}
		if op < 0 || op == 6 {
			return nil
		}
		var args []string
		switch op {
		case 0:
			args = []string{"fingerprint"}
		case 1:
			args = []string{"conectar"}
		case 2:
			args, err = selecionarAcaoOrganizacao(client)
			if err == nil {
				err = salvarIdentidadeConta(path, identidade, senha)
			}
		case 3:
			args = []string{"comprar"}
		case 4:
			args = []string{"aceitar", perguntarEmpresa("ID da organização recebido no convite")}
		case 5:
			args = []string{"exportar", perguntarEmpresa("Caminho do arquivo de backup .enc")}
		}
		if err == nil && len(args) > 0 {
			err = executarAcaoEmpresa(args, path, senha, identidade, client)
		}
		if err != nil {
			fmt.Println("Não foi possível concluir:", err)
		}
		if len(args) > 0 || err != nil {
			perguntarEmpresa("Enter para voltar")
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
	op, err := tui.EscolherOpcao(opcoes[org], []string{"Abrir credencial compartilhada", "Compartilhar credencial pessoal", "Criar workspace", "Convidar pessoa", "Compartilhar item com pessoa", "Criar equipe", "Compartilhar item com equipe", "Remover membro", "Assinatura e assentos", "Alterar quantidade de assentos", "Renovar contrato", "Ver pessoas, equipes e workspaces", "Cancelar renovação automática"})
	if err != nil || op < 0 {
		return nil, err
	}
	switch op {
	case 2:
		return []string{"workspace", org, perguntarEmpresa("Nome do workspace")}, nil
	case 3:
		return []string{"convidar", org, perguntarEmpresa("ID da conta da pessoa (ela encontra em Minha identidade)")}, nil
	case 8:
		return []string{"assinatura", org}, nil
	case 9:
		return []string{"assentos", org, perguntarEmpresa("Quantidade total desejada")}, nil
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
		nome := perguntarEmpresa("Nome da equipe")
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
		v, err := runner.UnlockVault(resolveVaultPath(""))
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
