package main

import (
	"bytes"
	"kofre/pkg/corporativo"
	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/vault"
	"os"
	"path/filepath"
	"testing"
)

func prepararSessaoContaTeste(t *testing.T) *sessaoConta {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AWS_S3_BUCKET", "")
	t.Setenv("KOFRE_PIN", "senha-mestra-teste")
	anterior := vaultDaConta
	vaultDaConta = filepath.Join(dir, "cofre.enc")
	t.Cleanup(func() { vaultDaConta = anterior; contaEmUso = nil })
	v := vault.NewManaged()
	defer v.Close()
	key, salt, err := mycrypto.DeriveKeyBytes([]byte("senha-mestra-teste"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(key)
	b, err := v.Pack(key, salt)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(vaultDaConta, b, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := abrirSessaoConta(filepath.Join(dir, "identidade-empresa.enc"))
	if err != nil {
		t.Fatal(err)
	}
	contaEmUso = s
	t.Cleanup(s.fechar)
	return s
}

func TestContaNoCofreExportaComSenhaMestra(t *testing.T) {
	s := prepararSessaoContaTeste(t)
	i, err := corporativo.PrepararCadastro("Pessoa fictícia")
	if err != nil {
		t.Fatal(err)
	}
	defer i.Fechar()
	if err = salvarIdentidadeConta(s.identidade, i, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(s.identidade); !os.IsNotExist(err) {
		t.Fatal("cadastro criou arquivo com segunda senha")
	}
	got, err := abrirIdentidadeConta(s.identidade, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Fechar()
	publicaGot, _ := got.Publica()
	publicaOriginal, _ := i.Publica()
	if got.Token != i.Token || publicaGot != publicaOriginal {
		t.Fatal("identidade alterada")
	}
	backup := filepath.Join(filepath.Dir(s.arquivo), "backup.enc")
	if err = exportarIdentidadeConta(backup, i, nil); err != nil {
		t.Fatal(err)
	}
	if err = exportarIdentidadeConta(s.arquivo, i, nil); err == nil {
		t.Fatal("exportação substituiu cofre")
	}
	if err = exportarIdentidadeConta(backup, i, nil); err == nil {
		t.Fatal("exportação substituiu backup existente")
	}
	if errado, err := corporativo.AbrirIdentidade(backup, []byte("senha-errada")); err == nil {
		errado.Fechar()
		t.Fatal("backup aceitou senha errada")
	}
	portatil, err := corporativo.AbrirIdentidade(backup, []byte("senha-mestra-teste"))
	if err != nil {
		t.Fatal(err)
	}
	defer portatil.Fechar()
	if portatil.Token != i.Token {
		t.Fatal("backup perdeu conta")
	}
	b, err := os.ReadFile(s.arquivo)
	if err != nil {
		t.Fatal(err)
	}
	salt, payload, err := vault.UnpackHeader(b)
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := mycrypto.DeriveKeyBytes([]byte("senha-mestra-teste"), salt)
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(key)
	v, err := vault.DecryptAndLoad(payload, key, salt)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	if !v.TemConta() {
		t.Fatal("conta não persistida")
	}
}

func TestMigracaoPreservaBackupERecusaGravacaoDesatualizada(t *testing.T) {
	s := prepararSessaoContaTeste(t)
	i, err := corporativo.PrepararCadastro("Pessoa anterior")
	if err != nil {
		t.Fatal(err)
	}
	defer i.Fechar()
	senha := []byte("senha-separada-antiga")
	if err = corporativo.SalvarIdentidade(s.identidade, i, senha); err != nil {
		t.Fatal(err)
	}
	antes, _ := os.ReadFile(s.identidade)
	if err = migrarIdentidadeProtegida(s.identidade, []byte("incorreta")); err == nil || s.cofre.TemConta() {
		t.Fatal("migração aceitou senha incorreta")
	}
	if err = migrarIdentidadeProtegida(s.identidade, senha); err != nil {
		t.Fatal(err)
	}
	depois, _ := os.ReadFile(s.identidade)
	if !bytes.Equal(antes, depois) {
		t.Fatal("migração alterou backup anterior")
	}
	if err = os.WriteFile(s.arquivo, []byte("alteracao concorrente ficticia"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = salvarIdentidadeConta(s.identidade, i, nil); err == nil {
		t.Fatal("sobregravou outra alteração")
	}
	depois, _ = os.ReadFile(s.arquivo)
	if string(depois) != "alteracao concorrente ficticia" {
		t.Fatal("arquivo concorrente perdido")
	}
}
