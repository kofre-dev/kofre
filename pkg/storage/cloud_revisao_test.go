package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestCloudConflitoNaoAdotaBaseQueNaoFoiEditada(t *testing.T) {
	var mu sync.Mutex
	remoto := []byte("cofre remoto fictício A")
	puts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == "GET" {
			w.Header().Set("ETag", revisaoArquivoCloud(remoto))
			_, _ = w.Write(remoto)
			return
		}
		puts++
		if r.Header.Get("If-Match") != revisaoArquivoCloud(remoto) {
			http.Error(w, "conflito", 409)
			return
		}
		remoto, _ = io.ReadAll(r.Body)
		w.Header().Set("ETag", revisaoArquivoCloud(remoto))
	}))
	defer srv.Close()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "revisao.json")
	for n := 0; n < 2; n++ {
		c := NewKofreCloudStorage(srv.URL, "credencial fictícia")
		if err := c.DefinirArquivoRevisao(path); err != nil {
			t.Fatal(err)
		}
		if err := c.Save(ctx, []byte("cofre local diferente")); err == nil {
			t.Fatal("base desconhecida sobrescreveu remoto")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("sondagem gravou uma base que não foi editada")
		}
	}
	c := NewKofreCloudStorage(srv.URL, "credencial fictícia", "conta-estavel")
	if err := c.DefinirArquivoRevisao(path); err != nil {
		t.Fatal(err)
	}
	base, err := c.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.ConfirmarLeitura(base); err != nil {
		t.Fatal(err)
	}
	// Outro PC altera depois da leitura que serviu como base local.
	mu.Lock()
	remoto = []byte("cofre remoto fictício B")
	mu.Unlock()
	reaberto := NewKofreCloudStorage(srv.URL, "credencial recuperada", "conta-estavel")
	if err := reaberto.DefinirArquivoRevisao(path); err != nil {
		t.Fatal(err)
	}
	if err := reaberto.Save(ctx, []byte("edição sobre A")); err == nil {
		t.Fatal("reinício perdeu a revisão antiga")
	}
	mu.Lock()
	if puts != 1 || !bytes.Equal(remoto, []byte("cofre remoto fictício B")) {
		t.Error("conflito alterou o remoto")
	}
	mu.Unlock()
	base, err = reaberto.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = reaberto.ConfirmarLeitura(base); err != nil {
		t.Fatal(err)
	}
	if err := reaberto.Save(ctx, []byte("edição sobre B")); err != nil {
		t.Fatal(err)
	}
	outra := NewKofreCloudStorage(srv.URL, "outra credencial", "outra-conta")
	if err := outra.DefinirArquivoRevisao(path); err == nil {
		t.Fatal("revisão aceita em outra conta")
	}
}

func TestSyncBuscaNovaVersaoSemPerderBackupOuRessuscitarExclusao(t *testing.T) {
	var mu sync.Mutex
	remoto := []byte("cifra inicial fictícia")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == "GET" {
			if remoto == nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("ETag", revisaoArquivoCloud(remoto))
			w.Write(remoto)
			return
		}
		if remoto == nil && r.Header.Get("If-Match") != "" {
			http.Error(w, "excluído", 409)
			return
		}
		t.Error("escrita inesperada")
	}))
	defer srv.Close()
	ctx := context.Background()
	local, _ := NewLocalStorage(filepath.Join(t.TempDir(), "cofre.enc"))
	inicial := append([]byte(nil), remoto...)
	if err := local.Save(ctx, inicial); err != nil {
		t.Fatal(err)
	}
	cloud := NewKofreCloudStorage(srv.URL, "credencial fictícia")
	if err := cloud.DefinirArquivoRevisao(local.Path() + ".cloud-revision.json"); err != nil {
		t.Fatal(err)
	}
	if err := cloud.Save(ctx, inicial); err != nil {
		t.Fatal(err)
	}
	store := NewSyncStorage(local, cloud)
	defer store.Close()
	nova := []byte("cifra alterada no outro PC")
	mu.Lock()
	remoto = nova
	mu.Unlock()
	atual, err := store.Load(ctx)
	if err != nil || !bytes.Equal(atual, nova) {
		t.Fatal("não buscou versão remota", err)
	}
	backups, _ := filepath.Glob(local.Path() + ".backup-*.enc")
	if len(backups) != 1 {
		t.Fatal("não preservou backup", backups)
	}
	backup, _ := os.ReadFile(backups[0])
	if !bytes.Equal(backup, inicial) {
		t.Fatal("backup diferente da base local")
	}
	mu.Lock()
	remoto = nil
	mu.Unlock()
	if _, err := cloud.Load(ctx); err != ErrNotFound {
		t.Fatal("exclusão não detectada", err)
	}
	if err := cloud.Save(ctx, []byte("edição local sobre cópia excluída")); err == nil {
		t.Fatal("ressuscitou exclusão sem conciliar")
	}
}
