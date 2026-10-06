package main

import "testing"

func TestMenuContaPreservaCofreAberto(t *testing.T) {
	anterior := vaultDaConta
	t.Cleanup(func() { vaultDaConta = anterior })
	vaultDaConta = "cofre-escolhido.enc"
	if resolveVaultPath("") != vaultDaConta {
		t.Fatal("menu voltou ao cofre padrão")
	}
	if resolveVaultPath("explicito.enc") != "explicito.enc" {
		t.Fatal("opção explícita perdeu prioridade")
	}
}
