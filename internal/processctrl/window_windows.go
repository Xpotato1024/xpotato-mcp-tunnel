package processctrl

import (
	"os/exec"
	"syscall"
)

// HideWindow affects only this subsequently spawned, owned child.
func HideWindow(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
