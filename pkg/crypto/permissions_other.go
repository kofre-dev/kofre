//go:build !windows

package crypto

import "os"

// RestrictFilePermissions restringe as permissoes do arquivo em sistemas Unix (chmod 0600)
func RestrictFilePermissions(path string) error {
	return os.Chmod(path, 0600)
}
