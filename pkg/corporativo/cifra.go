package corporativo

import (
	"crypto/ecdh"
	"crypto/hpke"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// Info vincula cada cópia ao item, organização, destinatário e versão. Não há
// segredo comum para todos os espaços nem cifra reutilizável em outra conta.
func contexto(org, item, pessoa string, versao uint64) []byte {
	return []byte(fmt.Sprintf("kofre:corporativo:hpke:v1:%s:%s:%s:%d", org, item, pessoa, versao))
}

func Fingerprint(publica string) string {
	bytes, err := hex.DecodeString(publica)
	if err != nil || len(bytes) != 32 {
		return ""
	}
	h := sha256.Sum256(bytes)
	return hex.EncodeToString(h[:])
}

func Cifrar(publica, org, item, pessoa string, versao uint64, conteudo []byte) ([]byte, error) {
	if len(conteudo) > 64*1024 {
		return nil, errors.New("segredo excede 64 KiB")
	}
	b, err := hex.DecodeString(publica)
	if err != nil {
		return nil, err
	}
	pub, err := ecdh.X25519().NewPublicKey(b)
	if err != nil {
		return nil, err
	}
	kem, err := hpke.NewDHKEMPublicKey(pub)
	if err != nil {
		return nil, err
	}
	return hpke.Seal(kem, hpke.HKDFSHA256(), hpke.AES256GCM(), contexto(org, item, pessoa, versao), conteudo)
}

func Abrir(privada []byte, org, item, pessoa string, versao uint64, cifra []byte) ([]byte, error) {
	if len(cifra) > 64*1024+128 {
		return nil, errors.New("cifra excede limite")
	}
	priv, err := ecdh.X25519().NewPrivateKey(privada)
	if err != nil {
		return nil, err
	}
	kem, err := hpke.NewDHKEMPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	return hpke.Open(kem, hpke.HKDFSHA256(), hpke.AES256GCM(), contexto(org, item, pessoa, versao), cifra)
}
