//go:build linux

package main

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

func dispatchProcess(ctx context.Context, cmd *exec.Cmd) (int, error) {
	if e := ctx.Err(); e != nil {
		return 1, e
	}
	// No credentials or locks are held here. Exec preserves the terminal's
	// foreground process group for Setup/credentials and propagates exit/signals.
	return 1, syscall.Exec(cmd.Path, cmd.Args, os.Environ())
}
