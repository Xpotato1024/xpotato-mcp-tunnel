//go:build !linux && !darwin

package main

import (
	"context"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/processctrl"
	"os/exec"
	"time"
)

func dispatchProcess(ctx context.Context, cmd *exec.Cmd) (int, error) {
	return processctrl.RunContext(ctx, cmd, nil, 2*time.Second)
}
