//go:build !windows

package crypto

import (
	"runtime"

	"golang.org/x/sys/unix"
)

// ProtectProcess blinda o processo contra ptrace (debuggers) e geração de coredump
func ProtectProcess() error {
	protectProcessOS()
	return nil // Os limites das outras plataformas são documentados separadamente.
}

// LockMemory trava o buffer na RAM física usando mlock(2), impedindo swap para o disco
func LockMemory(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	return unix.Mlock(b)
}

// UnlockMemory destrava o buffer da RAM física
func UnlockMemory(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	return unix.Munlock(b)
}

// WipeBytes sobrescreve um buffer sensível com zeros de forma imune a otimizações de compilação
func WipeBytes(b []byte) {
	if len(b) == 0 {
		return
	}
	for i := range b {
		b[i] = 0
	}
	runtime.KeepAlive(b)
}
