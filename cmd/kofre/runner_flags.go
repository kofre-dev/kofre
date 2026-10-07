package main

import (
	"errors"
	"flag"
	"io"
	"strings"
	"time"

	"kofre/pkg/runner"
)

type opcoesRunner struct {
	entrada, vault  string
	campos, comando []string
	ttl             time.Duration
}

func lerOpcoesRunner(args []string, shell bool) (opcoesRunner, error) {
	var opcoes opcoesRunner
	var campos, alias string
	fs := flag.NewFlagSet("exec/shell", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opcoes.entrada, "entry", "", "ID ou título exato")
	fs.StringVar(&alias, "only", "", "alias de --entry")
	fs.StringVar(&campos, "fields", "", "campos exatos separados por vírgula")
	fs.StringVar(&opcoes.vault, "vault", "", "arquivo do cofre")
	if shell {
		fs.DurationVar(&opcoes.ttl, "ttl", 0, "duração do shell")
	}
	if err := fs.Parse(args); err != nil {
		return opcoes, err
	}
	if opcoes.entrada != "" && alias != "" {
		return opcoes, errors.New("use somente --entry ou seu alias --only")
	}
	if alias != "" {
		opcoes.entrada = alias
	}
	if campos != "" {
		opcoes.campos = strings.Split(campos, ",")
		for i := range opcoes.campos {
			opcoes.campos[i] = strings.TrimSpace(opcoes.campos[i])
		}
	}
	if err := runner.ValidarSelecao(opcoes.entrada, opcoes.campos); err != nil {
		return opcoes, err
	}
	opcoes.comando = fs.Args()
	if shell && (len(opcoes.comando) > 0 || opcoes.ttl < 0) {
		return opcoes, errors.New("shell não aceita comando posicional nem prazo negativo")
	}
	if !shell && len(opcoes.comando) == 0 {
		return opcoes, errors.New("informe o comando após --")
	}
	return opcoes, nil
}
