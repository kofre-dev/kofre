//go:build !windows

package arquivo

import "os"

func substituir(origem, destino string) error { return os.Rename(origem, destino) }
