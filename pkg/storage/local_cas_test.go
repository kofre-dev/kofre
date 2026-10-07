package storage

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"kofre/internal/arquivo"
)

func TestLocalCASDuasInstanciasNaoPerdemAtualizacao(t *testing.T) {
	ctx := context.Background()
	primeiro := localFixture(t)
	segundo, err := NewLocalStorage(primeiro.Path())
	if err != nil {
		t.Fatal(err)
	}
	base := []byte("base cifrada fictícia")
	if err := primeiro.Save(ctx, base); err != nil {
		t.Fatal(err)
	}
	inicio := make(chan struct{})
	resultados := make(chan error, 2)
	var wg sync.WaitGroup
	for i, local := range []*LocalStorage{primeiro, segundo} {
		wg.Add(1)
		go func(i int, local *LocalStorage) {
			defer wg.Done()
			<-inicio
			_, err := local.SubstituirSeIgual(ctx, base, true, []byte{byte('A' + i)})
			resultados <- err
		}(i, local)
	}
	close(inicio)
	wg.Wait()
	close(resultados)
	sucessos, conflitos := 0, 0
	for err := range resultados {
		if err == nil {
			sucessos++
		} else if errors.Is(err, ErrConflito) {
			conflitos++
		} else {
			t.Fatal(err)
		}
	}
	if sucessos != 1 || conflitos != 1 {
		t.Fatalf("sucessos=%d conflitos=%d; esperado um de cada", sucessos, conflitos)
	}
	backups, _ := filepath.Glob(primeiro.Path() + ".backup-*.enc")
	if len(backups) != 1 {
		t.Fatal("CAS perdedor gerou backup", backups)
	}
	backup, _ := os.ReadFile(backups[0])
	if !bytes.Equal(backup, base) {
		t.Fatal("backup da base original não foi preservado")
	}
}

func TestProcessoFixtureSeguraLockDeEscrita(t *testing.T) {
	path := os.Getenv("KOFRE_FIXTURE_LOCK_PATH")
	if path == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := arquivo.ComExclusao(ctx, path+".escrita.lock", func() error {
		if err := os.WriteFile(path+".ready", []byte("pronto"), 0600); err != nil {
			return err
		}
		for {
			if _, err := os.Stat(path + ".release"); err == nil {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
		}
		return arquivo.Gravar(path, []byte("edição de outro processo"), nil)
	})
	if err != nil {
		os.Exit(81)
	}
	os.Exit(0)
}

func TestEscritoresRespeitamLockDeOutroProcessoERevalidamBase(t *testing.T) {
	ctx := context.Background()
	local := localFixture(t)
	base := []byte("base anterior")
	if err := local.Save(ctx, base); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestProcessoFixtureSeguraLockDeEscrita$")
	cmd.Env = append(os.Environ(), "KOFRE_FIXTURE_LOCK_PATH="+local.Path())
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.WriteFile(local.Path()+".release", []byte("sair"), 0600); _ = cmd.Process.Kill() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(local.Path() + ".ready"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("processo não obteve lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for nome, executar := range map[string]func(context.Context) error{
		"Save":            func(c context.Context) error { return local.Save(c, []byte("descartar")) },
		"SaveComBackup":   func(c context.Context) error { _, e := local.SaveComBackup(c, []byte("descartar")); return e },
		"PreservarBackup": func(c context.Context) error { _, e := local.PreservarBackup(c, []byte("descartar")); return e },
	} {
		c, cancel := context.WithTimeout(ctx, 40*time.Millisecond)
		err := executar(c)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("%s ignorou lock de outro processo: %v", nome, err)
		}
	}
	resultado := make(chan error, 1)
	go func() {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_, err := local.SubstituirSeIgual(c, base, true, []byte("candidato remoto validado"))
		resultado <- err
	}()
	select {
	case err := <-resultado:
		t.Fatalf("CAS passou antes do fim da escrita concorrente: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
	if err := os.WriteFile(local.Path()+".release", []byte("continuar"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal("processo fixture falhou", err)
	}
	if err := <-resultado; !errors.Is(err, ErrConflito) {
		t.Fatal("CAS não conferiu base após obter lock", err)
	}
	atual, err := local.Load(ctx)
	if err != nil || string(atual) != "edição de outro processo" {
		t.Fatal("edição concorrente foi perdida", err)
	}
	backups, _ := filepath.Glob(local.Path() + ".backup-*.enc")
	if len(backups) != 0 {
		t.Fatal("escrita recusada deixou backup inesperado")
	}
}
