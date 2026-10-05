//go:build windows

package storage

import (
	"context"
	"golang.org/x/sys/windows"
	"testing"
)

func TestSubstituicaoFalhaPreservaCofreWindows(t *testing.T) {
	l := localFixture(t)
	if err := l.Save(context.Background(), []byte("original")); err != nil {
		t.Fatal(err)
	}
	p, err := windows.UTF16PtrFromString(l.filePath)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.Save(context.Background(), []byte("novo")); err == nil {
		windows.CloseHandle(h)
		t.Fatal("esperava falha com destino aberto sem compartilhamento de exclusão")
	}
	windows.CloseHandle(h)
	got, err := l.Load(context.Background())
	if err != nil || string(got) != "original" {
		t.Fatal("falha de substituição perdeu o cofre anterior")
	}
}
