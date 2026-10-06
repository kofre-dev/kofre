package tui

import (
	"kofre/pkg/config"
	"kofre/pkg/storage"
	"path/filepath"
	"testing"
)

func TestRetomarContaRespeitaDestinoECloudDesativada(t *testing.T) {
	for _, caso := range []string{"s3_proprio", "s3_por_flag", "cloud_desativada", "cloud_ativada"} {
		t.Run(caso, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("APPDATA", dir)
			t.Setenv("XDG_CONFIG_HOME", dir)
			cfg := config.DefaultConfig()
			cfg.Mode, cfg.CloudEnabled, cfg.KofreToken = "kofre_cloud", true, "token-ficticio"
			var remoto storage.StorageProvider
			switch caso {
			case "s3_proprio", "s3_por_flag":
				remoto = storage.NewS3Storage(storage.S3Config{Bucket: "ficticio"})
				if caso == "s3_proprio" {
					cfg.Mode = "custom_s3"
				}
			case "cloud_desativada":
				remoto = storage.NewKofreCloudStorage("https://example.invalid", "token-ficticio")
				cfg.CloudEnabled, cfg.Mode = false, "local"
			}
			if err := config.SaveConfig(cfg); err != nil {
				t.Fatal(err)
			}
			local, err := storage.NewLocalStorage(filepath.Join(dir, "teste.enc"))
			if err != nil {
				t.Fatal(err)
			}
			m := NewModel(local)
			next, _ := m.retomarConta(contaRetornouMsg{local: local, remote: remoto})
			m = next.(Model)
			defer m.Close()
			if m.state != ViewUnlock {
				t.Fatal("retorno deixou o cofre aberto")
			}
			if caso == "cloud_desativada" {
				if m.storage != local {
					t.Fatal("nuvem desativada continuou conectada")
				}
				return
			}
			syncer, ok := m.storage.(*storage.SyncStorage)
			if !ok {
				t.Fatal("armazenamento esperado ausente")
			}
			l, r := syncer.Providers()
			if l != local {
				t.Fatal("cofre local foi trocado")
			}
			if caso == "cloud_ativada" {
				if _, ok := r.(*storage.KofreCloudStorage); !ok {
					t.Fatal("nuvem não ativada")
				}
			} else if r != remoto {
				t.Fatal("S3 próprio substituído por outro destino")
			}
		})
	}
}
