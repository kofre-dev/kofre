package corporativo

import (
	"context"
	"fmt"
	mycrypto "kofre/pkg/crypto"
)

type Pessoa struct {
	Nome            string `json:"nome"`
	ChavePublica    string `json:"chave_publica"`
	ChaveAssinatura string `json:"chave_assinatura"`
}
type Equipe struct {
	Nome    string   `json:"nome"`
	Membros []string `json:"membros"`
}
type Workspace struct {
	Nome string `json:"nome"`
}
type ItemResumo struct {
	Workspace string `json:"workspace"`
	Versao    uint64 `json:"versao"`
	Papel     string `json:"papel"`
}
type Recursos struct {
	Pessoas    map[string]Pessoa     `json:"pessoas"`
	Equipes    map[string]Equipe     `json:"equipes"`
	Workspaces map[string]Workspace  `json:"workspaces"`
	Segredos   map[string]ItemResumo `json:"segredos"`
}
type Acesso struct {
	Pessoas map[string]string `json:"pessoas"`
	Equipes map[string]string `json:"equipes"`
}
type Copia struct {
	ChavePublica      string `json:"chave_publica"`
	Cifra             []byte `json:"cifra"`
	Autor             string `json:"autor"`
	PublicaAutor      string `json:"publica_autor"`
	AssinaturaPublica string `json:"assinatura_publica"`
	Assinatura        []byte `json:"assinatura"`
}
type Item struct {
	Workspace string           `json:"workspace"`
	Versao    uint64           `json:"versao"`
	Acesso    Acesso           `json:"acesso"`
	Copias    map[string]Copia `json:"copias"`
}
type ItemRecebido struct {
	Workspace string `json:"workspace"`
	Versao    uint64 `json:"versao"`
	Acesso    Acesso `json:"acesso"`
	Copia     Copia  `json:"copia"`
}

func (c *Client) Recursos(ctx context.Context, org string) (*Recursos, error) {
	if !IDValido(org) {
		return nil, fmt.Errorf("organização inválida")
	}
	var result Recursos
	if err := c.Request(ctx, "GET", "/v1/corporativo/organizacoes/"+org+"/recursos", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) Ler(ctx context.Context, org, id string) ([]byte, *ItemRecebido, error) {
	if !IDValido(org) || !IDValido(id) {
		return nil, nil, fmt.Errorf("recurso inválido")
	}
	var item ItemRecebido
	if err := c.Request(ctx, "GET", "/v1/corporativo/organizacoes/"+org+"/segredos/"+id, nil, &item); err != nil {
		return nil, nil, err
	}
	publica, err := c.Identidade.Publica()
	if err != nil || publica != item.Copia.ChavePublica {
		return nil, nil, fmt.Errorf("cópia não pertence à sua chave de identidade")
	}
	if !verificarAssinatura(org, id, c.Identidade.ID, item.Workspace, item.Versao, item.Copia) {
		return nil, nil, fmt.Errorf("assinatura do item inválida")
	}
	if err = c.confiar(item.Copia.Autor, Pessoa{Nome: item.Copia.Autor, ChavePublica: item.Copia.PublicaAutor, ChaveAssinatura: item.Copia.AssinaturaPublica}, c.ConfirmarAutor); err != nil {
		return nil, nil, err
	}
	var data []byte
	err = c.Identidade.ComPrivada(func(priv []byte) error {
		var e error
		data, e = Abrir(priv, org, id, c.Identidade.ID, item.Versao, item.Copia.Cifra)
		return e
	})
	return data, &item, err
}

func (c *Client) Gravar(ctx context.Context, org, id, workspace string, versao uint64, a Acesso, data []byte, confirmar func(string, Pessoa, string) bool) error {
	protegido, err := mycrypto.SealMemory(data)
	if err != nil {
		return err
	}
	defer protegido.Close()
	return c.GravarProtegido(ctx, org, id, workspace, versao, a, protegido, confirmar)
}

// GravarProtegido mantém o conteúdo selado durante rede e confirmação de chaves.
// O chamador deve apagar a cópia de origem antes de chamar esta função.
func (c *Client) GravarProtegido(ctx context.Context, org, id, workspace string, versao uint64, a Acesso, data *mycrypto.SealedBuffer, confirmar func(string, Pessoa, string) bool) error {
	if !IDValido(org) || !IDValido(id) || !IDValido(workspace) || versao == 0 {
		return fmt.Errorf("recurso ou versão inválida")
	}
	if a.Pessoas == nil {
		a.Pessoas = map[string]string{}
	}
	if a.Equipes == nil {
		a.Equipes = map[string]string{}
	}
	diretorio, err := c.Recursos(ctx, org)
	if err != nil {
		return err
	}
	destinatarios := map[string]bool{}
	for pessoa := range a.Pessoas {
		destinatarios[pessoa] = true
	}
	for idEquipe := range a.Equipes {
		equipe, ok := diretorio.Equipes[idEquipe]
		if !ok {
			return fmt.Errorf("equipe não encontrada")
		}
		for _, pessoa := range equipe.Membros {
			destinatarios[pessoa] = true
		}
	}
	copias := map[string]Copia{}
	if c.Identidade.Pins == nil {
		c.Identidade.Pins = map[string]string{}
	}
	for idPessoa := range destinatarios {
		pessoa, ok := diretorio.Pessoas[idPessoa]
		if !ok || pessoa.ChavePublica == "" {
			return fmt.Errorf("destinatário %s precisa configurar sua identidade", idPessoa)
		}
		if err = c.confiar(idPessoa, pessoa, confirmar); err != nil {
			return err
		}
		var cifra []byte
		err := data.WithBytes(func(aberto []byte) error {
			var e error
			cifra, e = Cifrar(pessoa.ChavePublica, org, id, idPessoa, versao, aberto)
			return e
		})
		if err != nil {
			return err
		}
		copia := Copia{ChavePublica: pessoa.ChavePublica, Cifra: cifra}
		if err = c.Identidade.assinar(org, id, idPessoa, workspace, versao, &copia); err != nil {
			return err
		}
		copias[idPessoa] = copia
	}
	return c.Request(ctx, "PUT", "/v1/corporativo/organizacoes/"+org+"/segredos/"+id, Item{workspace, versao, a, copias}, nil)
}

func (c *Client) confiar(id string, pessoa Pessoa, confirmar func(string, Pessoa, string) bool) error {
	if !IDValido(id) {
		return fmt.Errorf("identidade inválida")
	}
	fingerprint := FingerprintIdentidade(pessoa.ChavePublica, pessoa.ChaveAssinatura)
	if fingerprint == "" {
		return fmt.Errorf("chaves inválidas da identidade")
	}
	if c.Identidade.Pins == nil {
		c.Identidade.Pins = map[string]string{}
	}
	if antiga := c.Identidade.Pins[id]; antiga != "" && antiga != fingerprint {
		return fmt.Errorf("chave de %s mudou; restaure ou confirme a identidade por outro canal", id)
	}
	if c.Identidade.Pins[id] == "" {
		if id == c.Identidade.ID {
			pub, _ := c.Identidade.Publica()
			sign, _ := c.Identidade.PublicaAssinatura()
			if pub != pessoa.ChavePublica || sign != pessoa.ChaveAssinatura {
				return fmt.Errorf("sua chave foi substituída")
			}
		} else if confirmar == nil || !confirmar(id, pessoa, fingerprint) {
			return fmt.Errorf("fingerprint não confirmado")
		}
		c.Identidade.Pins[id] = fingerprint
	}
	return nil
}
