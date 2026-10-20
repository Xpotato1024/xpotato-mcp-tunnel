//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

func dispatchFixtureCMD(wrapper string) *exec.Cmd {
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /d /s /c ""` + wrapper + `" version "two words""`}
	return cmd
}
