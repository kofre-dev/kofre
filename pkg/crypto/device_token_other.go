//go:build !windows

package crypto

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
)

func getDeviceKeyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}
	dir := filepath.Join(home, ".config", "kofre")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "device_key.bin"), nil
}

// SaveDeviceMasterKey em plataformas não-Windows (fallback seguro)
func SaveDeviceMasterKey(key []byte) error {
	path, err := getDeviceKeyPath()
	if err != nil {
		return err
	}
	return os.WriteFile(path, key, 0600)
}

// LoadDeviceMasterKey em plataformas não-Windows
func LoadDeviceMasterKey() ([]byte, error) {
	path, err := getDeviceKeyPath()
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func HasDeviceMasterKey() bool {
	key, err := LoadDeviceMasterKey()
	if err == nil && len(key) == 32 {
		ZeroBytes(key)
		return true
	}
	return false
}

func GenerateRandomOTP() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	num := (int(b[0])<<16 | int(b[1])<<8 | int(b[2])) % 900000 + 100000
	return fmt.Sprintf("%06d", num)
}
