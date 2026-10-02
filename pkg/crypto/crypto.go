package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"runtime"
	"unsafe"

	"golang.org/x/crypto/argon2"
)

const (
	SaltLength   = 16
	NonceLength  = 12
	KeyLength    = 32
	ArgonTime    = 3
	ArgonMemory  = 64 * 1024 // 64 MB
	ArgonThreads = 4
)

var (
	ErrDecryptionFailed = errors.New("falha na descriptografia: chave incorreta ou dados corrompidos")
	ErrInvalidPayload   = errors.New("payload criptografado invalido")
)

// DeriveKey gera uma chave AES-256 de 32 bytes a partir de um segredo/passphrase usando Argon2id.
// Se salt for nil ou vazio, um novo salt criptograficamente seguro e gerado.
func DeriveKey(secret string, salt []byte) ([]byte, []byte, error) {
	password := []byte(secret)
	defer ZeroBytes(password)
	return DeriveKeyBytes(password, salt)
}

func DeriveKeyBytes(password, salt []byte) ([]byte, []byte, error) {
	if len(salt) == 0 {
		salt = make([]byte, SaltLength)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, nil, fmt.Errorf("falha ao gerar salt seguro: %w", err)
		}
	}

	key := argon2.IDKey(password, salt, ArgonTime, ArgonMemory, ArgonThreads, KeyLength)
	return key, salt, nil
}

// Encrypt encripta dados com AES-256-GCM.
// Formato do payload retornado: [12 bytes Nonce][Ciphertext + 16 bytes Auth Tag]
func Encrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("falha ao inicializar cifra AES: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("falha ao inicializar modo GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("falha ao gerar nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt descriptografa dados protegidos com AES-256-GCM.
func Decrypt(payload, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("falha ao inicializar cifra AES: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("falha ao inicializar modo GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(payload) < nonceSize {
		return nil, ErrInvalidPayload
	}

	nonce, ciphertext := payload[:nonceSize], payload[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// ZeroBytes limpa buffers de memoria sensiveis (chaves, senhas) sobrescrevendo com zeros de forma segura.
func ZeroBytes(b []byte) {
	WipeBytes(b)
}

// WipeString sobrescreve os bytes de uma string alocada no heap com zeros de forma imune a otimizacoes
func WipeString(s string) {
	if len(s) == 0 {
		return
	}
	p := unsafe.StringData(s)
	b := unsafe.Slice(p, len(s))
	for i := range b {
		b[i] = 0
	}
	runtime.KeepAlive(b)
}

