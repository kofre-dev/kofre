//go:build windows

package arquivo

import "golang.org/x/sys/windows"

func substituir(origem, destino string) error {
	o, err := windows.UTF16PtrFromString(origem)
	if err != nil {
		return err
	}
	d, err := windows.UTF16PtrFromString(destino)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(o, d, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
