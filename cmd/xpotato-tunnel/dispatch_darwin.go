package main

import (
	"context"
	"os/exec"
	"syscall"
)

// Replace only this verified dispatcher, keeping its foreground terminal and
// process identity. The selected core owns its runtime/guardian afterwards.
func dispatchProcess(ctx context.Context, cmd *exec.Cmd) (int, error) {
	if err := ctx.Err(); err != nil {
		return 1, err
	}
	environment := cmd.Env
	if environment == nil {
		environment = cmd.Environ()
	}
	return 1, syscall.Exec(cmd.Path, cmd.Args, environment)
}
