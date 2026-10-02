//go:build linux

package crypto

import "golang.org/x/sys/unix"

func protectProcessOS() {
	// PR_SET_DUMPABLE = 0: Impede que outros processos do mesmo usuário façam ptrace/ReadProcessMemory
	// e impede que core dumps sejam gerados em disco no caso de falha fatal.
	_ = unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0)
}
