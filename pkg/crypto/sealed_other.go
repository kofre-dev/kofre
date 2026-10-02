//go:build !windows

package crypto

import "crypto/rand"

// Defesa contra texto simples ocioso. A chave continua no processo nestas
// plataformas; isto não constitui isolamento contra leitura arbitrária da RAM.
func sealMemory(plain []byte) ([]byte, []byte, error) {
	key := make([]byte, KeyLength)
	if _, err := rand.Read(key); err != nil {
		ZeroBytes(key)
		return nil, nil, err
	}
	data, err := Encrypt(plain, key)
	if err != nil {
		ZeroBytes(key)
		return nil, nil, err
	}
	return data, key, nil
}

func openMemory(data, key []byte) ([]byte, error) { return Decrypt(data, key) }
