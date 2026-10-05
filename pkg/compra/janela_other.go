//go:build !windows

package compra

import "os/exec"

func configurarJanela(*exec.Cmd) {}
