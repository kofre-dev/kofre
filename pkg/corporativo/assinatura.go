package corporativo

import (
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func (i *Identidade) chaveAssinatura() (ed25519.PrivateKey, error) {
	var seed []byte
	err := i.ComPrivada(func(priv []byte) error {
		var e error
		seed, e = hkdf.Key(sha256.New, priv, nil, "kofre:corporativo:ed25519:v1", 32)
		return e
	})
	if err != nil {
		return nil, err
	}
	defer clear(seed)
	return ed25519.NewKeyFromSeed(seed), nil
}

func (i *Identidade) AssinarMensagem(mensagem []byte) (string, error) {
	key, err := i.chaveAssinatura()
	if err != nil {
		return "", err
	}
	defer clear(key)
	return hex.EncodeToString(ed25519.Sign(key, mensagem)), nil
}
func (i *Identidade) PublicaAssinatura() (string, error) {
	key, err := i.chaveAssinatura()
	if err != nil {
		return "", err
	}
	defer clear(key)
	return hex.EncodeToString(key.Public().(ed25519.PublicKey)), nil
}
func FingerprintIdentidade(publica, assinatura string) string {
	a, e := hex.DecodeString(publica)
	b, e2 := hex.DecodeString(assinatura)
	if e != nil || e2 != nil || len(a) != 32 || len(b) != 32 {
		return ""
	}
	h := sha256.Sum256(append(a, b...))
	return hex.EncodeToString(h[:])
}
func mensagemAssinada(org, item, pessoa, workspace string, versao uint64, c Copia) []byte {
	b, _ := json.Marshal([]any{"kofre:corporativo:assinatura:v1", org, item, pessoa, workspace, versao, c.Autor, c.PublicaAutor, c.AssinaturaPublica, c.ChavePublica, c.Cifra})
	return b
}
func (i *Identidade) assinar(org, item, pessoa, workspace string, versao uint64, c *Copia) error {
	key, err := i.chaveAssinatura()
	if err != nil {
		return err
	}
	defer clear(key)
	c.Autor = i.ID
	c.PublicaAutor, _ = i.Publica()
	c.AssinaturaPublica = hex.EncodeToString(key.Public().(ed25519.PublicKey))
	c.Assinatura = ed25519.Sign(key, mensagemAssinada(org, item, pessoa, workspace, versao, *c))
	return nil
}
func verificarAssinatura(org, item, pessoa, workspace string, versao uint64, c Copia) bool {
	key, err := hex.DecodeString(c.AssinaturaPublica)
	return err == nil && len(key) == 32 && ed25519.Verify(key, mensagemAssinada(org, item, pessoa, workspace, versao, c), c.Assinatura)
}
