package managed

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

const readinessTimeout = 30 * time.Second
const maxReceiptBytes = 16 * 1024 * 1024
const launchReceiptScope = "launch-files"

// These are a bounded launch/configuration check, not a claim that every
// transitive dependency is unchanged. Deep diagnosis is an explicit operation.
func launchFiles(root string) []string {
	node, _ := nodeFiles(root)
	relative, _ := filepath.Rel(root, node)
	return []string{executable(), filepath.ToSlash(relative), "package-lock.json", "stdio.mjs", "activity.mjs", "licensing.mjs", "node_modules/@wonderwhy-er/desktop-commander/dist/index.js"}
}

func safeRuntimeFile(root, relative string) (string, error) {
	if relative == "" || strings.ContainsAny(relative, "\\:\x00") || filepath.IsAbs(relative) {
		return "", errors.New("invalid runtime path")
	}
	current := root
	st, err := os.Lstat(root)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("runtime directory unavailable or linked")
	}
	parts := strings.Split(relative, "/")
	for index, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", errors.New("invalid runtime path")
		}
		current = filepath.Join(current, part)
		st, err = os.Lstat(current)
		if err != nil || st.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("runtime launch file missing or linked")
		}
		if index == len(parts)-1 {
			if !st.Mode().IsRegular() {
				return "", errors.New("runtime launch file is not regular")
			}
		} else if !st.IsDir() {
			return "", errors.New("runtime parent is not a directory")
		}
	}
	return current, nil
}

func readReceipt(root string, expected Manifest) (Receipt, error) {
	var receipt Receipt
	path, err := safeRuntimeFile(root, "receipt.json")
	if err != nil {
		return receipt, err
	}
	f, err := os.Open(path)
	if err != nil {
		return receipt, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxReceiptBytes+1))
	if err != nil || len(data) > maxReceiptBytes || json.Unmarshal(data, &receipt) != nil || receipt.Manifest != expected {
		return receipt, errors.New("runtime receipt invalid; run explicit repair")
	}
	if receipt.SchemaVersion != 0 && (receipt.SchemaVersion != 2 || receipt.Scope != launchReceiptScope) {
		return receipt, errors.New("unsupported runtime receipt scope")
	}
	if len(receipt.Files) == 0 || len(receipt.Files) > 100000 {
		return receipt, errors.New("runtime receipt inventory invalid")
	}
	for name, sum := range receipt.Files {
		parts := strings.Split(name, "/")
		if name == "" || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") {
			return receipt, errors.New("invalid runtime receipt path")
		}
		for _, part := range parts {
			if part == "" || part == "." || part == ".." {
				return receipt, errors.New("invalid runtime receipt path")
			}
		}
		digest, e := hex.DecodeString(sum)
		if e != nil || len(digest) != 32 {
			return receipt, errors.New("invalid runtime receipt hash")
		}
	}
	return receipt, nil
}

func captureLaunchReceipt(ctx context.Context, root string, manifest Manifest) (Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, readinessTimeout)
	defer cancel()
	receipt := Receipt{Manifest: manifest, Files: map[string]string{}, SchemaVersion: 2, Scope: launchReceiptScope}
	for _, name := range launchFiles(root) {
		path, err := safeRuntimeFile(root, name)
		if err != nil {
			return receipt, err
		}
		sum, err := hashFileContext(ctx, path)
		if err != nil {
			return receipt, err
		}
		receipt.Files[name] = sum
	}
	return receipt, nil
}

func (m Manager) verifyReadiness(parent context.Context, root string, manifest Manifest) error {
	ctx, cancel := context.WithTimeout(parent, readinessTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	receipt, err := readReceipt(root, manifest)
	if err != nil {
		return err
	}
	names := launchFiles(root)
	var completed atomic.Int64
	return runStageDetail(m.Progress, "起動入口・構成の確認（全依存の診断ではありません、上限30秒）", progressInterval, func() string { return fmt.Sprintf("files %d/%d", completed.Load(), len(names)) }, func() error {
		for _, name := range names {
			path, e := safeRuntimeFile(root, name)
			if e != nil {
				return e
			}
			sum, e := hashFileContext(ctx, path)
			if e != nil {
				return e
			}
			if sum != receipt.Files[name] {
				return errors.New("runtime launch/configuration hash mismatch; run explicit repair")
			}
			if name == executable() && sum != manifest.ExecutableSHA {
				return errors.New("official tunnel executable changed")
			}
			if name == "package-lock.json" && sum != dependencyLockSHA {
				return errors.New("reviewed dependency lock changed")
			}
			for _, asset := range []string{"stdio.mjs", "activity.mjs", "licensing.mjs"} {
				if name == asset {
					data, _ := assets.ReadFile("assets/" + asset)
					if sum != hashBytes(data) {
						return errors.New("runtime adapter differs from this core")
					}
				}
			}
			completed.Add(1)
		}
		entry, e := safeRuntimeFile(root, names[len(names)-1])
		if e != nil {
			return e
		}
		f, e := os.Open(entry)
		if e != nil {
			return e
		}
		defer f.Close()
		data, e := io.ReadAll(io.LimitReader(f, 16*1024*1024+1))
		if len(data) > 16*1024*1024 {
			return errors.New("Commander entry exceeds size limit")
		}
		if e != nil || strings.Count(string(data), marker) != 1 || chromeCall.Match(data) {
			return errors.New("Commander startup suppression missing")
		}
		return ctx.Err()
	})
}

// DeepVerify may download the pinned packages into a private temporary reference
// and scan all files. It is never called by setup, start, update or plugin.
func (m Manager) DeepVerify(home, id string) error {
	manifest, err := m.manifest()
	if err != nil {
		return err
	}
	if !compatibleRuntimeID(manifest, id) {
		return errors.New("unsupported runtime for deep diagnosis")
	}
	root := filepath.Join(home, "releases", id)
	if err = m.verifyReadiness(m.context(), root, manifest); err != nil {
		return err
	}
	receipt, err := readReceipt(root, manifest)
	if err != nil {
		return err
	}
	if receipt.SchemaVersion == 0 {
		return m.verifyDirectory(m.context(), root, manifest)
	}
	ctx, cancel := context.WithTimeout(m.context(), fileOperationTimeout)
	defer cancel()
	reference, err := os.MkdirTemp(filepath.Join(home, "releases"), ".doctor-reference-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(reference) // exact temporary directory created by this call
	populate := m.populate
	if populate == nil {
		populate = m.populateRuntime
	}
	if err = populate(ctx, reference, manifest); err != nil {
		return err
	}
	expected := map[string]string{}
	err = m.inventory(ctx, "診断用 reference の全件hash（明示操作）", reference, func(name, sum string) error { expected[name] = sum; return nil })
	if err != nil {
		return err
	}
	count := 0
	err = m.inventoryWithTotal(ctx, "明示的な全依存診断（上限10分）", root, int64(len(expected)), func(name, sum string) error {
		count++
		if expected[name] != sum {
			return errors.New("runtime differs from pinned package reference")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if count != len(expected) {
		return errors.New("runtime reference inventory differs")
	}
	return nil
}
