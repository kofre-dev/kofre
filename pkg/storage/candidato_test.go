package storage

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/vault"
)

func cifraCandidata(t *testing.T, senha string) []byte {
	t.Helper()
	v := vault.NewManaged()
	defer v.Close()
	if _, err := v.AddEntry(vault.SecretEntry{Title: "fixture", Fields: []vault.Field{{Name: "senha", Value: "somente-teste", Protected: true}}}); err != nil {
		t.Fatal(err)
	}
	key, salt, err := mycrypto.DeriveKeyBytes([]byte(senha), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(key)
	data, err := v.Pack(key, salt)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func autenticarCandidato(data []byte, senha string) error {
	salt, payload, err := vault.UnpackHeader(data)
	if err != nil {
		return err
	}
	key, _, err := mycrypto.DeriveKeyBytes([]byte(senha), salt)
	if err != nil {
		return err
	}
	defer mycrypto.ZeroBytes(key)
	v, err := vault.DecryptAndLoad(payload, key, salt)
	if err != nil {
		return err
	}
	v.Close()
	return nil
}

func TestPrimeiraBaixadaSomenteInstalaDepoisDaAutenticacao(t *testing.T) {
	ctx := context.Background()
	correto := cifraCandidata(t, "senha-fixture-123")
	corrompido := bytes.Clone(correto)
	corrompido[len(corrompido)-1] ^= 1
	for nome, data := range map[string][]byte{"correto": correto, "tag-corrompida": corrompido, "formato-invalido": []byte("nao-e-cofre")} {
		t.Run(nome, func(t *testing.T) {
			local := localFixture(t)
			s := NewSyncStorage(local, &remoteFixture{data: data})
			defer s.Close()
			candidato, err := s.Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := local.Load(ctx); !errors.Is(err, ErrNotFound) {
				t.Fatal("download foi instalado antes de autenticar")
			}
			if err := s.Save(ctx, correto); err == nil {
				t.Fatal("Save ignorou candidato não confirmado")
			}
			if err := autenticarCandidato(candidato, "senha-fixture-123"); err != nil {
				if nome == "correto" {
					t.Fatal(err)
				}
				if _, err := local.Load(ctx); !errors.Is(err, ErrNotFound) {
					t.Fatal("candidato inválido criou cofre local")
				}
				return
			}
			if err := ConfirmarCarga(ctx, s, candidato); err != nil {
				t.Fatal(err)
			}
			instalado, err := local.Load(ctx)
			if err != nil || !bytes.Equal(instalado, correto) {
				t.Fatal("candidato autenticado não instalado", err)
			}
		})
	}
}

func TestDownloadCorrompidoOuSenhaAlteradaPreservaLocalERevisao(t *testing.T) {
	ctx := context.Background()
	inicial := cifraCandidata(t, "senha-antiga-123")
	novo := cifraCandidata(t, "senha-nova-456")
	for _, corromper := range []bool{false, true} {
		t.Run(map[bool]string{false: "senha-alterada", true: "tag-invalida"}[corromper], func(t *testing.T) {
			remoto := bytes.Clone(novo)
			if corromper {
				remoto[len(remoto)-1] ^= 1
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Error("download enviou alteração")
					http.Error(w, "inesperado", 500)
					return
				}
				w.Header().Set("ETag", revisaoArquivoCloud(remoto))
				_, _ = w.Write(remoto)
			}))
			defer srv.Close()
			local := localFixture(t)
			if err := local.Save(ctx, inicial); err != nil {
				t.Fatal(err)
			}
			cloud := NewKofreCloudStorage(srv.URL, "fixture-nao-real")
			s := NewSyncStorage(local, cloud)
			defer s.Close()
			if err := cloud.guardarRevisao(revisaoArquivoCloud(inicial)); err != nil {
				t.Fatal(err)
			}
			candidato, err := s.Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := autenticarCandidato(candidato, "senha-antiga-123"); err == nil {
				t.Fatal("senha antiga abriu novo candidato")
			}
			guardado, _ := local.Load(ctx)
			if !bytes.Equal(guardado, inicial) || !cloud.coincideComBase(inicial) {
				t.Fatal("falha de autenticação alterou base local ou revisão")
			}
			backups, _ := filepath.Glob(local.Path() + ".backup-*.enc")
			if len(backups) != 0 {
				t.Fatal("download não autenticado gerou backup/promoção")
			}
			if err := autenticarCandidato(candidato, "senha-nova-456"); err != nil {
				if !corromper {
					t.Fatal(err)
				}
				return
			}
			if corromper {
				t.Fatal("cifra adulterada abriu")
			}
			if err := ConfirmarCarga(ctx, s, candidato); err != nil {
				t.Fatal(err)
			}
			if !cloud.coincideComBase(novo) {
				t.Fatal("cópia autenticada não virou base")
			}
			backups, _ = filepath.Glob(local.Path() + ".backup-*.enc")
			if len(backups) != 1 {
				t.Fatal("backup anterior não preservado")
			}
			backup, _ := os.ReadFile(backups[0])
			if !bytes.Equal(backup, inicial) {
				t.Fatal("backup não contém estado anterior")
			}
		})
	}
}

func TestConfirmacaoNaoDescartaAlteracaoLocalDuranteAbertura(t *testing.T) {
	ctx := context.Background()
	data := cifraCandidata(t, "senha-fixture-123")
	local := localFixture(t)
	s := NewSyncStorage(local, &remoteFixture{data: data})
	defer s.Close()
	candidato, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := autenticarCandidato(candidato, "senha-fixture-123"); err != nil {
		t.Fatal(err)
	}
	if err := local.Save(ctx, []byte("criado por outro processo")); err != nil {
		t.Fatal(err)
	}
	if err := ConfirmarCarga(ctx, s, candidato); !errors.Is(err, ErrConflito) {
		t.Fatal("confirmação sobrescreveu arquivo criado em paralelo", err)
	}
	guardado, _ := local.Load(ctx)
	if string(guardado) != "criado por outro processo" {
		t.Fatal("perdeu arquivo concorrente")
	}
}

func TestMigracaoLegadaCriaUmBackupMesmoEmSaveComBackup(t *testing.T) {
	ctx := context.Background()
	key, salt, err := mycrypto.DeriveKeyBytes([]byte("senha-fixture-123"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(key)
	cipher, err := mycrypto.Encrypt([]byte(`{"schema_version":1,"entries":[]}`), key)
	if err != nil {
		t.Fatal(err)
	}
	legado := append(append(bytes.Clone(vault.MagicHeader), salt...), cipher...)
	novo := cifraCandidata(t, "senha-fixture-123")
	for _, explicito := range []bool{false, true} {
		local := localFixture(t)
		if err := local.Save(ctx, legado); err != nil {
			t.Fatal(err)
		}
		if explicito {
			_, err = local.SaveComBackup(ctx, novo)
		} else {
			err = local.Save(ctx, novo)
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := local.Save(ctx, novo); err != nil {
			t.Fatal(err)
		}
		backups, _ := filepath.Glob(local.Path() + ".backup-*.enc")
		if len(backups) != 1 {
			t.Fatal("migração sem backup único", backups)
		}
		backup, _ := os.ReadFile(backups[0])
		if !bytes.Equal(backup, legado) || autenticarCandidato(backup, "senha-fixture-123") != nil {
			t.Fatal("backup legado alterado ou não autenticado")
		}
	}
}
