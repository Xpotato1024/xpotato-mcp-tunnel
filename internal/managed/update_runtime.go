package managed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/lockfile"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/processctrl"
)

func (m Manager) context() context.Context {
	if m.Context != nil {
		return m.Context
	}
	return context.Background()
}
func (m Manager) manifest() (Manifest, error) {
	if m.runtimeManifest != nil {
		return m.runtimeManifest()
	}
	return manifest()
}

func compatibleRuntimeID(m Manifest, id string) bool {
	if id == releaseID(m) {
		return true
	}
	// These reviewed legacy recipes have the same dependency pins and adapters.
	// The directory name alone never authorizes adoption: full verification follows.
	for _, version := range []string{"0.2.6", "0.2.7", "0.2.8", "0.2.9"} {
		if id == "core-"+version+"-"+m.Platform+"-tunnel-"+tunnelVersion+"-dc-"+commanderVersion+"-node-"+nodeVersion {
			return true
		}
	}
	return false
}

type runtimePreparation struct {
	manager                Manager
	manifest               Manifest
	id, destination, stage string
	unlock                 func()
	closed, consumed       bool
}

func (manager Manager) PrepareRuntime(home, current string) (core.PreparedRuntime, error) {
	ctx := manager.context()
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	m, e := manager.manifest()
	if e != nil {
		return nil, e
	}
	releases := filepath.Join(home, "releases")
	i, e := platform.CurrentIdentity()
	if e != nil {
		return nil, errors.New("install lock owner unavailable")
	}
	unlock, e := lockfile.Acquire(filepath.Join(releases, "install.lock"), i.Owner)
	if e != nil {
		return nil, e
	}
	p := &runtimePreparation{manager: manager, manifest: m, id: releaseID(m), unlock: unlock}
	fail := func(e error) (core.PreparedRuntime, error) { p.Close(); return nil, e }
	// Only the explicitly selected compatible runtime is considered for legacy
	// reuse. No enumeration/rehashing of other installations or staged leftovers.
	if compatibleRuntimeID(m, current) {
		candidate := filepath.Join(releases, current)
		if st, err := os.Lstat(candidate); err == nil {
			if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
				return fail(errors.New("runtime generation directory invalid"))
			}
			p.id = current
		} else if !os.IsNotExist(err) {
			return fail(err)
		}
	}
	p.destination = filepath.Join(releases, p.id)
	if st, e := os.Lstat(p.destination); e == nil {
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return fail(errors.New("runtime generation directory invalid"))
		}
		return p, nil
	} else if !os.IsNotExist(e) {
		return fail(e)
	}
	p.stage, e = os.MkdirTemp(releases, ".stage-")
	if e != nil {
		return fail(e)
	}
	populate := manager.populate
	if populate == nil {
		populate = manager.populateRuntime
	}
	if e = populate(ctx, p.stage, m); e != nil {
		return fail(e)
	}
	return p, nil
}

func (p *runtimePreparation) Verify() (string, error) {
	if p.closed || p.consumed {
		return "", errors.New("runtime preparation already consumed")
	}
	p.consumed = true
	dir := p.stage
	if dir == "" {
		dir = p.destination
	}
	if e := p.manager.verifyDirectory(p.manager.context(), dir, p.manifest); e != nil {
		return "", e
	}
	if e := p.manager.context().Err(); e != nil {
		return "", e
	}
	if p.stage != "" {
		if e := os.Rename(p.stage, p.destination); e != nil {
			return "", e
		}
		p.stage = ""
	}
	return p.id, nil
}
func (p *runtimePreparation) Close() {
	if p.closed {
		return
	}
	p.closed = true
	if p.stage != "" {
		os.RemoveAll(p.stage)
	} // exact temporary child created above only
	if p.unlock != nil {
		p.unlock()
	}
}

func (m Manager) inventory(ctx context.Context, label, root string, visit func(string, string) error) error {
	ctx, cancel := context.WithTimeout(ctx, fileOperationTimeout)
	defer cancel()
	if m.inventoryStarted != nil {
		m.inventoryStarted()
	}
	var files, bytes atomic.Int64
	detail := func() string { return fmt.Sprintf("%d files / %d bytes", files.Load(), bytes.Load()) }
	return runStageDetail(m.Progress, label, progressInterval, detail, func() error {
		return walkFileHashesObserved(ctx, root, visit, func(p InventoryProgress) { files.Store(p.Files); bytes.Store(p.Bytes) })
	})
}

func runOwnedContext(ctx context.Context, cmd *exec.Cmd) error {
	code, e := processctrl.RunContext(ctx, cmd, nil, time.Second)
	if e != nil {
		return e
	}
	if code != 0 {
		return errors.New("owned command failed")
	}
	return nil
}
