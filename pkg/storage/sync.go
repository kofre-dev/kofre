package storage

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// SyncStorage combina o armazenamento local com a nuvem (Kofre Cloud ou S3 próprio).
// Ele garante um design offline-first ultra rápido:
// 1. Qualquer criação, edição ou exclusão de credencial é gravada localmente de forma atômica e instantânea (< 1ms).
// 2. A sincronização com a nuvem (S3/Railway) roda em segundo plano (background worker assíncrono).
// 3. Modificações em sequência são automaticamente agrupadas (coalescing), evitando requisições HTTP desnecessárias.
// 4. Ao sair da aplicação, o Flush garante que a última versão pendente seja enviada.
type SyncStorage struct {
	local     StorageProvider
	remote    StorageProvider
	mu        sync.Mutex
	pending   []byte
	isSyncing bool
}

// NewSyncStorage cria uma instância que sincroniza local e remoto
func NewSyncStorage(local, remote StorageProvider) *SyncStorage {
	return &SyncStorage{
		local:  local,
		remote: remote,
	}
}

// Load carrega os dados locais se existirem; se não existirem, busca na nuvem e grava local
func (s *SyncStorage) Load(ctx context.Context) ([]byte, error) {
	localExists, _ := s.local.Exists(ctx)
	if localExists {
		return s.local.Load(ctx)
	}

	// Não existe localmente: verifica se existe na nuvem (ex: setup em máquina nova)
	if s.remote != nil {
		remoteExists, errRemote := s.remote.Exists(ctx)
		if errRemote == nil && remoteExists {
			data, err := s.remote.Load(ctx)
			if err == nil && len(data) >= 60 {
				// Salva automaticamente a cópia local para uso offline
				_ = s.local.Save(ctx, data)
				return data, nil
			}
		}
	}

	return s.local.Load(ctx)
}

// Save grava primeiro localmente e despacha a sincronização com a nuvem em segundo plano
func (s *SyncStorage) Save(ctx context.Context, data []byte) error {
	// 1. Grava no disco local (atomic, instantâneo, offline-first)
	if err := s.local.Save(ctx, data); err != nil {
		return fmt.Errorf("falha ao salvar localmente: %w", err)
	}

	// 2. Se houver nuvem configurada, agenda sincronização assíncrona em segundo plano
	if s.remote != nil {
		s.mu.Lock()
		// Cria cópia segura dos bytes
		cp := make([]byte, len(data))
		copy(cp, data)
		s.pending = cp

		if !s.isSyncing {
			s.isSyncing = true
			go s.backgroundSyncWorker()
		}
		s.mu.Unlock()
	}

	return nil
}

// backgroundSyncWorker processa a sincronização em lote/segundo plano sem travar a interface
func (s *SyncStorage) backgroundSyncWorker() {
	for {
		s.mu.Lock()
		if len(s.pending) == 0 {
			s.isSyncing = false
			s.mu.Unlock()
			return
		}
		dataToSync := s.pending
		s.pending = nil
		s.mu.Unlock()

		syncCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		if err := s.remote.Save(syncCtx, dataToSync); err != nil {
			log.Printf("[Sync em Segundo Plano] Cofre salvo localmente, mas sincronização em nuvem falhou: %v", err)
		}
		cancel()
	}
}

// Flush aguarda a sincronização pendente finalizar (útil ao fechar a aplicação)
func (s *SyncStorage) Flush(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for {
		s.mu.Lock()
		busy := s.isSyncing || len(s.pending) > 0
		s.mu.Unlock()

		if !busy || time.Now().After(deadline) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Exists verifica existência local ou remota
func (s *SyncStorage) Exists(ctx context.Context) (bool, error) {
	localExists, err := s.local.Exists(ctx)
	if err == nil && localExists {
		return true, nil
	}
	if s.remote != nil {
		return s.remote.Exists(ctx)
	}
	return localExists, err
}

// Location retorna uma descrição clara da persistência
func (s *SyncStorage) Location() string {
	if s.remote != nil {
		return fmt.Sprintf("%s (Auto-Sync Assíncrono: %s)", s.local.Location(), s.remote.Location())
	}
	return s.local.Location()
}
