package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"kofre/internal/arquivo"
	"os"
	"sync"
	"time"
)

// SyncStorage conserva o último estado pendente até confirmar a gravação remota.
// Com LocalStorage, um marcador no disco permite retomar o envio após reinício.
type SyncStorage struct {
	local, remote    StorageProvider
	mu               sync.Mutex
	pending          []byte
	sequence         uint64
	syncing          bool
	lastErr, initErr error
	marker           string
	ctx              context.Context
	cancel           context.CancelFunc
	wake, changed    chan struct{}
	done             chan struct{}
	candidato        *cargaRemota
}

// Uma leitura remota ainda não é uma base editável: falta autenticar o conteúdo.
type cargaRemota struct {
	dados, anterior []byte
	tinhaLocal      bool
}

func NewSyncStorage(local, remote StorageProvider) *SyncStorage {
	ctx, cancel := context.WithCancel(context.Background())
	s := &SyncStorage{local: local, remote: remote, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), changed: make(chan struct{}, 1), done: make(chan struct{})}
	if l, ok := local.(*LocalStorage); ok && remote != nil {
		if cloud, ok := remote.(*KofreCloudStorage); ok {
			s.initErr = cloud.DefinirArquivoRevisao(l.filePath + ".cloud-revision.json")
		}
		s.marker = l.filePath + ".sync-pending.json"
		data, err := os.ReadFile(s.marker)
		if err == nil {
			var mark struct {
				Remote string `json:"remote"`
			}
			if err = json.Unmarshal(data, &mark); err != nil {
				s.initErr = fmt.Errorf("marcador de sincronização inválido: %w", err)
			} else if mark.Remote != syncDestination(remote) {
				s.initErr = errors.New("pendência pertence a outro destino; sincronize com o destino anterior antes de trocar")
			} else {
				s.pending, s.initErr = local.Load(ctx)
				s.sequence++
			}
		} else if !os.IsNotExist(err) {
			s.initErr = err
		}
	}
	go s.backgroundSyncWorker()
	if len(s.pending) > 0 {
		s.signal(s.wake)
	}
	return s
}
func (s *SyncStorage) signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
func (s *SyncStorage) Load(ctx context.Context) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initErr != nil {
		return nil, s.initErr
	}
	if s.candidato != nil {
		return bytes.Clone(s.candidato.dados), nil
	}
	data, err := s.local.Load(ctx)
	if err == nil {
		cloud, gerenciada := s.remote.(*KofreCloudStorage)
		_, arquivoLocal := s.local.(*LocalStorage)
		if gerenciada && arquivoLocal && s.pending == nil && cloud.coincideComBase(data) {
			timeout, cancel := context.WithTimeout(ctx, 3*time.Second)
			remoto, falha := cloud.Load(timeout)
			cancel()
			if falha == nil {
				s.candidato = &cargaRemota{dados: bytes.Clone(remoto), anterior: bytes.Clone(data), tinhaLocal: true}
				data = remoto
			}
			s.lastErr = falha // Offline conserva a cópia local e informa a pendência.
		}
		return data, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if s.remote == nil {
		return nil, err
	}
	data, err = s.remote.Load(ctx)
	if err != nil {
		return nil, err
	}
	s.candidato = &cargaRemota{dados: bytes.Clone(data)}
	return data, nil
}

// ConfirmarCarga instala somente o candidato que o chamador já decifrou e
// validou. Uma senha errada, formato inválido ou falha de autenticação nunca
// deve chegar aqui. Até esta etapa Load não altera o arquivo nem sua revisão.
func (s *SyncStorage) ConfirmarCarga(ctx context.Context, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initErr != nil {
		return s.initErr
	}
	if s.candidato == nil {
		return nil
	}
	candidato := s.candidato
	if !bytes.Equal(candidato.dados, data) {
		return errors.New("conteúdo validado diverge do candidato recebido")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var err error
	if local, ok := s.local.(*LocalStorage); ok {
		_, err = local.SubstituirSeIgual(ctx, candidato.anterior, candidato.tinhaLocal, data)
	} else {
		atual, e := s.local.Load(ctx)
		if e != nil && !errors.Is(e, ErrNotFound) {
			return e
		}
		if (e == nil) != candidato.tinhaLocal || !bytes.Equal(atual, candidato.anterior) {
			return fmt.Errorf("%w: cofre local mudou durante a validação; cópias preservadas", ErrConflito)
		}
		if !candidato.tinhaLocal || !bytes.Equal(atual, data) {
			err = s.local.Save(ctx, data)
		}
	}
	if err != nil {
		return err
	}
	// Atualiza a pré-condição para permitir repetir somente o ACK, se ele falhar.
	candidato.anterior, candidato.tinhaLocal = bytes.Clone(data), true
	if cloud, ok := s.remote.(*KofreCloudStorage); ok {
		if err = cloud.ConfirmarLeitura(data); err != nil {
			return fmt.Errorf("cofre autenticado salvo; confirmação da revisão pendente: %w", err)
		}
	}
	s.candidato = nil
	return nil
}
func (s *SyncStorage) Save(ctx context.Context, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initErr != nil {
		return s.initErr
	}
	if s.candidato != nil {
		return errors.New("valide e confirme o cofre recebido antes de salvar alterações")
	}
	if err := s.ctx.Err(); err != nil {
		return errors.New("sincronização encerrada")
	}
	if s.remote != nil && s.marker != "" {
		mark, _ := json.Marshal(map[string]string{"remote": syncDestination(s.remote)})
		if err := arquivo.Gravar(s.marker, mark, nil); err != nil {
			return fmt.Errorf("falha ao preservar pendência: %w", err)
		}
	}
	if err := s.local.Save(ctx, data); err != nil {
		return fmt.Errorf("falha ao salvar localmente: %w", err)
	}
	if s.remote != nil {
		s.pending = append([]byte(nil), data...)
		s.sequence++
		s.signal(s.wake)
	}
	return nil
}
func (s *SyncStorage) backgroundSyncWorker() {
	defer close(s.done)
	atraso := time.Second
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.wake:
		}
		for {
			s.mu.Lock()
			if s.pending == nil {
				s.mu.Unlock()
				break
			}
			data := append([]byte(nil), s.pending...)
			seq := s.sequence
			s.syncing = true
			s.mu.Unlock()
			ctx, cancel := context.WithTimeout(s.ctx, 20*time.Second)
			err := s.remote.Save(ctx, data)
			cancel()
			s.mu.Lock()
			s.syncing = false
			s.lastErr = err
			if err == nil && s.sequence == seq {
				if s.marker != "" {
					err = os.Remove(s.marker)
					if os.IsNotExist(err) {
						err = nil
					}
				}
				if err == nil {
					s.pending = nil
				} else {
					s.lastErr = fmt.Errorf("falha ao confirmar pendência: %w", err)
				}
			}
			more := s.pending != nil
			s.mu.Unlock()
			s.signal(s.changed)
			if errors.Is(err, ErrConflito) {
				break
			}
			if !more {
				break
			}
			if err != nil {
				timer := time.NewTimer(atraso)
				atraso = min(atraso*2, time.Minute)
				select {
				case <-s.ctx.Done():
					timer.Stop()
					return
				case <-s.wake:
					timer.Stop()
				case <-timer.C:
				}
			} else {
				atraso = time.Second
			}
		}
	}
}
func (s *SyncStorage) Flush(timeout time.Duration) error {
	if s.initErr != nil {
		return s.initErr
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	s.signal(s.wake)
	for {
		s.mu.Lock()
		busy := s.syncing || s.pending != nil
		err := s.lastErr
		s.mu.Unlock()
		if !busy {
			return nil
		}
		if errors.Is(err, ErrConflito) {
			return err
		}
		select {
		case <-s.changed:
		case <-ticker.C:
		case <-s.ctx.Done():
			return errors.New("sincronização encerrada com pendência")
		case <-timer.C:
			if err != nil {
				return fmt.Errorf("envio permanece pendente: %w", err)
			}
			return errors.New("tempo esgotado; envio permanece pendente")
		}
	}
}
func (s *SyncStorage) Close() { s.cancel(); <-s.done }

// Permite reabrir a sincronização após trocar a configuração de conta.
func (s *SyncStorage) Providers() (StorageProvider, StorageProvider) { return s.local, s.remote }

// Apenas observação do estado local do worker; não executa rede nem autoriza acesso.
func (s *SyncStorage) StatusSincronizacao() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initErr != nil {
		return true, s.initErr
	}
	return s.pending != nil || s.syncing || s.candidato != nil, s.lastErr
}

// Path identifica o arquivo local também quando a sincronização está ativa.
func (s *SyncStorage) Path() string {
	if local, ok := s.local.(interface{ Path() string }); ok {
		return local.Path()
	}
	return ""
}
func (s *SyncStorage) Exists(ctx context.Context) (bool, error) {
	if s.initErr != nil {
		return false, s.initErr
	}
	exists, err := s.local.Exists(ctx)
	if err != nil || exists {
		return exists, err
	}
	if s.remote != nil {
		return s.remote.Exists(ctx)
	}
	return false, nil
}
func (s *SyncStorage) Location() string {
	if s.remote != nil {
		return fmt.Sprintf("%s (sincronização: %s)", s.local.Location(), s.remote.Location())
	}
	return s.local.Location()
}
