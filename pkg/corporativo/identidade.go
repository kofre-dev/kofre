package corporativo

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"

	"kofre/internal/arquivo"
	mycrypto "kofre/pkg/crypto"
)

type Identidade struct {
	ID                  string            `json:"id"`
	Nome                string            `json:"nome"`
	Token               string            `json:"token"`
	Recuperacao         string            `json:"recuperacao"`
	Privada             []byte            `json:"privada"`
	Pins                map[string]string `json:"pins,omitempty"`
	RecuperacaoPendente string            `json:"recuperacao_pendente,omitempty"`
}

func NovaIdentidade() (*Identidade, error) {
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Identidade{Privada: k.Bytes(), Pins: map[string]string{}}, nil
}

func (i *Identidade) Publica() (string, error) {
	k, err := ecdh.X25519().NewPrivateKey(i.Privada)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(k.PublicKey().Bytes()), nil
}

func SalvarIdentidade(path string, i *Identidade, password []byte) error {
	if len(password) < 12 {
		return errors.New("use senha de identidade com pelo menos 12 bytes")
	}
	key, salt, err := mycrypto.DeriveKeyBytes(password, nil)
	if err != nil {
		return err
	}
	defer mycrypto.ZeroBytes(key)
	return SalvarIdentidadeComChave(path, i, key, salt)
}

// Exporta a identidade com a mesma chave/salt do cofre, sem reter a senha mestra.
func SalvarIdentidadeComChave(path string, i *Identidade, key, salt []byte) error {
	if len(key) != 32 || len(salt) != mycrypto.SaltLength {
		return errors.New("chave ou salt inválido")
	}
	data, err := json.Marshal(i)
	if err != nil {
		return err
	}
	defer mycrypto.ZeroBytes(data)
	if len(data) > 4*1024*1024-128 {
		return errors.New("identidade excede limite de armazenamento")
	}
	cifra, err := mycrypto.Encrypt(data, key)
	if err != nil {
		return err
	}
	payload := append([]byte("KFRID001"), salt...)
	payload = append(payload, cifra...)
	return arquivo.Gravar(path, payload, mycrypto.RestrictFilePermissions)
}

func AbrirIdentidade(path string, password []byte) (*Identidade, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > 4*1024*1024 {
		return nil, errors.New("arquivo de identidade excede limite")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(payload) < 52 || string(payload[:8]) != "KFRID001" {
		return nil, errors.New("arquivo de identidade inválido")
	}
	key, _, err := mycrypto.DeriveKeyBytes(password, payload[8:24])
	if err != nil {
		return nil, err
	}
	defer mycrypto.ZeroBytes(key)
	data, err := mycrypto.Decrypt(payload[24:], key)
	if err != nil {
		return nil, err
	}
	defer mycrypto.ZeroBytes(data)
	var i Identidade
	if json.Unmarshal(data, &i) != nil || len(i.Privada) != 32 {
		return nil, errors.New("identidade inválida")
	}
	if i.Pins == nil {
		i.Pins = map[string]string{}
	}
	return &i, nil
}

func (i *Identidade) Fechar() { mycrypto.ZeroBytes(i.Privada); i.Token = ""; i.Recuperacao = "" }
