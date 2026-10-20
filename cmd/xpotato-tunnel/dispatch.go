package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/managed"
)

func dispatchArguments(args []string) (string, []string, error) {
	// Thin wrappers append the bound home last; earlier user arguments cannot
	// redirect the launcher to some other installation.
	if len(args) < 2 || args[len(args)-2] != "--home" || !filepath.IsAbs(args[len(args)-1]) {
		return "", nil, errors.New("installed command home argument missing")
	}
	return args[len(args)-1], args, nil
}

func dispatch(ctx context.Context, args []string) (int, error) {
	if e := ctx.Err(); e != nil {
		return 1, e
	}
	home, args, e := dispatchArguments(args)
	if e != nil {
		return 1, e
	}
	command, o, e := parse(args)
	if e != nil {
		return 1, e
	}
	var target string
	if command == "start" || command == "version" {
		device := o.Device
		if device == "" {
			if resolved, err := core.Resolve(core.Options{Home: home}); err == nil {
				device = resolved.Device
			}
		}
		target, e = managed.DispatchTarget(ctx, home, device)
	} else {
		target, e = managed.ManagementTarget(ctx, home)
	}
	if e != nil {
		return 1, e
	}
	cmd := exec.Command(target, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return dispatchProcess(ctx, cmd)
}
