//go:build windows

package compra

import (
	"os/exec"
	"syscall"
)

func configurarJanela(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
