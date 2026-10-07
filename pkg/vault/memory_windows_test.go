//go:build windows

package vault

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"golang.org/x/sys/windows"
	mycrypto "kofre/pkg/crypto"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"
)

// Este teste somente lê a memória do subprocesso de fixture que ele próprio cria.
// Não procura processos do usuário, não grava dumps e não utiliza o vault pessoal.
func TestProtectedVaultMemoryIsolation(t *testing.T) {
	seed := make([]byte, 24)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	canary := make([]byte, hex.EncodedLen(len(seed)))
	hex.Encode(canary, seed)
	key, salt := make([]byte, 32), make([]byte, 16)
	plain := append([]byte(`{"schema_version":1,"entries":[{"id":"fixture","title":"Fixture","fields":[{"name":"Senha","protected":true,"value":"`), canary...)
	plain = append(plain, []byte(`"}],"notes":"`)...)
	plain = append(plain, canary...)
	plain = append(plain, []byte(`","attachments":[{"filename":"ficticio.bin","data":"`)...)
	encodedCanary := make([]byte, base64.StdEncoding.EncodedLen(len(canary)))
	base64.StdEncoding.Encode(encodedCanary, canary)
	plain = append(plain, encodedCanary...)
	plain = append(plain, []byte(`","size":48}]}]}`)...)
	payload, err := mycrypto.Encrypt(plain, key)
	mycrypto.ZeroBytes(plain)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "fixture.enc")
	if err := os.WriteFile(file, append(append(append([]byte{}, MagicHeader...), salt...), payload...), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestProtectedVaultMemoryChild$")
	cmd.Env = append(os.Environ(), "KOFRE_MEMORY_FIXTURE="+file)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	scanner := bufio.NewScanner(stdout)
	ready := func() {
		t.Helper()
		if !scanner.Scan() || scanner.Text() != "ready" {
			t.Fatal("subprocesso de fixture nao ficou pronto")
		}
	}
	ready()
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	for _, stage := range []string{"loaded", "create", "update", "pack", "openV3", "close", "positive"} {
		if stage != "loaded" {
			fmt.Fprintln(stdin, stage)
			ready()
		}
		found, read, failures := scanFixtureMemory(t, handle, canary)
		t.Logf("etapa=%s bytes_lidos=%d falhas_leitura=%d ocorrencias=%d", stage, read, failures, found)
		if read == 0 || failures != 0 {
			t.Fatal("varredura incompleta")
		}
		if stage == "positive" {
			if found == 0 {
				t.Fatal("controle positivo nao detectado")
			}
		} else if found != 0 {
			t.Fatal("canario em texto simples no processo ocioso")
		}
	}
}

func scanFixtureMemory(t *testing.T, h windows.Handle, canary []byte) (found int, total uint64, failures int) {
	t.Helper()
	// UTF-8, UTF-16LE e []rune (UTF-32LE) usados pelos componentes do terminal.
	patterns := [][]byte{canary, make([]byte, len(canary)*2), make([]byte, len(canary)*4)}
	base64Canary := make([]byte, base64.StdEncoding.EncodedLen(len(canary)))
	base64.StdEncoding.Encode(base64Canary, canary)
	patterns = append(patterns, base64Canary)
	for i, c := range canary {
		patterns[1][2*i] = c
		patterns[2][4*i] = c
	}
	buffer := make([]byte, 1024*1024)
	const memPrivate = 0x20000
	for address := uintptr(0); ; {
		var info windows.MemoryBasicInformation
		if err := windows.VirtualQueryEx(h, address, &info, unsafe.Sizeof(info)); err != nil {
			break
		}
		next := info.BaseAddress + info.RegionSize
		if next <= address {
			break
		}
		if info.State == windows.MEM_COMMIT && info.Type == memPrivate && info.Protect&windows.PAGE_GUARD == 0 && info.Protect&0xff != windows.PAGE_NOACCESS && info.Protect&0xff != windows.PAGE_EXECUTE {
			for pos := info.BaseAddress; pos < next; {
				size := min(uintptr(len(buffer)), next-pos)
				var read uintptr
				if err := windows.ReadProcessMemory(h, pos, &buffer[0], size, &read); err != nil {
					failures++
				} else {
					total += uint64(read)
					for _, pattern := range patterns {
						found += bytes.Count(buffer[:read], pattern)
					}
				}
				if size < uintptr(len(buffer)) {
					break
				}
				pos += size - uintptr(len(patterns[2]))
			}
		}
		address = next
	}
	return
}

func TestProtectedVaultMemoryChild(t *testing.T) {
	file := os.Getenv("KOFRE_MEMORY_FIXTURE")
	if file == "" {
		t.Skip("helper do teste de memoria")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	salt, payload, err := UnpackHeader(raw)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	v, err := DecryptAndLoad(payload, key, salt)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	var positive []byte
	var packed []byte
	fmt.Println("ready")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		switch scanner.Text() {
		case "create":
			entry, _ := v.GetEntry("fixture")
			err = entry.Fields[0].WithValue(func(value []byte) error {
				field, e := NewProtectedField("Senha", value)
				if e != nil {
					return e
				}
				defer field.Close()
				created := SecretEntry{ID: "created", Title: "Criada", Fields: []Field{field}, Attachments: entry.Attachments}
				if e := entry.WithNotes(created.SetNotes); e != nil {
					return e
				}
				defer created.CloseNotes()
				_, e = v.AddEntry(created)
				return e
			})
			if err != nil {
				t.Fatal(err)
			}
		case "update":
			entry, e := v.GetEntry("created")
			if e != nil {
				t.Fatal(e)
			}
			entry.Title = "Editada"
			if err := v.UpdateEntry(entry); err != nil {
				t.Fatal(err)
			}
		case "pack":
			packed, err = v.Pack(key, salt)
			if err != nil {
				t.Fatal(err)
			}
		case "openV3":
			v.Close()
			v, err = DecryptAndLoad(packed, key, salt)
			if err != nil {
				t.Fatal(err)
			}
		case "close":
			v.Close()
		case "positive":
			// Controle positivo explícito: deixa o JSON decifrado vivo para provar
			// que OpenProcess/ReadProcessMemory conseguem observar esta fixture.
			positive, err = mycrypto.Decrypt(payload, key)
			if err != nil {
				t.Fatal(err)
			}
		}
		fmt.Println("ready")
	}
	runtime.KeepAlive(positive)
	mycrypto.ZeroBytes(positive)
}
