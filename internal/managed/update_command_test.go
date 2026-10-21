package managed

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
)

func TestMigrationAbruptExitPreservesFallbackAndCanResume(t *testing.T) {
	if home := os.Getenv("XPOTATO_MIGRATION_FIXTURE"); home != "" {
		source, _ := os.Executable()
		p, e := prepareCommandUpdate(context.Background(), home, source)
		if e != nil {
			os.Exit(2)
		}
		p.checkpoint = func(step string) error {
			if step == os.Getenv("XPOTATO_MIGRATION_STEP") {
				os.WriteFile(filepath.Join(home, "ready"), []byte("ready"), 0600)
				for {
					time.Sleep(time.Second)
				}
			}
			return nil
		}
		if e = p.Migrate(); e != nil {
			os.Exit(3)
		}
		p.Close()
		os.Exit(0)
	}
	for _, step := range []string{"record", "wrapper", "complete"} {
		t.Run(step, func(t *testing.T) {
			home, _ := legacyCommandFixture(t)
			source, _ := os.Executable()
			cmd := exec.Command(source, "-test.run=^TestMigrationAbruptExitPreservesFallbackAndCanResume$")
			cmd.Env = append(os.Environ(), "XPOTATO_MIGRATION_FIXTURE="+home, "XPOTATO_MIGRATION_STEP="+step)
			if e := cmd.Start(); e != nil {
				t.Fatal(e)
			}
			defer func() { cmd.Process.Kill(); cmd.Wait() }()
			deadline := time.Now().Add(10 * time.Second)
			for {
				if _, e := os.Stat(filepath.Join(home, "ready")); e == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("migration fixture not ready")
				}
				time.Sleep(10 * time.Millisecond)
			}
			cmd.Process.Kill()
			cmd.Wait()
			target, e := DispatchTarget(context.Background(), home, "")
			if e != nil || target != corePath(home, core.CorePin{Version: "0.2.6"}) {
				t.Fatal("old fallback lost", e)
			}
			p, e := prepareCommandUpdate(context.Background(), home, source)
			if e != nil {
				t.Fatal("OS lock did not release/recovery rejected", e)
			}
			if e = p.Migrate(); e != nil {
				t.Fatal(e)
			}
			p.Close()
		})
	}
}

func TestStableCommandSetupCanAddPreviouslyDeclinedDirectory(t *testing.T) {
	home, _ := legacyCommandFixture(t)
	source, _ := os.Executable()
	p, e := prepareCommandUpdate(context.Background(), home, source)
	if e != nil {
		t.Fatal(e)
	}
	if e = p.Migrate(); e != nil {
		t.Fatal(e)
	}
	p.Close()
	bin := t.TempDir()
	if e = SetupCommand(home, bin, &commandPrompt{yes: true}); e != nil {
		t.Fatal(e)
	}
	r, e := readCommandRecord(home)
	if e != nil || r.Directory != bin {
		t.Fatal("approved selection lost", e)
	}
	for path, want := range stableShimFiles(home, corePath(home, *r.Launcher), bin) {
		got, e := os.ReadFile(path)
		if e != nil || string(got) != string(want) {
			t.Fatal("new approved wrapper missing", e)
		}
	}
}

func TestCoreChangesAfterPreparationAreRejectedBeforeActivation(t *testing.T) {
	home, _ := legacyCommandFixture(t)
	source, _ := os.Executable()
	p, e := prepareCommandUpdate(context.Background(), home, source)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	os.WriteFile(corePath(home, p.Current()), []byte("changed after source preflight"), 0700)
	if e = p.Verify(); e == nil {
		t.Fatal("changed prepared core accepted")
	}
}

func TestDispatcherRefreshPreservesDeviceFallbackAndResumes(t *testing.T) {
	home, record := legacyCommandFixture(t)
	source, _ := os.Executable()
	old := core.CorePin{Version: record.Version}
	old.SHA256, _ = hashFile(corePath(home, old))
	record.Launcher, record.Fallback, record.Installer = &old, &old, &old
	record.Files = map[string]string{}
	for name, data := range stableShimFiles(home, corePath(home, old), filepath.Join(home, "bin")) {
		atomicBytes(name, data)
		record.Files[name] = hashBytes(data)
	}
	saveCommandRecord(filepath.Join(home, "command-install.json"), record)
	p, err := prepareCommandUpdate(context.Background(), home, source)
	if err != nil {
		t.Fatal(err)
	}
	if *p.record.Launcher != p.current || *p.record.Fallback != old {
		t.Fatal("dispatcher rotation changed fallback")
	}
	// Simulate interruption after the record, before any wrapper is replaced.
	if err = saveCommandRecord(filepath.Join(home, "command-install.json"), p.record); err != nil {
		t.Fatal(err)
	}
	p.Close()
	p, err = prepareCommandUpdate(context.Background(), home, source)
	if err != nil {
		t.Fatal("old wrappers could not resume", err)
	}
	defer p.Close()
	if err = p.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err = p.Verify(); err != nil {
		t.Fatal(err)
	}
	target, err := DispatchTarget(context.Background(), home, "")
	if err != nil || target != corePath(home, old) {
		t.Fatal("device fallback changed", err)
	}
}

func legacyCommandFixture(t *testing.T) (string, commandRecord) {
	t.Helper()
	home := t.TempDir()
	if e := SetupCommand(home, "", &commandPrompt{}); e != nil {
		t.Fatal(e)
	}
	r, e := readCommandRecord(home)
	if e != nil {
		t.Fatal(e)
	}
	source, _ := os.Executable()
	old := corePath(home, core.CorePin{Version: "0.2.6"})
	if e = stageCopy(source, old, nil); e != nil {
		t.Fatal(e)
	}
	r.Version = "0.2.6"
	r.Files = map[string]string{}
	for path, b := range shimFiles(home, old, filepath.Join(home, "bin")) {
		if e = atomicBytes(path, b); e != nil {
			t.Fatal(e)
		}
		r.Files[path] = hashBytes(b)
	}
	if e = saveCommandRecord(filepath.Join(home, "command-install.json"), r); e != nil {
		t.Fatal(e)
	}
	return home, r
}

func TestCommandUpdateMigrationFailureResumesWithLegacyFallback(t *testing.T) {
	for _, step := range []string{"record", "wrapper", "complete"} {
		t.Run(step, func(t *testing.T) {
			home, old := legacyCommandFixture(t)
			source, _ := os.Executable()
			p, e := prepareCommandUpdate(context.Background(), home, source)
			if e != nil {
				t.Fatal(e)
			}
			p.checkpoint = func(s string) error {
				if s == step {
					return errors.New("fixture interruption")
				}
				return nil
			}
			if e = p.Migrate(); e == nil {
				t.Fatal("fixture failure ignored")
			}
			p.Close()
			r, e := readCommandRecord(home)
			if e != nil {
				t.Fatal(e)
			}
			if r.Fallback == nil || r.Fallback.Version != old.Version {
				t.Fatal("legacy fallback lost")
			}
			target, e := DispatchTarget(context.Background(), home, "")
			if e != nil || target != corePath(home, *r.Fallback) {
				t.Fatal("unselected device did not retain old core", e)
			}
			p, e = prepareCommandUpdate(context.Background(), home, source)
			if e != nil {
				t.Fatal(e)
			}
			if e = p.Migrate(); e != nil {
				t.Fatal("resume rejected partial migration", e)
			}
			p.Close()
			for path, want := range stableShimFiles(home, corePath(home, *r.Launcher), filepath.Join(home, "bin")) {
				got, _ := os.ReadFile(path)
				if string(got) != string(want) {
					t.Fatal("incomplete wrapper migration")
				}
			}
		})
	}
}

func TestCommandPreflightRejectsSameVersionDifferentBytes(t *testing.T) {
	home, _ := legacyCommandFixture(t)
	dest := corePath(home, core.CorePin{Version: core.Version})
	os.WriteFile(dest, []byte("same-version different bytes"), 0700)
	source, _ := os.Executable()
	if p, e := prepareCommandUpdate(context.Background(), home, source); e == nil {
		p.Close()
		t.Fatal("same-version overwrite accepted")
	}
	b, _ := os.ReadFile(dest)
	if string(b) != "same-version different bytes" {
		t.Fatal("existing core changed")
	}
}

func TestCommandMigrationPreservesModifiedWrapper(t *testing.T) {
	home, r := legacyCommandFixture(t)
	var modified string
	for path := range r.Files {
		modified = path
		break
	}
	os.WriteFile(modified, []byte("owner edited"), 0600)
	source, _ := os.Executable()
	if p, e := prepareCommandUpdate(context.Background(), home, source); e == nil {
		p.Close()
		t.Fatal("modified wrapper accepted")
	}
	b, _ := os.ReadFile(modified)
	if string(b) != "owner edited" {
		t.Fatal("owner file changed")
	}
}

func TestWrapperEditedAfterPreparationIsPreserved(t *testing.T) {
	home, r := legacyCommandFixture(t)
	source, _ := os.Executable()
	p, e := prepareCommandUpdate(context.Background(), home, source)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	var modified string
	for path := range r.Files {
		modified = path
		break
	}
	os.WriteFile(modified, []byte("owner edit during long runtime preparation"), 0600)
	if e = p.Migrate(); e == nil {
		t.Fatal("late wrapper edit overwritten")
	}
	b, _ := os.ReadFile(modified)
	if string(b) != "owner edit during long runtime preparation" {
		t.Fatal("late owner edit lost")
	}
}

func TestLegacySetupRequiresExplicitUpdateBeforeRewritingWrappers(t *testing.T) {
	home, r := legacyCommandFixture(t)
	before := map[string]string{}
	for path := range r.Files {
		b, _ := os.ReadFile(path)
		before[path] = string(b)
	}
	if e := SetupCommand(home, "", &commandPrompt{}); e == nil {
		t.Fatal("legacy setup bypassed generation migration")
	}
	if _, e := PreviewCommand(home, ""); e == nil {
		t.Fatal("legacy setup did not fail before wizard")
	}
	for path, want := range before {
		b, _ := os.ReadFile(path)
		if string(b) != want {
			t.Fatal("legacy wrapper changed")
		}
	}
}

func TestDispatchUsesDeviceCoreAndRejectsChangedBytes(t *testing.T) {
	home, _ := legacyCommandFixture(t)
	source, _ := os.Executable()
	p, e := prepareCommandUpdate(context.Background(), home, source)
	if e != nil {
		t.Fatal(e)
	}
	if e = p.Migrate(); e != nil {
		t.Fatal(e)
	}
	pin := p.Current()
	p.Close()
	dir := filepath.Join(home, "devices", "fixture")
	os.MkdirAll(dir, 0700)
	i, e := platform.CurrentIdentity()
	if e != nil {
		t.Fatal(e)
	}
	d := core.Device{Schema: 3, Owner: i.Owner, Name: "fixture", Core: &pin}
	b, _ := json.Marshal(d)
	os.WriteFile(filepath.Join(dir, "device.json"), b, 0600)
	target, e := DispatchTarget(context.Background(), home, "fixture")
	if e != nil || target != corePath(home, pin) {
		t.Fatal("selected wrong core", e)
	}
	os.WriteFile(target, []byte("tampered"), 0700)
	if _, e = DispatchTarget(context.Background(), home, "fixture"); e == nil {
		t.Fatal("tampered selected core accepted")
	}
}
