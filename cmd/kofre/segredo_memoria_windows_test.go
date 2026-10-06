//go:build windows

package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	mycrypto "kofre/pkg/crypto"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"
)

func TestRevelacaoCorporativaSemResiduoImutavel(t *testing.T) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	canario := make([]byte, hex.EncodedLen(len(raw)))
	hex.Encode(canario, raw)
	cifra, err := mycrypto.Encrypt(canario, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "canario.enc")
	if err = os.WriteFile(path, cifra, 0600); err != nil {
		t.Fatal(err)
	}
	for _, modo := range []string{"antigo", "novo"} {
		t.Run(modo, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestRevelacaoMemoriaChild$")
			cmd.Env = append(os.Environ(), "KOFRE_REVELACAO_FIXTURE="+path, "KOFRE_REVELACAO_MODO="+modo)
			in, _ := cmd.StdinPipe()
			out, _ := cmd.StdoutPipe()
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { in.Close(); cmd.Process.Kill(); cmd.Wait() }()
			scanner := bufio.NewScanner(out)
			if !scanner.Scan() || scanner.Text() != "ready" {
				t.Fatal("fixture não ficou pronta")
			}
			h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, uint32(cmd.Process.Pid))
			if err != nil {
				t.Fatal(err)
			}
			defer windows.CloseHandle(h)
			found, n := buscarCanarioProcesso(t, h, canario)
			t.Logf("revelacao=%s bytes_lidos=%d ocorrencias=%d", modo, n, found)
			if modo == "antigo" && found == 0 {
				t.Fatal("instrumento não reproduziu o resíduo do fluxo antigo")
			}
			if modo == "novo" && found != 0 {
				t.Fatal("revelação reteve segredo em memória")
			}
			fmt.Fprintln(in, "positive")
			if !scanner.Scan() || scanner.Text() != "ready" {
				t.Fatal("controle positivo não ficou pronto")
			}
			if found, _ := buscarCanarioProcesso(t, h, canario); found == 0 {
				t.Fatal("controle positivo não detectado")
			}
		})
	}
}

func buscarCanarioProcesso(t *testing.T, h windows.Handle, canario []byte) (found int, total uint64) {
	t.Helper()
	patterns := [][]byte{canario, make([]byte, len(canario)*2), make([]byte, len(canario)*4)}
	for j, c := range canario {
		patterns[1][2*j] = c
		patterns[2][4*j] = c
	}
	buf := make([]byte, 1<<20)
	for addr := uintptr(0); ; {
		var info windows.MemoryBasicInformation
		if err := windows.VirtualQueryEx(h, addr, &info, unsafe.Sizeof(info)); err != nil {
			break
		}
		end := info.BaseAddress + info.RegionSize
		if end <= addr {
			break
		}
		if info.State == windows.MEM_COMMIT && info.Type == 0x20000 && info.Protect&windows.PAGE_GUARD == 0 && info.Protect&0xff != windows.PAGE_NOACCESS && info.Protect&0xff != windows.PAGE_EXECUTE {
			for p := info.BaseAddress; p < end; {
				size := min(uintptr(len(buf)), end-p)
				var read uintptr
				if err := windows.ReadProcessMemory(h, p, &buf[0], size, &read); err != nil {
					t.Fatal("leitura incompleta da fixture", err)
				}
				total += uint64(read)
				for _, pattern := range patterns {
					found += bytes.Count(buf[:read], pattern)
				}
				if size < uintptr(len(buf)) {
					break
				}
				p += size - uintptr(len(patterns[2]))
			}
		}
		addr = end
	}
	if total == 0 {
		t.Fatal("nenhuma região foi examinada")
	}
	return
}

func TestRevelacaoMemoriaChild(t *testing.T) {
	file := os.Getenv("KOFRE_REVELACAO_FIXTURE")
	if file == "" {
		t.Skip("fixture exclusiva")
	}
	cifra, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	mostrar := func() {
		data, err := mycrypto.Decrypt(cifra, make([]byte, 32))
		if err != nil {
			t.Fatal(err)
		}
		defer mycrypto.ZeroBytes(data)
		if os.Getenv("KOFRE_REVELACAO_MODO") == "antigo" {
			fmt.Fprintln(io.Discard, textoEmpresa(string(data)))
		} else if err := escreverSegredoPara(io.Discard, data); err != nil {
			t.Fatal(err)
		}
	}
	mostrar()
	fmt.Println("ready")
	scanner := bufio.NewScanner(os.Stdin)
	var positivo []byte
	for scanner.Scan() {
		if scanner.Text() == "positive" {
			positivo, err = mycrypto.Decrypt(cifra, make([]byte, 32))
			if err != nil {
				t.Fatal(err)
			}
			fmt.Println("ready")
		}
	}
	runtime.KeepAlive(positivo)
	mycrypto.ZeroBytes(positivo)
}
