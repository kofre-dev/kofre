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

// PurgeLegacyDeviceKey localiza e remove imediatamente com sobrescrita de zeros qualquer resquício de device_key.bin
func PurgeLegacyDeviceKey() {
	path, err := getDeviceKeyPath()
	if err != nil {
		return
	}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		zeroData := make([]byte, info.Size())
		_ = os.WriteFile(path, zeroData, 0600)
		_ = os.Remove(path)
	}
}

func GenerateRandomOTP() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	num := (int(b[0])<<16 | int(b[1])<<8 | int(b[2])) % 900000 + 100000
	return fmt.Sprintf("%06d", num)
}
