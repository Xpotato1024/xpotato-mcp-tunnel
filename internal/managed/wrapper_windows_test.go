//go:build windows

package managed

import (
	"os/exec"
	"syscall"
)

func windowsWrapperCommand(wrapper string) *exec.Cmd {
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /d /s /c ""` + wrapper + `" -test.run=^TestGeneratedWrapperExecutesNativeArguments$ -- "two words""`}
	return cmd
}
