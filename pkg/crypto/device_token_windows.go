//go:build windows

package crypto

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	modcrypt32             = syscall.NewLazyDLL("crypt32.dll")
	procCryptProtectData   = modcrypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = modcrypt32.NewProc("CryptUnprotectData")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func newBlob(d []byte) *dataBlob {
	if len(d) == 0 {
		return &dataBlob{}
	}
	return &dataBlob{
		cbData: uint32(len(d)),
		pbData: &d[0],
	}
}

func (b *dataBlob) toByteArray() []byte {
	d := make([]byte, b.cbData)
	copy(d, unsafe.Slice(b.pbData, b.cbData))
	return d
}

// EncryptWithDPAPI criptografa dados sensíveis vinculando-os ao hardware e usuário do Windows
func EncryptWithDPAPI(data []byte) ([]byte, error) {
	inBlob := newBlob(data)
	var outBlob dataBlob

	r, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(inBlob)),
		0,
		0,
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CryptProtectData falhou: %w", err)
	}

	result := outBlob.toByteArray()
	// Libera memória alocada pelo Windows
	_, _, _ = syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree").Call(uintptr(unsafe.Pointer(outBlob.pbData)))
	return result, nil
}

// DecryptWithDPAPI decifra dados protegidos pelo DPAPI do Windows
func DecryptWithDPAPI(encrypted []byte) ([]byte, error) {
	inBlob := newBlob(encrypted)
	var outBlob dataBlob

	r, _, err := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(inBlob)),
		0,
		0,
		0,
		0,
		0,
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CryptUnprotectData falhou: chave não pertence a este hardware ou usuário: %w", err)
	}

	result := outBlob.toByteArray()
	_, _, _ = syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree").Call(uintptr(unsafe.Pointer(outBlob.pbData)))
	return result, nil
}

func getDeviceKeyPath() (string, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = os.TempDir()
	}
	dir := filepath.Join(appData, "Kofre")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "device_key.bin"), nil
}

// SaveDeviceMasterKey persiste a chave mestra criptografada com DPAPI no computador local
func SaveDeviceMasterKey(key []byte) error {
	path, err := getDeviceKeyPath()
	if err != nil {
		return err
	}

	protected, err := EncryptWithDPAPI(key)
	if err != nil {
		return err
	}

	return os.WriteFile(path, protected, 0600)
}

// LoadDeviceMasterKey recupera a chave mestra utilizando o DPAPI do computador local
func LoadDeviceMasterKey() ([]byte, error) {
	path, err := getDeviceKeyPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return DecryptWithDPAPI(data)
}

// HasDeviceMasterKey verifica se há uma chave vinculada a este hardware
func HasDeviceMasterKey() bool {
	key, err := LoadDeviceMasterKey()
	if err == nil && len(key) == 32 {
		ZeroBytes(key)
		return true
	}
	return false
}

// GenerateRandomOTP gera um código de 6 dígitos numéricos
func GenerateRandomOTP() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	num := (int(b[0])<<16 | int(b[1])<<8 | int(b[2])) % 900000 + 100000
	return fmt.Sprintf("%06d", num)
}
