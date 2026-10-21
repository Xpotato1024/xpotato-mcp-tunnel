//go:build !windows

package processctrl

import "os/exec"

func HideWindow(*exec.Cmd) {}
