//go:build !windows

package main

import "os/exec"

func dispatchFixtureCMD(wrapper string) *exec.Cmd {
	return exec.Command(wrapper, "version", "two words")
}
