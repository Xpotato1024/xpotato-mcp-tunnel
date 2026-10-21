package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// A finite check of the shipped core/docs payload, never an installed runtime.
// The manifest is consistency metadata; publisher authentication belongs to the
// outer signed/digest-pinned installer acquisition, not this self-check.
func packageCheck() (int, error) {
	executable, err := os.Executable()
	if err != nil {
		return 1, err
	}
	root := filepath.Dir(executable)
	if err = checkPackage(root); err != nil {
		return 1, err
	}
	fmt.Println("Installer payload complete; runtime dependencies were not scanned.")
	return 0, nil
}

func checkPackage(root string) error {
	fail := errors.New("installer payload invalid or incomplete")
	read := func(name string, limit int64) ([]byte, error) {
		f, err := os.Open(filepath.Join(root, name))
		if err != nil {
			return nil, fail
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, limit+1))
		if err != nil || int64(len(data)) > limit {
			return nil, fail
		}
		return data, nil
	}
	data, err := read("build-info.json", 4096)
	if err != nil {
		return err
	}
	var info struct {
		Version, Platform string
		Dirty             bool
	}
	if json.Unmarshal(data, &info) != nil || info.Version != core.Version || info.Platform != "windows-amd64" || info.Dirty {
		return fail
	}
	data, err = read("SHA256SUMS", 32768)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 4 || len(lines) > 128 {
		return fail
	}
	seen := map[string]bool{}
	for _, line := range lines {
		parts := strings.SplitN(strings.TrimSpace(line), "  ", 2)
		if len(parts) != 2 {
			return fail
		}
		sum, err := hex.DecodeString(parts[0])
		if err != nil || len(sum) != 32 {
			return fail
		}
		name := parts[1]
		if name == "" || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") || seen[name] {
			return fail
		}
		seen[name] = true
		path := root
		components := strings.Split(name, "/")
		for i, part := range components {
			if part == "" || part == "." || part == ".." {
				return fail
			}
			path = filepath.Join(path, part)
			st, e := os.Lstat(path)
			if e != nil || st.Mode()&os.ModeSymlink != 0 {
				return fail
			}
			if i == len(components)-1 {
				if !st.Mode().IsRegular() || st.Size() > 128*1024*1024 {
					return fail
				}
			} else if !st.IsDir() {
				return fail
			}
		}
		f, e := os.Open(path)
		if e != nil {
			return fail
		}
		hash := sha256.New()
		_, e = io.Copy(hash, io.LimitReader(f, 128*1024*1024+1))
		closeErr := f.Close()
		if e != nil || closeErr != nil || hex.EncodeToString(hash.Sum(nil)) != parts[0] {
			return fail
		}
	}
	for _, required := range []string{"xpotato-tunnel.exe", "build-info.json", "README.md", "licenses/LICENSE", "licenses/COPYRIGHT", "licenses/THIRD-PARTY-NOTICES.txt"} {
		if !seen[required] {
			return fail
		}
	}
	return nil
}
