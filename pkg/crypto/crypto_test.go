package crypto

import (
	"bytes"
	"testing"
)

func TestArgon2AndAESGCM(t *testing.T) {
	secret := "minha-chave-mestra-1234"
	key1, salt1, err := DeriveKey(secret, nil)
	if err != nil {
		t.Fatalf("DeriveKey falhou: %v", err)
	}

	if len(key1) != KeyLength {
		t.Fatalf("tamanho de chave esperado %d, obtido %d", KeyLength, len(key1))
	}

	// Com o mesmo salt, deve derivar a mesma chave
	key2, _, err := DeriveKey(secret, salt1)
	if err != nil {
		t.Fatalf("DeriveKey com salt existente falhou: %v", err)
	}

	if !bytes.Equal(key1, key2) {
		t.Fatalf("chaves derivadas com o mesmo salt deveriam ser identicas")
	}

	// Teste de Criptografia e Decriptografia
	plaintext := []byte("segredo ultra confidencial: token-aws-xyz")
	encrypted, err := Encrypt(plaintext, key1)
	if err != nil {
		t.Fatalf("Encrypt falhou: %v", err)
	}

	decrypted, err := Decrypt(encrypted, key1)
	if err != nil {
		t.Fatalf("Decrypt falhou: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("esperado %s, obtido %s", string(plaintext), string(decrypted))
	}

	// Teste de adulteracao (tampering)
	encrypted[len(encrypted)-1] ^= 0xFF
	_, err = Decrypt(encrypted, key1)
	if err == nil {
		t.Fatalf("esperava erro de integridade ao decodificar payload adulterado")
	}
}

func TestPasswordGenerator(t *testing.T) {
	pwd := GenerateSecurePassword(32)
	if len(pwd) != 32 {
		t.Fatalf("esperado tamanho 32, obtido %d", len(pwd))
	}

	pwdDefault := GenerateSecurePassword(0)
	if len(pwdDefault) != 24 {
		t.Fatalf("esperado tamanho padrao 24, obtido %d", len(pwdDefault))
	}
}
