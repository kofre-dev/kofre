package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type remoteFixture struct {
	mu             sync.Mutex
	data           []byte
	fail           bool
	calls          int
	block, started chan struct{}
}

func (r *remoteFixture) Save(ctx context.Context, data []byte) error {
	r.mu.Lock()
	r.calls++
	fail, block, started := r.fail, r.block, r.started
	r.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if fail {
		return errors.New("falha fictícia")
	}
	r.mu.Lock()
	r.data = append([]byte(nil), data...)
	r.mu.Unlock()
	return nil
}
func (r *remoteFixture) Load(context.Context) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.data == nil {
		return nil, ErrNotFound
	}
	return append([]byte(nil), r.data...), nil
}
func (r *remoteFixture) Exists(ctx context.Context) (bool, error) {
	_, err := r.Load(ctx)
	return err == nil, nil
}
func (*remoteFixture) Location() string { return "remoto fictício" }
func localFixture(t *testing.T) *LocalStorage {
	t.Helper()
	l, err := NewLocalStorage(filepath.Join(t.TempDir(), "vault.enc"))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestSyncFalhaPreservaPendenciaERecuperaAposReinicio(t *testing.T) {
	l := localFixture(t)
	r := &remoteFixture{fail: true}
	s := NewSyncStorage(l, r)
	if err := s.Save(context.Background(), []byte("último estado cifrado fictício")); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(60 * time.Millisecond); err == nil {
		t.Fatal("Flush escondeu falha remota")
	}
	s.Close()
	if _, err := os.Stat(l.filePath + ".sync-pending.json"); err != nil {
		t.Fatal("pendência não foi persistida", err)
	}
	r.mu.Lock()
	r.fail = false
	r.mu.Unlock()
	resumed := NewSyncStorage(l, r)
	defer resumed.Close()
	if err := resumed.Flush(time.Second); err != nil {
		t.Fatal(err)
	}
	remote, _ := r.Load(context.Background())
	local, _ := l.Load(context.Background())
	if !bytes.Equal(remote, local) {
		t.Fatal("retomada perdeu último estado")
	}
	if _, err := os.Stat(l.filePath + ".sync-pending.json"); !os.IsNotExist(err) {
		t.Fatal("confirmação não removeu marcador")
	}
}
func TestSyncNovaVersaoNaoPerdidaDuranteUploadAnterior(t *testing.T) {
	l := localFixture(t)
	r := &remoteFixture{block: make(chan struct{}), started: make(chan struct{}, 1)}
	s := NewSyncStorage(l, r)
	defer s.Close()
	if err := s.Save(context.Background(), []byte("anterior")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.started:
	case <-time.After(time.Second):
		t.Fatal("worker não iniciou")
	}
	if err := s.Save(context.Background(), []byte("nova")); err != nil {
		t.Fatal(err)
	}
	close(r.block)
	if err := s.Flush(time.Second); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Load(context.Background())
	if string(got) != "nova" {
		t.Fatal("upload antigo sobrescreveu o último estado")
	}
}
func TestStressSyncConcorrenteConvergeComDisco(t *testing.T) {
	l := localFixture(t)
	r := &remoteFixture{}
	s := NewSyncStorage(l, r)
	defer s.Close()
	var wg sync.WaitGroup
	for worker := range 16 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for n := range 64 {
				if err := s.Save(context.Background(), []byte(fmt.Sprintf("estado %d/%d", worker, n))); err != nil {
					t.Error(err)
				}
			}
		}(worker)
	}
	wg.Wait()
	if err := s.Flush(5 * time.Second); err != nil {
		t.Fatal(err)
	}
	remote, _ := r.Load(context.Background())
	local, _ := l.Load(context.Background())
	if !bytes.Equal(remote, local) {
		t.Fatal("remoto diverge do último estado no disco")
	}
	t.Log("1.024 gravações concorrentes; estado final local e remoto idêntico")
}
func TestSyncDestinoAlteradoNaoEnviaPendencia(t *testing.T) {
	l := localFixture(t)
	if err := l.Save(context.Background(), []byte("fictício")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.filePath+".sync-pending.json", []byte(`{"remote":"outro destino"}`), 0600); err != nil {
		t.Fatal(err)
	}
	r := &remoteFixture{}
	s := NewSyncStorage(l, r)
	defer s.Close()
	if _, err := s.Load(context.Background()); err == nil {
		t.Fatal("aceitou marcador de outro destino")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.calls != 0 {
		t.Fatal("enviou cofre para destino alterado")
	}
}

func TestSyncTrocaDeContaMesmoEndpointNaoEnviaPendencia(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		http.Error(w, "offline fictício", 503)
	}))
	defer endpoint.Close()
	local := localFixture(t)
	first := NewSyncStorage(local, NewKofreCloudStorage(endpoint.URL, "kfr_conta_a_ficticia"))
	if err := first.Save(context.Background(), []byte("estado da conta A")); err != nil {
		t.Fatal(err)
	}
	if err := first.Flush(50 * time.Millisecond); err == nil {
		t.Fatal("ignorou erro remoto")
	}
	first.Close()
	mu.Lock()
	before := calls
	mu.Unlock()
	second := NewSyncStorage(local, NewKofreCloudStorage(endpoint.URL, "kfr_conta_b_ficticia"))
	defer second.Close()
	if _, err := second.Load(context.Background()); err == nil {
		t.Fatal("aceitou pendência da conta anterior")
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != before {
		t.Fatal("enviou conteúdo de A para B")
	}
}
