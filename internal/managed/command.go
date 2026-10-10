package managed

import (
	"errors"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Install a versioned copy and shims inside the designated application home.
// No PATH, execution-policy, registry, shell-profile, or system changes.
func InstallCommand(home string) error {
	if strings.ContainsAny(home, "\r\n\x00\"%!") {
		return errors.New("command installation path contains unsupported shell characters")
	}
	source, e := os.Executable()
	if e != nil {
		return e
	}
	name := "xpotato-tunnel"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dest := filepath.Join(home, "core", core.Version, name)
	if !strings.EqualFold(filepath.Clean(source), filepath.Clean(dest)) {
		if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			return e
		}
		b, e := os.ReadFile(source)
		if e != nil {
			return e
		}
		if old, err := os.ReadFile(dest); err == nil {
			if hashBytes(old) != hashBytes(b) {
				return errors.New("same core version has different bytes; existing executable preserved")
			}
		} else if os.IsNotExist(err) {
			if e = os.WriteFile(dest, b, 0700); e != nil {
				return e
			}
		} else {
			return err
		}
	}
	bin := filepath.Join(home, "bin")
	if e = os.MkdirAll(bin, 0700); e != nil {
		return e
	}
	if runtime.GOOS == "windows" {
		ps := "& '" + strings.ReplaceAll(dest, "'", "''") + "' @args --home '" + strings.ReplaceAll(home, "'", "''") + "'\r\nexit $LASTEXITCODE\r\n"
		cmd := "@echo off\r\n\"" + dest + "\" %* --home \"" + home + "\"\r\nexit /b %errorlevel%\r\n"
		if e = os.WriteFile(filepath.Join(bin, "xpotato-tunnel.ps1"), []byte(ps), 0600); e != nil {
			return e
		}
		return os.WriteFile(filepath.Join(bin, "xpotato-tunnel.cmd"), []byte(cmd), 0600)
	}
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	return os.WriteFile(filepath.Join(bin, "xpotato-tunnel"), []byte("#!/bin/sh\nexec "+q(dest)+" \"$@\" --home "+q(home)+"\n"), 0700)
}
