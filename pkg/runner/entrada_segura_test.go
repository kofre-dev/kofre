package runner

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	mycrypto "kofre/pkg/crypto"
)

func TestEntradaVisualApagaBufferENaoUsaPINDoAmbiente(t *testing.T) {
	v := cofreRunner(t)
	key, salt, err := mycrypto.DeriveKeyBytes([]byte("senha-fixture-123"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(key)
	data, err := v.Pack(key, salt)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "vault.enc")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KOFRE_PIN", "senha-errada-no-ambiente")
	buf := []byte("senha-fixture-123")
	chamou := false
	aberto, k, _, err := UnlockVaultComEntrada(path, func() ([]byte, error) { chamou = true; return buf, nil })
	if err != nil {
		t.Fatal(err)
	}
	defer aberto.Close()
	defer mycrypto.ZeroBytes(k)
	if !chamou || !bytes.Equal(buf, make([]byte, len(buf))) {
		t.Fatal("leitor ignorado ou senha não apagada")
	}
	if aberto.Count() != 2 {
		t.Fatal("cofre fictício não foi autenticado")
	}
}

func TestCancelarEntradaNaoContaComoSenhaErrada(t *testing.T) {
	v := cofreRunner(t)
	key, salt, err := mycrypto.DeriveKeyBytes([]byte("senha-fixture-123"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mycrypto.ZeroBytes(key)
	data, err := v.Pack(key, salt)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "vault.enc")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cancelou := errors.New("cancelamento fictício")
	buf := []byte("parcial")
	_, _, _, err = UnlockVaultComEntrada(path, func() ([]byte, error) { return buf, cancelou })
	if !errors.Is(err, cancelou) || !bytes.Equal(buf, make([]byte, len(buf))) {
		t.Fatal("cancelamento ou limpeza falhou")
	}
	if _, err = os.Stat(path + ".tentativas.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelamento foi contabilizado como senha errada")
	}
}
