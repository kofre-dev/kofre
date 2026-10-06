package corporativo

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strconv"

	"kofre/internal/arquivo"
	mycrypto "kofre/pkg/crypto"
)

type Identidade struct {
	ID                  string            `json:"id"`
	Nome                string            `json:"nome"`
	Email               string            `json:"email,omitempty"`
	Token               string            `json:"token"`
	Recuperacao         string            `json:"recuperacao"`
	Privada             []byte            `json:"privada"`
	Pins                map[string]string `json:"pins,omitempty"`
	RecuperacaoPendente string            `json:"recuperacao_pendente,omitempty"`
	privadaSelada       *mycrypto.SealedBuffer
}

// ProtegerPrivada encerra a cópia aberta enquanto menus ou rede aguardam.
func (i *Identidade) ProtegerPrivada() error {
	if i.privadaSelada != nil {
		return nil
	}
	if len(i.Privada) != 32 {
		return errors.New("chave de identidade inválida")
	}
	s, err := mycrypto.SealMemory(i.Privada)
	if err != nil {
		return err
	}
	mycrypto.ZeroBytes(i.Privada)
	i.Privada = nil
	i.privadaSelada = s
	return nil
}

func (i *Identidade) ComPrivada(use func([]byte) error) error {
	if i.privadaSelada != nil {
		return i.privadaSelada.WithBytes(use)
	}
	if len(i.Privada) != 32 {
		return errors.New("chave de identidade indisponível")
	}
	return use(i.Privada)
}

// Serializar evita os pools de json.Marshal para dados da conta. Apague o retorno.
func (i *Identidade) Serializar() ([]byte, error) {
	pins, err := json.Marshal(i.Pins)
	if err != nil {
		return nil, err
	}
	var out []byte
	err = i.ComPrivada(func(priv []byte) error {
		out = make([]byte, 0, 6*(len(i.ID)+len(i.Nome)+len(i.Email)+len(i.Token)+len(i.Recuperacao)+len(i.RecuperacaoPendente))+len(pins)+1024)
		out = append(out, '{')
		for n, f := range [][2]string{{"id", i.ID}, {"nome", i.Nome}, {"email", i.Email}, {"token", i.Token}, {"recuperacao", i.Recuperacao}, {"recuperacao_pendente", i.RecuperacaoPendente}} {
			if n > 0 {
				out = append(out, ',')
			}
			out = strconv.AppendQuote(out, f[0])
			out = append(out, ':')
			out = strconv.AppendQuote(out, f[1])
		}
		out = append(out, []byte(`,"privada":"`)...)
		out = base64.StdEncoding.AppendEncode(out, priv)
		out = append(out, []byte(`","pins":`)...)
		out = append(out, pins...)
		out = append(out, '}')
		return nil
	})
	return out, err
}

func NovaIdentidade() (*Identidade, error) {
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Identidade{Privada: k.Bytes(), Pins: map[string]string{}}, nil
}

func (i *Identidade) Publica() (string, error) {
	var publica string
	err := i.ComPrivada(func(priv []byte) error {
		k, e := ecdh.X25519().NewPrivateKey(priv)
		if e != nil {
			return e
		}
		publica = hex.EncodeToString(k.PublicKey().Bytes())
		return nil
	})
	return publica, err
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
	payload, err := CifrarIdentidadeComChave(i, key, salt)
	if err != nil {
		return err
	}
	return arquivo.Gravar(path, payload, mycrypto.RestrictFilePermissions)
}

func CifrarIdentidadeComChave(i *Identidade, key, salt []byte) ([]byte, error) {
	if len(key) != 32 || len(salt) != mycrypto.SaltLength {
		return nil, errors.New("chave ou salt inválido")
	}
	data, err := i.Serializar()
	if err != nil {
		return nil, err
	}
	defer mycrypto.ZeroBytes(data)
	if len(data) > 4*1024*1024-128 {
		return nil, errors.New("identidade excede limite de armazenamento")
	}
	cifra, err := mycrypto.Encrypt(data, key)
	if err != nil {
		return nil, err
	}
	payload := append([]byte("KFRID001"), salt...)
	payload = append(payload, cifra...)
	return payload, nil
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
	return AbrirIdentidadeBytes(payload, password)
}

func AbrirIdentidadeBytes(payload, password []byte) (*Identidade, error) {
	if len(payload) > 4*1024*1024 {
		return nil, errors.New("identidade excede limite")
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

func (i *Identidade) Fechar() {
	mycrypto.ZeroBytes(i.Privada)
	i.Privada = nil
	i.privadaSelada.Close()
	i.Token = ""
	i.Recuperacao = ""
	i.RecuperacaoPendente = ""
}
