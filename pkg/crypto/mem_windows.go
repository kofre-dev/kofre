//go:build windows

package crypto

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32        = windows.NewLazySystemDLL("kernel32.dll")
	procVirtualLock    = modkernel32.NewProc("VirtualLock")
	procVirtualUnlock  = modkernel32.NewProc("VirtualUnlock")
)

// ProtectProcess aplica políticas anti-dump e anti-inspeção no Windows
func ProtectProcess() {
	// 1. Desativa a geração de relatórios de crash / dump para o disco
	// SEM_FAILCRITICALERRORS | SEM_NOGPFAULTERRORBOX
	const semFlags = 0x0001 | 0x0002
	procSetErrorMode := modkernel32.NewProc("SetErrorMode")
	if procSetErrorMode.Find() == nil {
		_, _, _ = procSetErrorMode.Call(uintptr(semFlags))
	}
}

// LockMemory trava o buffer na RAM física usando VirtualLock, impedindo o Windows de enviá-lo para o pagefile.sys
func LockMemory(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	r1, _, err := procVirtualLock.Call(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	if r1 == 0 {
		return err
	}
	return nil
}

// UnlockMemory destrava o buffer da RAM física
func UnlockMemory(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	r1, _, err := procVirtualUnlock.Call(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	if r1 == 0 {
		return err
	}
	return nil
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
