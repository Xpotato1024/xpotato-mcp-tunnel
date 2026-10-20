// Package atomicfile publishes one complete same-directory file. It protects
// against process interruption; filesystem/power-loss durability is OS dependent.
package atomicfile

import (
	"context"
	"os"
	"path/filepath"
)

func Write(ctx context.Context, path string, data []byte, mode os.FileMode) error {
	return write(ctx, path, data, mode, nil)
}

func write(ctx context.Context, path string, data []byte, mode os.FileMode, hook func(string) error) error {
	checkpoint := func(step string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if hook != nil {
			return hook(step)
		}
		return nil
	}
	if err := checkpoint("before-create"); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".write-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(mode); err != nil {
		return err
	}
	if err = checkpoint("before-write"); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = checkpoint("before-sync"); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = checkpoint("before-replace"); err != nil {
		return err
	}
	// Cancellation after this boundary means publication succeeded. Never report
	// cancellation as if the old generation were still selected after replacement.
	if err = replace(f.Name(), path); err != nil {
		return err
	}
	if hook != nil {
		return hook("after-replace")
	}
	return nil
}
