//go:build !windows

package managed

import "os/exec"

func windowsWrapperCommand(wrapper string) *exec.Cmd { return exec.Command(wrapper) }
