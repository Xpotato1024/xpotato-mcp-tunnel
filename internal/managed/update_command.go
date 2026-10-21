package managed

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/atomicfile"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/lockfile"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
)

func corePath(home string, p core.CorePin) string {
	name := "xpotato-tunnel"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(home, "core", p.Version, name)
}

func VerifyCore(ctx context.Context, home string, p core.CorePin) error {
	if !p.Valid() {
		return errors.New("core generation metadata invalid")
	}
	h, e := hashFileContext(ctx, corePath(home, p))
	if e != nil {
		return e
	}
	if h != p.SHA256 {
		return errors.New("selected core SHA256 mismatch; existing generation preserved")
	}
	return nil
}
func RunningCore(ctx context.Context) (core.CorePin, error) {
	source, e := os.Executable()
	if e != nil {
		return core.CorePin{}, e
	}
	h, e := hashFileContext(ctx, source)
	return core.CorePin{Version: core.Version, SHA256: h}, e
}
func InstalledCore(home string) (core.CorePin, error) {
	p := core.CorePin{Version: core.Version}
	h, e := hashFile(corePath(home, p))
	p.SHA256 = h
	return p, e
}

type commandPreparation struct {
	ctx             context.Context
	home            string
	record          commandRecord
	current, legacy core.CorePin
	files           map[string][]byte
	unlock          func()
	checkpoint      func(string) error // fixture fault injection; never set by CLI
	closed          bool
}

func (p *commandPreparation) Current() core.CorePin { return p.current }
func (p *commandPreparation) Legacy() core.CorePin  { return p.legacy }
func (p *commandPreparation) Close() {
	if !p.closed {
		p.closed = true
		p.unlock()
	}
}
func (p *commandPreparation) check(step string) error {
	if e := p.ctx.Err(); e != nil {
		return e
	}
	if p.checkpoint != nil {
		return p.checkpoint(step)
	}
	return nil
}

func PrepareCommandUpdate(ctx context.Context, home string) (core.CommandUpdate, error) {
	source, e := os.Executable()
	if e != nil {
		return nil, e
	}
	return prepareCommandUpdate(ctx, home, source)
}

func (m Manager) PrepareCommand(ctx context.Context, home string) (core.CommandUpdate, error) {
	m.Context = ctx
	var prepared core.CommandUpdate
	e := m.stage("新coreの読取・コピー・SHA-256を確認", func() error { var err error; prepared, err = PrepareCommandUpdate(ctx, home); return err })
	return prepared, e
}
func prepareCommandUpdate(ctx context.Context, home, source string) (*commandPreparation, error) {
	return prepareCommandUpdateSelection(ctx, home, source, nil)
}
func prepareCommandUpdateSelection(ctx context.Context, home, source string, selection *commandRecord) (*commandPreparation, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if e := validateCommandPath(home); e != nil {
		return nil, e
	}
	i, e := platform.CurrentIdentity()
	if e != nil {
		return nil, errors.New("command installation identity unavailable")
	}
	unlock, e := lockfile.Acquire(filepath.Join(home, "command-install.lock"), i.Owner)
	if e != nil {
		return nil, e
	}
	p := &commandPreparation{ctx: ctx, home: home, unlock: unlock}
	fail := func(e error) (*commandPreparation, error) { p.Close(); return nil, e }
	r, e := readCommandRecord(home)
	if e != nil {
		return fail(e)
	}
	if selection != nil {
		if r.Directory != "" && r.Directory != selection.Directory {
			return fail(core.ErrCommandDirectoryConflict)
		}
		r.Directory, r.Decided = selection.Directory, selection.Decided
	}
	if r.Directory != "" {
		if e = validateCommandPath(r.Directory); e != nil {
			return fail(e)
		}
	}
	if len(r.Files) == 0 {
		return fail(errors.New("prepared short-command installation not found; complete setup before update"))
	}
	pin, e := prepareCore(ctx, home, source)
	if e != nil {
		return fail(e)
	}
	p.current = pin
	refreshLauncher := false
	if r.Launcher == nil {
		legacy := core.CorePin{Version: r.Version}
		if !coreVersionForPath(legacy.Version) {
			return fail(errors.New("legacy core version invalid"))
		}
		legacy.SHA256, e = hashFileContext(ctx, corePath(home, legacy))
		if e != nil {
			return fail(e)
		}
		r.Launcher = &pin
		r.Fallback = &legacy
		r.PreviousFiles = r.Files
	} else {
		if e = VerifyCore(ctx, home, *r.Launcher); e != nil {
			return fail(e)
		}
		if r.Fallback == nil {
			return fail(errors.New("legacy command fallback missing"))
		}
		if e = VerifyCore(ctx, home, *r.Fallback); e != nil {
			return fail(e)
		}
		if *r.Launcher != pin {
			// A previous launcher cannot parse newly added management commands.
			// Rotate only the dispatcher; each device still selects its own core.
			// Keep PreviousFiles across retries after the new record is published.
			refreshLauncher = true
			r.Launcher = &pin
		}
	}
	p.legacy = *r.Fallback
	files := stableShimFiles(home, corePath(home, *r.Launcher), filepath.Join(home, "bin"))
	if r.Directory != "" {
		for path, b := range stableShimFiles(home, corePath(home, *r.Launcher), r.Directory) {
			files[path] = b
		}
	}
	previousFiles := map[string]string{}
	for path, b := range files {
		old, err := commandFileBytes(path)
		if err != nil && !os.IsNotExist(err) {
			return fail(err)
		}
		if err == nil {
			h := hashBytes(old)
			if h != r.Files[path] && h != r.PreviousFiles[path] && h != hashBytes(b) {
				return fail(errors.New("existing command file was modified; preserved"))
			}
			previousFiles[path] = h
		}
	}
	if refreshLauncher {
		r.PreviousFiles = previousFiles
	}
	r.Files = map[string]string{}
	for path, b := range files {
		r.Files[path] = hashBytes(b)
	}
	r.Version = core.Version
	r.Installer = &pin
	p.record, p.files = r, files
	return p, nil
}
func coreVersionForPath(version string) bool {
	return (core.CorePin{Version: version, SHA256: strings.Repeat("0", 64)}).Valid()
}

func prepareCore(ctx context.Context, home, source string) (core.CorePin, error) {
	h, e := hashFileContext(ctx, source)
	if e != nil {
		return core.CorePin{}, e
	}
	pin := core.CorePin{Version: core.Version, SHA256: h}
	dest := corePath(home, pin)
	if old, e := hashFileContext(ctx, dest); e == nil {
		if old != h {
			return core.CorePin{}, errors.New("same core version has different bytes; existing executable preserved")
		}
	} else if os.IsNotExist(e) {
		if e = stageCopyChecked(ctx, source, dest, h, nil); e != nil {
			return core.CorePin{}, e
		}
	} else {
		return core.CorePin{}, e
	}
	return pin, ctx.Err()
}

func (p *commandPreparation) Migrate() error {
	if p.closed {
		return errors.New("command preparation closed")
	}
	if e := p.check("before-record"); e != nil {
		return e
	}
	for path, b := range p.files {
		if _, e := p.approvedWrapper(path, b); e != nil {
			return e
		}
	}
	// Publish fallback before any wrapper can invoke the fixed launcher. Partial
	// migration accepts only the recorded old bytes or exact new wrapper bytes.
	b, e := json.MarshalIndent(p.record, "", "  ")
	if e != nil {
		return e
	}
	if e = atomicfile.Write(p.ctx, filepath.Join(p.home, "command-install.json"), append(b, '\n'), 0600); e != nil {
		return e
	}
	if e = p.check("record"); e != nil {
		return e
	}
	for path, b := range p.files {
		if e = p.check("before-wrapper"); e != nil {
			return e
		}
		mode := os.FileMode(0600)
		if runtime.GOOS != "windows" {
			mode = 0700
		}
		// Avoid even replacing unchanged wrappers on subsequent updates.
		unchanged, err := p.approvedWrapper(path, b)
		if err != nil {
			return err
		}
		if unchanged {
			continue
		}
		if e = atomicfile.Write(p.ctx, path, b, mode); e != nil {
			return e
		}
		if e = p.check("wrapper"); e != nil {
			return e
		}
	}
	return p.check("complete")
}

func commandFileBytes(path string) ([]byte, error) {
	st, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("command file is not a regular file; preserved")
	}
	return os.ReadFile(path)
}
func (p *commandPreparation) approvedWrapper(path string, want []byte) (bool, error) {
	b, e := commandFileBytes(path)
	if os.IsNotExist(e) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	h := hashBytes(b)
	if h == hashBytes(want) {
		return true, nil
	}
	if h != p.record.Files[path] && h != p.record.PreviousFiles[path] {
		return false, errors.New("command file changed during update; preserved")
	}
	return false, nil
}

func (p *commandPreparation) Verify() error {
	if e := p.check("verify-core"); e != nil {
		return e
	}
	if e := VerifyCore(p.ctx, p.home, p.current); e != nil {
		return e
	}
	if e := VerifyCore(p.ctx, p.home, *p.record.Launcher); e != nil {
		return e
	}
	return VerifyCore(p.ctx, p.home, *p.record.Fallback)
}

func stableShimFiles(home, dest, dir string) map[string][]byte {
	files := shimFiles(home, dest, dir)
	for path, b := range files {
		s := string(b)
		if runtime.GOOS == "windows" {
			if strings.HasSuffix(path, ".cmd") {
				s = strings.Replace(s, "\" %* --home", "\" _dispatch %* --home", 1)
			} else {
				s = strings.Replace(s, " @args --home", " '_dispatch' @args --home", 1)
			}
		} else {
			s = strings.Replace(s, " \"$@\" --home", " _dispatch \"$@\" --home", 1)
		}
		files[path] = []byte(s)
	}
	return files
}

// DispatchTarget reads only owner-checked nonsecret metadata and hashes the
// selected core. The selected core still validates device/scope and bounded
// runtime launch files before loading credentials. It never executes PATH tools.
func DispatchTarget(ctx context.Context, home, device string) (string, error) {
	r, e := readCommandRecord(home)
	if e != nil {
		return "", e
	}
	if r.Launcher == nil || r.Fallback == nil {
		return "", errors.New("stable command installation incomplete")
	}
	pin := *r.Fallback
	if device != "" {
		if !validDeviceName(device) {
			return "", errors.New("invalid device name")
		}
		b, err := os.ReadFile(filepath.Join(home, "devices", device, "device.json"))
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil {
			var d core.Device
			if json.Unmarshal(b, &d) != nil || d.Schema != 3 || d.Name != device || d.Owner != r.Owner {
				return "", errors.New("device dispatch metadata invalid")
			}
			if d.Core != nil {
				pin = *d.Core
			}
		}
	}
	if e = VerifyCore(ctx, home, pin); e != nil {
		return "", e
	}
	return corePath(home, pin), nil
}

// Management commands must understand generation metadata even after rolling
// Start back to a legacy core that predates that metadata.
func ManagementTarget(ctx context.Context, home string) (string, error) {
	r, e := readCommandRecord(home)
	if e != nil {
		return "", e
	}
	if r.Installer == nil {
		return "", errors.New("management core generation missing")
	}
	if e = VerifyCore(ctx, home, *r.Installer); e != nil {
		return "", e
	}
	return corePath(home, *r.Installer), nil
}
func validDeviceName(name string) bool {
	if len(name) < 1 || len(name) > 64 {
		return false
	}
	for i, c := range name {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && (c != '-' || i == 0) {
			return false
		}
	}
	return true
}
