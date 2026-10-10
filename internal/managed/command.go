package managed

import (
	"encoding/json"
	"errors"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/lockfile"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type commandRecord struct {
	Owner, Version, Directory string
	Decided                   bool
	Files                     map[string]string
}

func readCommandRecord(home string) (commandRecord, error) {
	var r commandRecord
	b, e := os.ReadFile(filepath.Join(home, "command-install.json"))
	if os.IsNotExist(e) {
		return r, nil
	}
	if e != nil {
		return r, e
	}
	if json.Unmarshal(b, &r) != nil {
		return r, errors.New("command install record invalid")
	}
	i, e := platform.CurrentIdentity()
	if e != nil || r.Owner != i.Owner {
		return r, errors.New("command installation owner not verified")
	}
	return r, nil
}
func saveCommandRecord(file string, r commandRecord) error {
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	return atomicBytes(file, append(b, '\n'))
}
func validateCommandPath(p string) error {
	if !filepath.IsAbs(p) || strings.ContainsAny(p, "\r\n\x00\"%!") {
		return errors.New("command directory must be an absolute path without unsupported shell characters")
	}
	return nil
}
func shimFiles(home, dest, dir string) map[string][]byte {
	if runtime.GOOS == "windows" {
		return map[string][]byte{filepath.Join(dir, "xpotato-tunnel.ps1"): []byte("& '" + strings.ReplaceAll(dest, "'", "''") + "' @args --home '" + strings.ReplaceAll(home, "'", "''") + "'\r\nexit $LASTEXITCODE\r\n"), filepath.Join(dir, "xpotato-tunnel.cmd"): []byte("@echo off\r\n\"" + dest + "\" %* --home \"" + home + "\"\r\nexit /b %errorlevel%\r\n")}
	}
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	return map[string][]byte{filepath.Join(dir, "xpotato-tunnel"): []byte("#!/bin/sh\nexec " + q(dest) + " \"$@\" --home " + q(home) + "\n")}
}

// Setup offers a short per-user command with local consent. It never mutates
// PATH; subsequent setup/update reuse the selected directory without prompting.
func SetupCommand(home, requested string, prompt core.Prompt) error {
	r, e := readCommandRecord(home)
	if e != nil {
		return e
	}
	if requested != "" && r.Directory != "" && requested != r.Directory {
		return errors.New("existing short command directory preserved; use the previously selected directory")
	}
	if !r.Decided || (requested != "" && r.Directory == "") {
		dir := requested
		if dir == "" {
			h, e := os.UserHomeDir()
			if e != nil {
				return e
			}
			dir = filepath.Join(h, ".local", "bin")
		}
		if e = validateCommandPath(dir); e != nil {
			return e
		}
		yes, e := prompt.Consent("Install short command xpotato-tunnel in '" + dir + "'? PATH stays unchanged; type yes")
		if e != nil {
			return e
		}
		r.Decided = true
		if yes {
			r.Directory = dir
		}
	}
	return installCommand(home, r)
}

// Explicit update installs this reviewed executable and refreshes the already
// approved command location; no background/latest update or new consent scope.
func InstallCommand(home string) error {
	r, e := readCommandRecord(home)
	if e != nil {
		return e
	}
	return installCommand(home, r)
}
func installCommand(home string, r commandRecord) error {
	if e := validateCommandPath(home); e != nil {
		return e
	}
	if r.Directory != "" {
		if e := validateCommandPath(r.Directory); e != nil {
			return e
		}
	}
	i, e := platform.CurrentIdentity()
	if e != nil {
		return errors.New("command installation identity unavailable")
	}
	release, e := lockfile.Acquire(filepath.Join(home, "command-install.lock"), i.Owner)
	if e != nil {
		return e
	}
	defer release()
	source, e := os.Executable()
	if e != nil {
		return e
	}
	name := "xpotato-tunnel"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dest := filepath.Join(home, "core", core.Version, name)
	expected, e := hashFile(source)
	if e != nil {
		return e
	}
	if old, e := hashFile(dest); e == nil {
		if old != expected {
			return errors.New("same core version has different bytes; existing executable preserved")
		}
	} else if os.IsNotExist(e) {
		if e = stageCopy(source, dest, nil); e != nil {
			return e
		}
	} else {
		return e
	}
	files := shimFiles(home, dest, filepath.Join(home, "bin"))
	if r.Directory != "" {
		for p, b := range shimFiles(home, dest, r.Directory) {
			files[p] = b
		}
	}
	intent := commandRecord{}
	if b, e := os.ReadFile(filepath.Join(home, "command-install-intent.json")); e == nil {
		json.Unmarshal(b, &intent)
	}
	legacyName := name
	legacy := shimFiles(home, filepath.Join(home, "core", "0.2.0", legacyName), filepath.Join(home, "bin"))
	for p, b := range files {
		old, e := os.ReadFile(p)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if e == nil {
			hash := hashBytes(old)
			approved := r.Files[p] == hash || (intent.Owner == i.Owner && intent.Version == core.Version && intent.Files[p] == hash) || hash == hashBytes(b)
			if legacyBytes, ok := legacy[p]; ok && hash == hashBytes(legacyBytes) {
				approved = true
			}
			if !approved {
				return errors.New("existing command file was modified or belongs to another installation; preserved")
			}
		}
	}
	next := commandRecord{Owner: i.Owner, Version: core.Version, Directory: r.Directory, Decided: r.Decided, Files: map[string]string{}}
	for p, b := range files {
		next.Files[p] = hashBytes(b)
	}
	if e = saveCommandRecord(filepath.Join(home, "command-install-intent.json"), next); e != nil {
		return e
	}
	for p, b := range files {
		if e = atomicBytes(p, b); e != nil {
			return e
		}
		if runtime.GOOS != "windows" {
			if e = os.Chmod(p, 0700); e != nil {
				return e
			}
		}
	}
	if e = saveCommandRecord(filepath.Join(home, "command-install.json"), next); e != nil {
		return e
	}
	return nil
}
func StartCommand(home string) string {
	r, e := readCommandRecord(home)
	if e == nil && r.Directory != "" {
		for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
			if strings.EqualFold(filepath.Clean(dir), filepath.Clean(r.Directory)) {
				return "xpotato-tunnel"
			}
		}
		if runtime.GOOS == "windows" {
			return filepath.Join(r.Directory, "xpotato-tunnel.cmd")
		}
		return filepath.Join(r.Directory, "xpotato-tunnel")
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "bin", "xpotato-tunnel.cmd")
	}
	return filepath.Join(home, "bin", "xpotato-tunnel")
}
