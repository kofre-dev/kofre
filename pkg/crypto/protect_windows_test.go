//go:build windows

package crypto

import (
	"os"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

func TestProtectProcessBlocksVMRead(t *testing.T) {
	ProtectProcess()
	// DACL não contém SeDebugPrivilege. Execute a verificação com uma cópia
	// do token da thread sem privilégios elevados, sem alterar o processo/runner.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.ImpersonateSelf(windows.SecurityImpersonation); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := windows.RevertToSelf(); err != nil {
			t.Fatal(err)
		}
	}()
	var token windows.Token
	if err := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY|windows.TOKEN_ADJUST_PRIVILEGES, true, &token); err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	if err := windows.AdjustTokenPrivileges(token, true, nil, 0, nil, nil); err != nil {
		t.Fatal(err)
	}

	pid := uint32(os.Getpid())
	// 0x410 = PROCESS_VM_READ | PROCESS_QUERY_INFORMATION (the attack vector used in Inspect-KofreMemory.ps1)
	const desiredAccess = 0x0010 | 0x0400
	h, err := windows.OpenProcess(desiredAccess, false, pid)
	if err == nil {
		windows.CloseHandle(h)
		t.Fatalf("esperava erro de acesso negado em OpenProcess(0x410), mas obteve handle valido")
	}

	if err != windows.ERROR_ACCESS_DENIED {
		t.Fatalf("OpenProcess retornou erro: %v (esperado ERROR_ACCESS_DENIED)", err)
	}
}
