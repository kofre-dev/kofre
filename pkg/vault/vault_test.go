package vault

import (
	"bytes"
	"testing"

	mycrypto "kofre/pkg/crypto"
)

func TestVaultLifecycle(t *testing.T) {
	v := NewManaged()

	// Adiciona entradas
	e1, err := v.AddEntry(SecretEntry{
		Title:    "AWS Deployer",
		Category: CategoryToken,
		Fields: []Field{
			{Name: "AccessKey", Value: "AKIA12345", Protected: false},
			{Name: "SecretKey", Value: "SuperSecretKey999", Protected: true},
		},
		Notes: "Chave do bot de CI/CD",
	})

	if err != nil {
		t.Fatal(err)
	}
	e2, err := v.AddEntry(SecretEntry{
		Title:    "Servidor Linux Producao",
		Category: CategorySSHKey,
		Fields: []Field{
			{Name: "Host", Value: "192.168.1.100", Protected: false},
			{Name: "User", Value: "root", Protected: false},
		},
		Notes: "Porta 2222",
	})

	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	if v.Count() != 2 {
		t.Fatalf("esperado 2 entradas, obtido %d", v.Count())
	}

	// Busca
	results := v.Search("aws", "")
	if len(results) != 1 || results[0].ID != e1.ID {
		t.Fatalf("falha ao buscar por 'aws'")
	}

	// Busca por categoria
	sshResults := v.Search("", CategorySSHKey)
	if len(sshResults) != 1 || sshResults[0].ID != e2.ID {
		t.Fatalf("falha ao filtrar por categoria SSH")
	}

	// Pack & Unpack com criptografia
	key, salt, err := mycrypto.DeriveKey("senha-teste", nil)
	if err != nil {
		t.Fatalf("DeriveKey falhou: %v", err)
	}

	packed, err := v.Pack(key, salt)
	if err != nil {
		t.Fatalf("Pack falhou: %v", err)
	}

	// UnpackHeader
	extractedSalt, payload, err := UnpackHeader(packed)
	if err != nil {
		t.Fatalf("UnpackHeader falhou: %v", err)
	}

	if !bytes.Equal(salt, extractedSalt) {
		t.Fatalf("salt extraido nao confere")
	}

	// DecryptAndLoad
	loadedVault, err := DecryptAndLoad(payload, key, extractedSalt)
	if err != nil {
		t.Fatalf("DecryptAndLoad falhou: %v", err)
	}

	if loadedVault.Count() != 2 {
		t.Fatalf("esperado 2 entradas no cofre decriptado, obtido %d", loadedVault.Count())
	}

	retrieved, err := loadedVault.GetEntry(e1.ID)
	if err != nil {
		t.Fatalf("GetEntry falhou: %v", err)
	}

	if retrieved.Title != "AWS Deployer" {
		t.Fatalf("titulo esperado 'AWS Deployer', obtido '%s'", retrieved.Title)
	}
}
