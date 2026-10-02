//go:build windows

package crypto

import (
	"fmt"
	"golang.org/x/sys/windows"
	"runtime"
	"unsafe"
)

var memoryCrypt = windows.NewLazySystemDLL("crypt32.dll")
var protectMemory = memoryCrypt.NewProc("CryptProtectMemory")
var unprotectMemory = memoryCrypt.NewProc("CryptUnprotectMemory")

func transformMemory(data []byte, proc *windows.LazyProc) error {
	// CRYPTPROTECTMEMORY_SAME_PROCESS = 0; tamanho múltiplo de 16.
	result, _, err := proc.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 0)
	runtime.KeepAlive(data)
	if result == 0 {
		return fmt.Errorf("protecao de memoria: %w", err)
	}
	return nil
}

func sealMemory(plain []byte) ([]byte, []byte, error) {
	data := make([]byte, (len(plain)/16+1)*16)
	copy(data, plain)
	if err := transformMemory(data, protectMemory); err != nil {
		ZeroBytes(data)
		return nil, nil, err
	}
	return data, nil, nil
}

func openMemory(data, _ []byte) ([]byte, error) {
	plain := append([]byte(nil), data...)
	if err := transformMemory(plain, unprotectMemory); err != nil {
		ZeroBytes(plain)
		return nil, err
	}
	return plain, nil
}
