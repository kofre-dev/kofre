//go:build windows

package crypto

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32       = windows.NewLazySystemDLL("kernel32.dll")
	modadvapi32       = windows.NewLazySystemDLL("advapi32.dll")
	procVirtualLock   = modkernel32.NewProc("VirtualLock")
	procVirtualUnlock = modkernel32.NewProc("VirtualUnlock")
	procLocalFree     = modkernel32.NewProc("LocalFree")

	procConvertStringSDToSD = modadvapi32.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")
	procSetKernelObjectSec  = modadvapi32.NewProc("SetKernelObjectSecurity")
)

// ProtectProcess blinda o processo Windows aplicando um DACL restritivo no kernel object
// que nega explicitamente PROCESS_VM_READ, PROCESS_VM_WRITE, PROCESS_VM_OPERATION,
// PROCESS_CREATE_THREAD, PROCESS_DUP_HANDLE e PROCESS_QUERY_INFORMATION (0x410)
// para outros processos locais (incluindo dumpers de memoria e anexadores de console).
// Não impede acesso com SeDebugPrivilege habilitado nem acesso pelo kernel.
func ProtectProcess() {
	// 1. Desativa dialogos de erro e crash dumps automaticos
	const semFlags = 0x0001 | 0x0002
	procSetErrorMode := modkernel32.NewProc("SetErrorMode")
	if procSetErrorMode.Find() == nil {
		_, _, _ = procSetErrorMode.Call(uintptr(semFlags))
	}

	// 2. Aplica DACL restritivo no processo atual:
	// Deny (WD - Everyone):
	//   PROCESS_CREATE_THREAD (0x0002)
	//   PROCESS_VM_OPERATION  (0x0008)
	//   PROCESS_VM_READ       (0x0010)
	//   PROCESS_VM_WRITE      (0x0020)
	//   PROCESS_DUP_HANDLE    (0x0040)
	//   PROCESS_QUERY_INFO    (0x0400)
	//   Total negado: 0x47A
	// Allow (WD - Everyone):
	//   PROCESS_TERMINATE     (0x0001)
	//   PROCESS_QUERY_LIMITED (0x1000)
	//   SYNCHRONIZE           (0x100000)
	//   Total permitido: 0x101001
	sddlStr, err := windows.UTF16PtrFromString("D:P(D;;0x47A;;;WD)(A;;0x101001;;;WD)")
	if err != nil {
		return
	}

	var pSd uintptr
	var sdSize uint32
	r1, _, _ := procConvertStringSDToSD.Call(
		uintptr(unsafe.Pointer(sddlStr)),
		1, // SDDL_REVISION_1
		uintptr(unsafe.Pointer(&pSd)),
		uintptr(unsafe.Pointer(&sdSize)),
	)
	if r1 != 0 && pSd != 0 {
		const daclSecInfo = 4 // DACL_SECURITY_INFORMATION
		currProc := windows.CurrentProcess()
		_, _, _ = procSetKernelObjectSec.Call(
			uintptr(currProc),
			uintptr(daclSecInfo),
			pSd,
		)
		_, _, _ = procLocalFree.Call(pSd)
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
