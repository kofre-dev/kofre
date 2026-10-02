package crypto

import (
	"errors"
	"sync"
)

// SealedBuffer mantém somente a representação cifrada entre chamadas a WithBytes.
// A função recebida não deve guardar o buffer, que é apagado ao retornar.
// O callback não pode reentrar em WithBytes/Close no mesmo SealedBuffer.
type SealedBuffer struct {
	mu     sync.Mutex
	data   []byte
	aux    []byte
	size   int
	closed bool
}

func SealMemory(plain []byte) (*SealedBuffer, error) {
	data, aux, err := sealMemory(plain)
	if err != nil {
		return nil, err
	}
	return &SealedBuffer{data: data, aux: aux, size: len(plain)}, nil
}

func (s *SealedBuffer) WithBytes(use func([]byte) error) error {
	if s == nil {
		return errors.New("segredo indisponivel")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("segredo encerrado")
	}
	plain, err := openMemory(s.data, s.aux)
	if err != nil {
		return err
	}
	defer ZeroBytes(plain)
	return use(plain[:s.size])
}

func (s *SealedBuffer) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ZeroBytes(s.data)
	ZeroBytes(s.aux)
	s.data, s.aux = nil, nil
	s.closed = true
}
