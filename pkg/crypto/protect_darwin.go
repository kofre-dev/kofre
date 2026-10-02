//go:build darwin

package crypto

import "golang.org/x/sys/unix"

func protectProcessOS() {
	// PT_DENY_ATTACH (31): Impede LLDB, DTrace e outros depuradores de anexar à memória do processo.
	const ptDenyAttach = 31
	_, _, _ = unix.Syscall(unix.SYS_PTRACE, uintptr(ptDenyAttach), 0, 0)
}
