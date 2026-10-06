package corporativo

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCifraIsolaDestinatarioContextoEAdulteracao(t *testing.T) {
	a, _ := NovaIdentidade()
	defer a.Fechar()
	b, _ := NovaIdentidade()
	defer b.Fechar()
	pub, _ := a.Publica()
	conteudo := []byte("credencial exclusivamente ficticia")
	cifra, err := Cifrar(pub, "org", "item", "pessoa", 1, conteudo)
	if err != nil {
		t.Fatal(err)
	}
	aberto, err := Abrir(a.Privada, "org", "item", "pessoa", 1, cifra)
	if err != nil || !bytes.Equal(aberto, conteudo) {
		t.Fatal("destinatário legítimo não abriu")
	}
	casos := []struct {
		priv              []byte
		org, item, pessoa string
		v                 uint64
	}{{b.Privada, "org", "item", "pessoa", 1}, {a.Privada, "outra", "item", "pessoa", 1}, {a.Privada, "org", "outro", "pessoa", 1}, {a.Privada, "org", "item", "outra", 1}, {a.Privada, "org", "item", "pessoa", 2}}
	for _, c := range casos {
		if _, err = Abrir(c.priv, c.org, c.item, c.pessoa, c.v, cifra); err == nil {
			t.Fatal("cifra aceita no contexto errado")
		}
	}
	cifra[len(cifra)-1] ^= 1
	if _, err = Abrir(a.Privada, "org", "item", "pessoa", 1, cifra); err == nil {
		t.Fatal("adulteração aceita")
	}
	if _, err = Cifrar(pub, "org", "item", "pessoa", 1, make([]byte, 65537)); err == nil {
		t.Fatal("limite ignorado")
	}
}

func TestIdentidadeBackupProtegido(t *testing.T) {
	i, _ := NovaIdentidade()
	defer i.Fechar()
	i.ID = "conta-ficticia"
	i.Token = "token-ficticio"
	i.Recuperacao = "recuperacao-ficticia"
	i.RecuperacaoPendente = "solicitacao-ficticia"
	path := filepath.Join(t.TempDir(), "identidade.enc")
	senha := []byte("senha ficticia longa")
	if err := SalvarIdentidade(path, i, senha); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if bytes.Contains(b, []byte(i.Token)) || bytes.Contains(b, i.Privada) {
		t.Fatal("identidade gravada aberta")
	}
	restaurada, err := AbrirIdentidade(path, senha)
	if err != nil {
		t.Fatal(err)
	}
	defer restaurada.Fechar()
	if !bytes.Equal(i.Privada, restaurada.Privada) || i.Token != restaurada.Token || i.RecuperacaoPendente != restaurada.RecuperacaoPendente {
		t.Fatal("backup perdeu identidade ou recuperação pendente")
	}
	if _, err = AbrirIdentidade(path, []byte("senha errada longa")); err == nil {
		t.Fatal("senha errada aceita")
	}
	b[len(b)-1] ^= 1
	os.WriteFile(path, b, 0600)
	if _, err = AbrirIdentidade(path, senha); err == nil {
		t.Fatal("backup adulterado aceito")
	}
}

func TestAssinaturaIsolaAutorEDestinatario(t *testing.T) {
	i, _ := NovaIdentidade()
	defer i.Fechar()
	i.ID = "autor-ficticio"
	pub, _ := i.Publica()
	c := Copia{ChavePublica: pub, Cifra: make([]byte, 48)}
	if err := i.assinar("org", "item", "destino", "workspace", 1, &c); err != nil {
		t.Fatal(err)
	}
	if !verificarAssinatura("org", "item", "destino", "workspace", 1, c) {
		t.Fatal("assinatura legítima inválida")
	}
	if verificarAssinatura("org", "item", "outro", "workspace", 1, c) || verificarAssinatura("org", "item", "destino", "outro", 1, c) || verificarAssinatura("org", "item", "destino", "workspace", 2, c) {
		t.Fatal("contexto adulterado aceito")
	}
	c.Cifra[0] ^= 1
	if verificarAssinatura("org", "item", "destino", "workspace", 1, c) {
		t.Fatal("cifra alterada aceita")
	}
	c.Cifra[0] ^= 1
	c.Autor = "outro"
	if verificarAssinatura("org", "item", "destino", "workspace", 1, c) {
		t.Fatal("autor alterado aceito")
	}
}
