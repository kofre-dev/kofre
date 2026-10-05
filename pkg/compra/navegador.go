package compra

import (
	"errors"
	"net/url"
	"os/exec"
	"runtime"
)

func AbrirNavegador(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "kofre.dev" || u.User != nil || u.Path != "/comprar" {
		return errors.New("endereço de compra inválido")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", raw)
	case "darwin":
		cmd = exec.Command("open", raw)
	default:
		cmd = exec.Command("xdg-open", raw)
	}
	configurarJanela(cmd)
	if err = cmd.Start(); err != nil {
		return errors.New("não foi possível abrir o navegador padrão")
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
