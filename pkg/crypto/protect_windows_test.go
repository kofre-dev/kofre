//go:build windows

package crypto

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestProtectProcessBlocksVMRead(t *testing.T) {
	ProtectProcess()

	pid := uint32(os.Getpid())
	// 0x410 = PROCESS_VM_READ | PROCESS_QUERY_INFORMATION (the attack vector used in Inspect-KofreMemory.ps1)
	const desiredAccess = 0x0010 | 0x0400
	h, err := windows.OpenProcess(desiredAccess, false, pid)
	if err == nil {
		windows.CloseHandle(h)
		t.Fatalf("esperava erro de acesso negado em OpenProcess(0x410), mas obteve handle valido")
	}

	if err != windows.ERROR_ACCESS_DENIED {
		t.Logf("OpenProcess retornou erro: %v (esperado ERROR_ACCESS_DENIED)", err)
	}
}
