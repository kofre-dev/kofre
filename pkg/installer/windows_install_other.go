//go:build !windows

package installer

import "fmt"

func RegisterInstallation(binaryPath, version string) error { return nil }
func RefreshRegistration(version string) error              { return nil }
func updateWindowsPath(dir string, add bool) error          { return nil }
func Uninstall() error                                      { return fmt.Errorf("desinstalacao automatica disponivel apenas no Windows") }
