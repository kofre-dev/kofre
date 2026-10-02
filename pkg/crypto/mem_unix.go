//go:build !windows

package crypto

import (
	"runtime"

	"golang.org/x/sys/unix"
)

// ProtectProcess blinda o processo contra ptrace (debuggers) e geração de coredump
func ProtectProcess() {
	switch runtime.GOOS {
	case "linux":
		// PR_SET_DUMPABLE = 0: Impede que outros processos do mesmo usuário façam ptrace/ReadProcessMemory
		// e impede que core dumps sejam gerados em disco no caso de falha fatal.
		_ = unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0)

	case "darwin":
		// PT_DENY_ATTACH (31): Impede LLDB, DTrace e outros depuradores de anexar à memória do processo.
		const ptDenyAttach = 31
		_, _, _ = unix.Syscall(unix.SYS_PTRACE, uintptr(ptDenyAttach), 0, 0)
	}
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
