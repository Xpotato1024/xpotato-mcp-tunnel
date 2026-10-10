// Package lockfile keeps a stable, owner-checked file and a non-inheritable OS
// lock. Kernel locks disappear on abrupt process exit; lock files are never
// deleted/replaced, avoiding stale-recovery races with another live holder.
package lockfile

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

var ErrBusy = errors.New("another operation holds this lock; no process or file was removed")
var ErrOwner = errors.New("lock owner or legacy format is unverified; existing file preserved")

type record struct {
	Schema int    `json:"schemaVersion"`
	Owner  string `json:"owner"`
	PID    int    `json:"pid"`
}

func Acquire(path, owner string) (func(), error) {
	if owner == "" {
		return nil, ErrOwner
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return nil, e
	}
	f, e := openLock(path)
	if e != nil {
		return nil, e
	}
	fail := func(err error) (func(), error) { f.Close(); return nil, err }
	if e = ownedFile(f); e != nil {
		return fail(ErrOwner)
	}
	if e = tryLock(f); e != nil {
		return fail(e)
	}
	// Read/write only while exclusively holding the exact same file handle.
	b, e := io.ReadAll(io.LimitReader(f, 4097))
	if e != nil {
		return fail(e)
	}
	if len(b) > 0 {
		var r record
		if len(b) > 4096 || json.Unmarshal(b, &r) != nil || r.Schema != 1 || r.Owner != owner {
			return fail(ErrOwner)
		}
	}
	metadata := path + ".owner.json"
	if previous, e := os.ReadFile(metadata); e == nil {
		var r record
		if json.Unmarshal(previous, &r) != nil || r.Schema != 1 || r.Owner != owner {
			return fail(ErrOwner)
		}
	} else if !os.IsNotExist(e) {
		return fail(e)
	}
	b, _ = json.Marshal(record{1, owner, os.Getpid()})
	// Keep the locked inode stable. Publish metadata separately by rename so
	// interruption cannot leave partially overwritten JSON blocking resumption.
	stage, err := os.CreateTemp(filepath.Dir(path), ".lock-metadata-")
	if err != nil {
		return fail(err)
	}
	defer os.Remove(stage.Name())
	_, e = stage.Write(b)
	if e == nil {
		e = stage.Sync()
	}
	ce := stage.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(stage.Name(), metadata)
	}
	if e != nil {
		return fail(e)
	}
	return func() { unlock(f); f.Close() }, nil
}

// Active reads no metadata and changes no content. A retained but unlocked
// file is inactive; an unknown/error state fails closed for callers.
func Active(path string) (bool, error) {
	f, e := os.Open(path)
	if os.IsNotExist(e) {
		return false, nil
	}
	if e != nil {
		return true, e
	}
	defer f.Close()
	if e = ownedFile(f); e != nil {
		return true, ErrOwner
	}
	if e = tryLock(f); e != nil {
		if errors.Is(e, ErrBusy) {
			return true, nil
		}
		return true, e
	}
	unlock(f)
	return false, nil
}
