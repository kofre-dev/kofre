package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	mycrypto "kofre/pkg/crypto"
	"kofre/pkg/vault"
)

func cofreRunner(t *testing.T) *vault.ManagedVault {
	t.Helper()
	v := vault.NewManaged()
	t.Cleanup(v.Close)
	for _, entrada := range []vault.SecretEntry{
		{ID: "primeira", Title: "Produção", Fields: []vault.Field{{Name: "API_TOKEN", Value: "segredo-ficticio-primeiro", Protected: true}, {Name: "OUTRO", Value: "nao-enviar", Protected: true}}},
		{ID: "segunda", Title: "Produção reserva", Fields: []vault.Field{{Name: "OUTRA_CONTA", Value: "nao-enviar", Protected: true}}},
	} {
		if _, err := v.AddEntry(entrada); err != nil {
			t.Fatal(err)
		}
	}
	return v
}

func TestSelecaoExigeEntradaECamposExatos(t *testing.T) {
	v := cofreRunner(t)
	for _, caso := range []struct {
		entrada string
		campos  []string
	}{
		{"", []string{"API_TOKEN"}}, {"primeira", nil}, {"Produ", []string{"API_TOKEN"}},
		{"primeira", []string{"API"}}, {"primeira", []string{""}},
		{"primeira", []string{"API_TOKEN", "API_TOKEN"}},
	} {
		if valores, err := valoresSelecionados(v, caso.entrada, caso.campos); err == nil {
			limparValores(valores)
			t.Errorf("seleção inválida aceita: %+v", caso)
		}
	}
	for _, seletor := range []string{"primeira", "Produção"} {
		valores, err := valoresSelecionados(v, seletor, []string{"API_TOKEN"})
		if err != nil {
			t.Fatal(err)
		}
		if len(valores) != 1 || string(valores["API_TOKEN"]) != "segredo-ficticio-primeiro" {
			t.Fatal("seleção vazou outros campos")
		}
		limparValores(valores)
	}
	if _, err := v.AddEntry(vault.SecretEntry{ID: "duplicada", Title: "Produção"}); err != nil {
		t.Fatal(err)
	}
	if _, err := valoresSelecionados(v, "Produção", []string{"API_TOKEN"}); err == nil {
		t.Fatal("título ambíguo aceito")
	}
	valores, err := valoresSelecionados(v, "primeira", []string{"API_TOKEN"})
	if err != nil {
		t.Fatal("ID exato deixou de selecionar", err)
	}
	limparValores(valores)
}

func TestSelecaoRecusaCampoReservadoColisaoENulo(t *testing.T) {
	for _, campos := range [][]vault.Field{
		{{Name: "kofre_pin", Value: "ficticio", Protected: true}},
		{{Name: "MYCOFRE_PIN", Value: "ficticio", Protected: true}},
		{{Name: "A-B", Value: "1"}, {Name: "A_B", Value: "2"}},
		{{Name: "TOKEN", Value: "x\x00y", Protected: true}},
		{{Name: "TOKEN", Value: "1"}, {Name: "TOKEN", Value: "2"}},
	} {
		v := vault.NewManaged()
		if _, err := v.AddEntry(vault.SecretEntry{ID: "entrada", Fields: campos}); err != nil {
			t.Fatal(err)
		}
		nomes := []string{campos[0].Name}
		if len(campos) > 1 && campos[1].Name != campos[0].Name {
			nomes = append(nomes, campos[1].Name)
		}
		valores, err := valoresSelecionados(v, "entrada", nomes)
		v.Close()
		if err == nil {
			limparValores(valores)
			t.Fatal("campo inválido aceito")
		}
	}
}

func TestAmbienteRemovePINInclusiveComCaixaDiferente(t *testing.T) {
	ambiente := ambienteFilho([]string{"PATH=caminho", "KOFRE_PIN=nao-enviar", "kofre_pin=nao-enviar", "MyCoFrE_pIn=nao-enviar", "API_TOKEN=anterior"}, map[string][]byte{"API_TOKEN": []byte("selecionado")})
	for _, item := range ambiente {
		nome, _, _ := strings.Cut(item, "=")
		if variavelDeDesbloqueio(nome) {
			t.Fatal("PIN vazou para ambiente filho")
		}
	}
	if strings.Join(ambiente, "|") != "PATH=caminho|API_TOKEN=selecionado" {
		t.Fatal("ambiente não respeitou seleção/substituição")
	}
}

func TestRunnerProcessoFilho(t *testing.T) {
	if os.Getenv("KOFRE_FIXTURE_FILHO") != "1" {
		return
	}
	if os.Getenv("KOFRE_PIN") != "" || os.Getenv("MYCOFRE_PIN") != "" || os.Getenv("OUTRO") != "" || os.Getenv("OUTRA_CONTA") != "" {
		os.Exit(71)
	}
	if os.Getenv("API_TOKEN") != "segredo-ficticio-primeiro" {
		os.Exit(72)
	}
	os.Exit(0)
}

func TestExecRealRecebeSomenteCampoSelecionadoSemPIN(t *testing.T) {
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
	t.Setenv("KOFRE_PIN", "senha-fixture-123")
	t.Setenv("MYCOFRE_PIN", "segredo-reserva-nao-enviar")
	t.Setenv("KOFRE_FIXTURE_FILHO", "1")
	t.Setenv("OUTRO", "")
	t.Setenv("OUTRA_CONTA", "")
	if code := RunExec([]string{os.Args[0], "-test.run=^TestRunnerProcessoFilho$"}, "primeira", path, "API_TOKEN"); code != 0 {
		t.Fatalf("filho real retornou %d", code)
	}
}

func TestRunnerPersisteBloqueioEntreChamadas(t *testing.T) {
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
	t.Setenv("KOFRE_PIN", "senha-incorreta")
	for i := 0; i < 3; i++ {
		if aberto, err := UnlockVault(path); err == nil {
			aberto.Close()
			t.Fatal("senha incorreta abriu fixture")
		}
	}
	t.Setenv("KOFRE_PIN", "senha-fixture-123")
	if aberto, err := UnlockVault(path); err == nil {
		aberto.Close()
		t.Fatal("nova chamada ignorou bloqueio persistido")
	} else if !strings.Contains(err.Error(), "aguarde") {
		t.Fatal("retorno não identificou espera", err)
	}
}
