//go:build windows

package crypto

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procSetNamedSecurityInfo = modadvapi32.NewProc("SetNamedSecurityInfoW")
	procGetSecDescDacl       = modadvapi32.NewProc("GetSecurityDescriptorDacl")
)

// RestrictFilePermissions restringe as permissoes do arquivo no Windows
// removendo qualquer heranca de diretorio e permitindo acesso apenas ao
// proprietario atual, administradores locais e SYSTEM.
func RestrictFilePermissions(path string) error {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}

	// SDDL:
	// D:P = Protected DACL (bloqueia herança de grupos como Sandbox/Everyone)
	// (A;;FA;;;OW) = Full Access para o Proprietário (Owner)
	// (A;;FA;;;BA) = Full Access para Administradores
	// (A;;FA;;;SY) = Full Access para NT AUTHORITY\SYSTEM
	sddlStr, err := windows.UTF16PtrFromString("D:P(A;;FA;;;OW)(A;;FA;;;BA)(A;;FA;;;SY)")
	if err != nil {
		return err
	}

	var pSd uintptr
	var sdSize uint32
	r1, _, errSys := procConvertStringSDToSD.Call(
		uintptr(unsafe.Pointer(sddlStr)),
		1, // SDDL_REVISION_1
		uintptr(unsafe.Pointer(&pSd)),
		uintptr(unsafe.Pointer(&sdSize)),
	)
	if r1 == 0 || pSd == 0 {
		return fmt.Errorf("ConvertStringSecurityDescriptor falhou: %w", errSys)
	}
	defer procLocalFree.Call(pSd)

	var daclPresent, daclDefaulted int32
	var pDacl uintptr
	r1, _, errSys = procGetSecDescDacl.Call(
		pSd,
		uintptr(unsafe.Pointer(&daclPresent)),
		uintptr(unsafe.Pointer(&pDacl)),
		uintptr(unsafe.Pointer(&daclDefaulted)),
	)
	if r1 == 0 {
		return fmt.Errorf("GetSecurityDescriptorDacl falhou: %w", errSys)
	}

	// SE_FILE_OBJECT = 1
	// DACL_SECURITY_INFORMATION (4) | PROTECTED_DACL_SECURITY_INFORMATION (0x80000000)
	const (
		seFileObject      = 1
		daclSecInfo       = 4
		protectedDaclInfo = 0x80000000
	)
	secInfo := uintptr(daclSecInfo | protectedDaclInfo)

	r1, _, _ = procSetNamedSecurityInfo.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		seFileObject,
		secInfo,
		0,
		0,
		pDacl,
		0,
	)
	if r1 != 0 {
		return fmt.Errorf("SetNamedSecurityInfo falhou com codigo %d", r1)
	}

	return nil
}
