//go:build !windows && !linux && !darwin

package crypto

func protectProcessOS() {
	// No-op para outros sistemas Unix/BSD não mapeados
}
