package crypto

import (
	"crypto/rand"
	"math/big"
)

const (
	lowerChars   = "abcdefghijklmnopqrstuvwxyz"
	upperChars   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	digitChars   = "0123456789"
	specialChars = "!@#$%^&*()-_=+[]{}|;:,.<>?"
	allChars     = lowerChars + upperChars + digitChars + specialChars
)

// GenerateSecurePassword gera uma senha forte com entropia criptografica
func GenerateSecurePassword(length int) string {
	if length <= 0 {
		length = 24
	}

	result := make([]byte, length)
	// Garante pelo menos um de cada tipo
	result[0] = randomChar(lowerChars)
	result[1] = randomChar(upperChars)
	result[2] = randomChar(digitChars)
	result[3] = randomChar(specialChars)

	// Preenche o resto com caracteres diversos
	for i := 4; i < length; i++ {
		result[i] = randomChar(allChars)
	}

	// Embaralha (Fisher-Yates)
	for i := length - 1; i > 0; i-- {
		jBig, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		j := int(jBig.Int64())
		result[i], result[j] = result[j], result[i]
	}

	return string(result)
}

func randomChar(charset string) byte {
	idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
	return charset[idx.Int64()]
}
