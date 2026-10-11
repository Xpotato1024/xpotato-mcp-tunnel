package core

import (
	"bytes"
	"errors"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func envUpdatePlan(t *testing.T, f fixture, tunnel string, roots []string) Options {
	t.Helper()
	f.o.EnvFile, f.o.Tunnel, f.o.Allow = "fixture.env", tunnel, roots
	o, out, e := preview(t, f, "yes")
	if e != nil || !strings.Contains(out, "変更差分") {
		t.Fatal(e, out)
	}
	return o
}

func TestConfirmedScopeUpdatePreservesOldProfileCustomSettingsAndKey(t *testing.T) {
	f := prepared(t)
	var c map[string]any
	readJSON(configFile(f.o), &c)
	c["custom-setting"] = "preserved"
	atomicJSON(configFile(f.o), c)
	before, _ := os.ReadFile(configFile(f.o))
	roots := []string{f.o.Allow[0], t.TempDir()}
	o := envUpdatePlan(t, f, f.o.Tunnel, roots)
	if _, e := f.app.Setup(o); e != nil {
		t.Fatal(e)
	}
	d, e := f.app.owned(f.o)
	if e != nil || d.ProfileRevision == "" || !equal(d.Approved, roots) {
		t.Fatal("scope not published", d, e)
	}
	old, _ := os.ReadFile(configFile(f.o))
	var next map[string]any
	readJSON(profileConfig(f.o.Home, d), &next)
	if !bytes.Equal(before, old) || next["custom-setting"] != "preserved" || f.v.saves != 1 || f.v.loads != 0 {
		t.Fatal("old profile/custom settings/credential changed")
	}
	if _, e := f.app.Start(f.o); e != nil {
		t.Fatal("updated profile failed scope validation", e)
	}
}

func TestEnvEffectiveScopeExpansionIsShownAndCancelWritesNothing(t *testing.T) {
	f := prepared(t)
	narrow := filepath.Join(f.o.Allow[0], "narrow")
	os.Mkdir(narrow, 0700)
	var c map[string]any
	readJSON(configFile(f.o), &c)
	c["allowedDirectories"] = []string{narrow}
	atomicJSON(configFile(f.o), c)
	before, _ := os.ReadFile(deviceFile(f.o))
	f.o.EnvFile = "fixture.env"
	_, out, e := preview(t, f, "no")
	after, _ := os.ReadFile(deviceFile(f.o))
	if e != ErrSetupCancelled || !bytes.Equal(before, after) || !strings.Contains(out, narrow) || !strings.Contains(out, "追加・拡張") || exists(filepath.Join(directory(f.o), "profiles")) {
		t.Fatal("expansion preview/cancel failed", e, out)
	}
}

func TestUpdateRejectsStaleSnapshotLocksAndInstallFailureBeforePublication(t *testing.T) {
	for _, mode := range []string{"config", "device", "input", "start-lock", "tunnel-lock", "new-tunnel-lock", "maintenance-lock", "staging", "install", "runtime"} {
		t.Run(mode, func(t *testing.T) {
			f := prepared(t)
			o := envUpdatePlan(t, f, f.o.Tunnel, []string{t.TempDir()})
			if mode == "new-tunnel-lock" {
				o = envUpdatePlan(t, f, "tunnel_new", []string{t.TempDir()})
			}
			before, _ := os.ReadFile(deviceFile(f.o))
			switch mode {
			case "config":
				os.WriteFile(configFile(f.o), append(o.reviewed.configBytes, ' '), 0600)
			case "device":
				os.WriteFile(deviceFile(f.o), append(before, ' '), 0600)
				before = append(before, ' ')
			case "input":
				o.Allow = []string{t.TempDir()}
			case "start-lock", "tunnel-lock", "new-tunnel-lock", "maintenance-lock":
				file := filepath.Join(directory(f.o), "start.lock")
				if mode == "tunnel-lock" {
					file = filepath.Join(f.o.Home, "tunnel-locks", digest(f.o.Tunnel)+".lock")
				}
				if mode == "new-tunnel-lock" {
					file = filepath.Join(f.o.Home, "tunnel-locks", digest(o.Tunnel)+".lock")
				}
				if mode == "maintenance-lock" {
					file = filepath.Join(directory(f.o), "maintenance.lock")
				}
				unlock, e := acquire(file, "fixture-owner")
				if e != nil {
					t.Fatal(e)
				}
				defer unlock()
			case "staging":
				if e := os.WriteFile(filepath.Join(directory(f.o), "profiles"), []byte("fixture blocker"), 0600); e != nil {
					t.Fatal(e)
				}
			case "install":
				f.app.InstallCommand = func(string) error { return errors.New("fixture install failure") }
			case "runtime":
				f.r.bad = true
			}
			if _, e := f.app.Setup(o); e == nil {
				t.Fatal("failure accepted")
			}
			after, _ := os.ReadFile(deviceFile(f.o))
			if !bytes.Equal(before, after) || f.v.saves != 1 || f.v.loads != 0 || f.r.runs != 0 {
				t.Fatal("failed update published state/touched key")
			}
		})
	}
}

func TestScopeShrinkAndUnchangedEnvResumeUseSameDeviceWithoutNewKey(t *testing.T) {
	f := prepared(t)
	narrow := filepath.Join(f.o.Allow[0], "narrow")
	if e := os.Mkdir(narrow, 0700); e != nil {
		t.Fatal(e)
	}
	o := envUpdatePlan(t, f, f.o.Tunnel, []string{narrow})
	if _, e := f.app.Setup(o); e != nil {
		t.Fatal(e)
	}
	d, e := f.app.owned(f.o)
	if e != nil || d.Name != f.o.Device || !equal(d.Approved, []string{narrow}) {
		t.Fatal("same-device shrink not published", e)
	}
	f.o.EnvFile, f.o.Allow = "fixture.env", []string{narrow}
	resume, out, e := preview(t, f, "yes")
	if e != nil || strings.Contains(out, "既存端末の変更差分") {
		t.Fatal("unchanged env treated as update", e, out)
	}
	if _, e = f.app.Setup(resume); e != nil {
		t.Fatal(e)
	}
	if f.v.saves != 1 || f.p.keys != 1 || f.v.loads != 0 || f.r.installs != 1 {
		t.Fatal("unchanged resume repeated completed work")
	}
}

type recordingRuntime struct {
	*fakeRuntime
	device Device
}

func (r *recordingRuntime) Run(home string, d Device, key []byte) (int, error) {
	r.device = d
	return r.fakeRuntime.Run(home, d, key)
}

func TestStartUsesNewProfileAndCredentialPublishedBeforeLockedReread(t *testing.T) {
	f := prepared(t)
	d, e := f.app.owned(f.o)
	if e != nil {
		t.Fatal(e)
	}
	newRoots := []string{t.TempDir()}
	d.Approved, d.Tunnel = newRoots, "tunnel_updated"
	d.ProfileRevision = strings.Repeat("a", 64)
	d.CredentialRevision = strings.Repeat("b", 32)
	d.Credential = credentialReference(f.o.Home, d)
	if e = atomicJSON(profileConfig(f.o.Home, d), map[string]any{"allowedDirectories": newRoots}); e != nil {
		t.Fatal(e)
	}
	v := &recordVault{records: map[string]string{d.Credential: "synthetic-updated-key"}}
	r := &recordingRuntime{fakeRuntime: f.r}
	f.app.Vault, f.app.Runtime = v, r
	reads := 0
	f.app.Identity = func() (platform.Identity, error) {
		reads++
		if reads == 2 {
			if !busy(filepath.Join(directory(f.o), "start.lock")) {
				t.Fatal("second state read preceded start lock")
			}
			if e := atomicJSON(deviceFile(f.o), d); e != nil {
				t.Fatal(e)
			}
		}
		return platform.Identity{Owner: "fixture-owner"}, nil
	}
	if code, e := f.app.Start(f.o); e != nil || code != 42 {
		t.Fatal("updated state not used", e)
	}
	if r.device.Credential != d.Credential || r.device.ProfileRevision != d.ProfileRevision || r.device.Tunnel != d.Tunnel || !equal(r.device.Approved, newRoots) || v.loads != 1 {
		t.Fatal("launch retained stale device state")
	}
}

func TestTunnelKeySavedThenInstallFailureKeepsOldPublishedDevice(t *testing.T) {
	f := newFixture(t)
	v := &recordVault{records: map[string]string{}}
	f.app.Vault = v
	if _, e := f.app.Setup(f.o); e != nil {
		t.Fatal(e)
	}
	old, e := f.app.owned(f.o)
	if e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(deviceFile(f.o))
	o := envUpdatePlan(t, f, "tunnel_other", f.o.Allow)
	f.app.InstallCommand = func(string) error { return errors.New("fixture late failure") }
	if _, e = f.app.Setup(o); e == nil {
		t.Fatal("late failure accepted")
	}
	after, _ := os.ReadFile(deviceFile(f.o))
	if !bytes.Equal(before, after) || len(v.records) != 2 || v.records[old.Credential] != "synthetic-test-key" || v.loads != 0 || f.r.runs != 0 {
		t.Fatal("late failure changed old publication/credential")
	}
}

type recordVault struct {
	records map[string]string
	loads   int
}

func (*recordVault) Ready() error                        { return nil }
func (v *recordVault) Save(ref string, key []byte) error { v.records[ref] = string(key); return nil }
func (v *recordVault) Load(ref string) ([]byte, error)   { v.loads++; return []byte(v.records[ref]), nil }

func TestTunnelRoundTripKeepsAllPreviousCredentialRecords(t *testing.T) {
	f := newFixture(t)
	v := &recordVault{records: map[string]string{}}
	f.app.Vault = v
	if _, e := f.app.Setup(f.o); e != nil {
		t.Fatal(e)
	}
	first, _ := f.app.owned(f.o)
	for _, tunnel := range []string{"tunnel_other", "tunnel_fixture"} {
		o := envUpdatePlan(t, f, tunnel, f.o.Allow)
		if _, e := f.app.Setup(o); e != nil {
			t.Fatal(e)
		}
	}
	last, e := f.app.owned(f.o)
	if e != nil || last.Credential == first.Credential || len(v.records) != 3 || v.records[first.Credential] != "synthetic-test-key" || v.loads != 0 {
		t.Fatal("prior keys replaced or read", last, e, v.records)
	}
}

func TestTunnelUpdateDeclinedKeyAndInvalidProfilePreserveState(t *testing.T) {
	f := prepared(t)
	before, _ := os.ReadFile(deviceFile(f.o))
	o := envUpdatePlan(t, f, "tunnel_changed", f.o.Allow)
	f.p.yes = false
	if _, e := f.app.Setup(o); e == nil {
		t.Fatal("declined key accepted")
	}
	after, _ := os.ReadFile(deviceFile(f.o))
	if !bytes.Equal(before, after) || f.v.saves != 1 {
		t.Fatal("declined key published device")
	}
	d, _ := f.app.owned(f.o)
	d.ProfileRevision = "../outside"
	atomicJSON(deviceFile(f.o), d)
	if _, e := f.app.Start(f.o); e == nil || f.v.loads != 0 {
		t.Fatal("unvalidated profile used")
	}
}

func TestInvalidRegistrationUpdateRejectedBeforeConfirmationAndPublication(t *testing.T) {
	f := newFixture(t)
	f.o.Device = strings.Repeat("a", 57)
	if _, e := f.app.Setup(f.o); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(deviceFile(f.o))
	f.o.EnvFile, f.o.MCPURL = "fixture.env", "https://example.invalid/mcp"
	if _, _, e := preview(t, f); e == nil {
		t.Fatal("invalid bundle accepted in preview")
	}
	// Also protect the update capability from a direct in-package caller.
	d, _ := f.app.owned(f.o)
	f.o.reviewed, _ = reviewExisting(f.o, d)
	if _, e := f.app.Setup(f.o); e == nil {
		t.Fatal("invalid bundle published")
	}
	after, _ := os.ReadFile(deviceFile(f.o))
	if !bytes.Equal(before, after) || f.v.saves != 1 || f.v.loads != 0 || f.r.runs != 0 {
		t.Fatal("rejected registration changed state")
	}
}

func TestPostPublicationRegistrationFailureIsExplicitAndResumable(t *testing.T) {
	f := prepared(t)
	o := envUpdatePlan(t, f, f.o.Tunnel, []string{t.TempDir()})
	registration := filepath.Join(directory(f.o), "registration")
	// Replace only a disposable fixture directory with a blocker file.
	if e := os.RemoveAll(registration); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(registration, []byte("fixture blocker"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := f.app.Setup(o); e == nil || !strings.Contains(e.Error(), "更新は保存済み") {
		t.Fatal("partial publication was ambiguous", e)
	}
	d, e := f.app.owned(f.o)
	if e != nil || !equal(d.Approved, o.Allow) {
		t.Fatal("published update unusable", e)
	}
	if e := os.Remove(registration); e != nil {
		t.Fatal(e)
	}
	f.o.EnvFile, f.o.Allow = "fixture.env", o.Allow
	resume, _, e := preview(t, f, "yes")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.app.Setup(resume); e != nil {
		t.Fatal("partial update cannot resume", e)
	}
	if f.v.saves != 1 || f.v.loads != 0 {
		t.Fatal("resume touched completed key")
	}
}
