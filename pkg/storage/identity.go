package storage

import (
	"crypto/sha256"
	"encoding/hex"
)

// O marcador distingue contas sem persistir token ou credenciais em texto claro.
func syncDestination(provider StorageProvider) string {
	discriminator := provider.Location()
	switch p := provider.(type) {
	case *KofreCloudStorage:
		discriminator = p.endpoint + "\x00" + p.token
	case *S3Storage:
		discriminator = p.config.Endpoint + "\x00" + p.config.Bucket + "\x00" + p.config.Key + "\x00" + p.config.AccessKeyID
	}
	hash := sha256.Sum256([]byte(discriminator))
	return hex.EncodeToString(hash[:])
}
