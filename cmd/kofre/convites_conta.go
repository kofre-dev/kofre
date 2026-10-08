package main

import (
	"context"
	"fmt"
	"time"

	"kofre/pkg/corporativo"
)

func convitesRecebidosConta(client *corporativo.Client) error {
	var resposta struct {
		Convites []struct {
			ID          string    `json:"id"`
			Organizacao string    `json:"organizacao"`
			Empresa     string    `json:"empresa"`
			Validade    time.Time `json:"validade"`
		} `json:"convites"`
	}
	ctx := context.Background()
	if err := client.Request(ctx, "GET", "/v1/corporativo/convites", nil, &resposta); err != nil {
		return err
	}
	if len(resposta.Convites) == 0 {
		informarConta("Você não tem convites pendentes. A empresa deve enviar o convite para o e-mail da sua conta.")
		return nil
	}
	nomes := make([]string, len(resposta.Convites))
	for i, convite := range resposta.Convites {
		nomes[i] = textoEmpresa(convite.Empresa) + " · até " + convite.Validade.Format("02/01/2006")
	}
	i, err := escolherOpcaoConta("Convites recebidos", nomes)
	if err != nil || i < 0 {
		return err
	}
	convite := resposta.Convites[i]
	acao, err := escolherOpcaoContaExplicada("Convite · "+textoEmpresa(convite.Empresa), []string{"Aceitar convite", "Recusar convite", "Voltar"}, []string{"Participe da empresa. Suas credenciais pessoais continuam privadas.", "Remova este convite sem entrar na empresa.", "Volte sem alterar o convite."})
	if err != nil || acao < 0 || acao == 2 {
		return err
	}
	base := "/v1/corporativo/organizacoes/" + convite.Organizacao
	if acao == 0 {
		err = client.Request(ctx, "POST", base+"/aceitar", map[string]string{"convite_id": convite.ID}, nil)
		if err == nil {
			informarConta(fmt.Sprintf("Você agora participa de %s. Abra Empresas e equipes → Minhas empresas.", textoEmpresa(convite.Empresa)))
		}
	} else {
		err = client.Request(ctx, "POST", base+"/convites/"+convite.ID+"/recusar", nil, nil)
		if err == nil {
			informarConta("Convite recusado.")
		}
	}
	return err
}
