package main

import "testing"

func TestOpcoesRunnerExigemEscopoERecusamAmbiguidade(t *testing.T) {
	for _, args := range [][]string{
		{"--", "comando"}, {"--entry", "AWS", "--", "comando"},
		{"--entry", "AWS", "--fields", "", "--", "comando"},
		{"--entry", "AWS", "--fields", "TOKEN,", "--", "comando"},
		{"--entry", "AWS", "--only", "outra", "--fields", "TOKEN", "--", "comando"},
		{"--entry", "AWS", "--fields", "TOKEN", "--desconhecida", "--", "comando"},
	} {
		if _, err := lerOpcoesRunner(args, false); err == nil {
			t.Fatalf("opções inválidas aceitas: %q", args)
		}
	}
	opcoes, err := lerOpcoesRunner([]string{"--entry", "AWS", "--fields", "TOKEN, USUARIO", "--", "comando", "--flag"}, false)
	if err != nil || opcoes.entrada != "AWS" || len(opcoes.campos) != 2 || opcoes.campos[1] != "USUARIO" || len(opcoes.comando) != 2 {
		t.Fatal("argumentos válidos não preservados", err)
	}
	if _, err := lerOpcoesRunner([]string{"--entry", "AWS", "--fields", "TOKEN", "--ttl", "-1s"}, true); err == nil {
		t.Fatal("TTL negativo aceito")
	}
}
