package main

import (
	"kofre/pkg/config"
	"kofre/pkg/storage"
	"testing"
)

func TestS3ProprioTemPrioridadeSobreTokenAntigo(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := config.DefaultConfig()
	cfg.Mode, cfg.CloudEnabled = "custom_s3", true
	cfg.KofreToken, cfg.S3Bucket = "token-antigo-ficticio", "bucket-ficticio"
	provider, err := resolveCloudOrS3Storage(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := provider.(*storage.S3Storage); !ok {
		t.Fatal("token antigo desviou sincronização do S3 próprio")
	}
}
