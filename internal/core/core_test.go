package core

import (
	"archive/zip"
	"errors"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeVault struct {
	key                 []byte
	saves, loads, ready int
	err                 error
}

func (v *fakeVault) Ready() error { v.ready++; return v.err }
func (v *fakeVault) Save(_ string, key []byte) error {
	v.saves++
	if v.err != nil {
		return v.err
	}
	v.key = append([]byte(nil), key...)
	return nil
}
func (v *fakeVault) Load(string) ([]byte, error) {
	v.loads++
	if v.err != nil {
		return nil, v.err
	}
	if len(v.key) == 0 {
		return nil, platform.ErrMissing
	}
	return append([]byte(nil), v.key...), nil
}

type fakePrompt struct {
	consents, keys int
	yes            bool
}

func (p *fakePrompt) Text(string) (string, error) { return "", errors.New("unexpected text prompt") }
func (p *fakePrompt) Consent(label string) (bool, error) {
	if strings.Contains(label, "previous launcher") {
		return true, nil
	}
	p.consents++
	return p.yes, nil
}
func (p *fakePrompt) Key() ([]byte, error) { p.keys++; return []byte("synthetic-test-key"), nil }

type fakeRuntime struct {
	installs, runs int
	next           string
	bad            bool
	received       []byte
}

func (r *fakeRuntime) Install(home string) (string, error) {
	r.installs++
	if r.bad {
		return "", errors.New("fixture installation failure")
	}
	if r.next == "" {
		r.next = "release-one"
	}
	os.MkdirAll(filepath.Join(home, "releases", r.next), 0700)
	return r.next, nil
}
func (r *fakeRuntime) Verify(home, id string) error {
	if r.bad || id == "" || !exists(filepath.Join(home, "releases", id)) {
		return errors.New("fixture integrity failure")
	}
	return nil
}
func (r *fakeRuntime) Run(_ string, _ Device, key []byte) (int, error) {
	r.runs++
	r.received = key
	return 42, nil
}

type fixture struct {
	app App
	o   Options
	v   *fakeVault
	p   *fakePrompt
	r   *fakeRuntime
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	home := filepath.Join(t.TempDir(), "App With Spaces")
	allowed := t.TempDir()
	v := &fakeVault{}
	p := &fakePrompt{yes: true}
	r := &fakeRuntime{}
	a := App{Identity: func() (platform.Identity, error) { return platform.Identity{Owner: "fixture-owner"}, nil }, Vault: v, Prompt: p, Runtime: r}
	return fixture{a, Options{Home: home, Device: "llm-01", Tunnel: "tunnel_fixture", Allow: []string{allowed}}, v, p, r}
}
func prepared(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t)
	if _, e := f.app.Setup(f.o); e != nil {
		t.Fatal(e)
	}
	return f
}
func TestSetupOnceAndPromptFreeStart(t *testing.T) {
	f := prepared(t)
	config, _ := os.ReadFile(configFile(f.o))
	if _, e := f.app.Setup(f.o); e != nil {
		t.Fatal(e)
	}
	if f.v.saves != 1 || f.p.keys != 1 || f.p.consents != 1 || f.r.installs != 1 {
		t.Fatal("setup repeated completed work")
	}
	code, e := f.app.Start(f.o)
	if e != nil || code != 42 {
		t.Fatalf("start %d %v", code, e)
	}
	if f.v.loads != 1 || f.p.keys != 1 || f.r.runs != 1 {
		t.Fatal("start prompted or failed to retrieve saved key")
	}
	for _, b := range f.r.received {
		if b != 0 {
			t.Fatal("loaded key buffer not cleared")
		}
	}
	current, _ := os.ReadFile(configFile(f.o))
	if string(config) != string(current) {
		t.Fatal("config changed")
	}
	b, _ := os.ReadFile(deviceFile(f.o))
	if strings.Contains(string(b), "synthetic-test-key") {
		t.Fatal("secret in state")
	}
	if busy(filepath.Join(directory(f.o), "start.lock")) {
		t.Fatal("start lock leaked")
	}
}
func TestConsentBeforeKeyAndResume(t *testing.T) {
	f := newFixture(t)
	f.p.yes = false
	if _, e := f.app.Setup(f.o); e == nil {
		t.Fatal("decline ignored")
	}
	if f.v.saves != 0 || f.p.keys != 0 {
		t.Fatal("key collected or saved without consent")
	}
	f.p.yes = true
	if _, e := f.app.Setup(f.o); e != nil {
		t.Fatal(e)
	}
	if f.r.installs != 1 || f.v.saves != 1 {
		t.Fatal("resume failed")
	}
}
func TestInterruptedConfigCreationResumes(t *testing.T) {
	f := prepared(t)
	var d Device
	readJSON(deviceFile(f.o), &d)
	d.ConfigReady = false
	d.CredentialReady = false
	atomicJSON(deviceFile(f.o), d)
	os.Remove(configFile(f.o))
	if _, e := f.app.Setup(f.o); e != nil {
		t.Fatal(e)
	}
	if !exists(configFile(f.o)) {
		t.Fatal("pending config not reconstructed")
	}
}
func TestLockedAndUnavailableNeverPromptOrSave(t *testing.T) {
	for _, failure := range []error{platform.ErrLocked, platform.ErrUnavailable} {
		t.Run(failure.Error(), func(t *testing.T) {
			f := newFixture(t)
			f.v.err = failure
			if _, e := f.app.Setup(f.o); !errors.Is(e, failure) {
				t.Fatal(e)
			}
			if f.p.keys != 0 || f.v.saves != 0 {
				t.Fatal("fallback or collection attempted")
			}
		})
	}
}
func TestMissingKeyDoesNotPrompt(t *testing.T) {
	f := prepared(t)
	f.v.key = nil
	if _, e := f.app.Start(f.o); !errors.Is(e, platform.ErrMissing) {
		t.Fatal(e)
	}
	if f.p.keys != 1 || f.r.runs != 0 {
		t.Fatal("unexpected prompt/start")
	}
}
func TestIntegrityBeforeCredentialRead(t *testing.T) {
	f := prepared(t)
	f.r.bad = true
	if _, e := f.app.Start(f.o); e == nil {
		t.Fatal("bad runtime accepted")
	}
	if f.v.loads != 0 {
		t.Fatal("credential read before verification")
	}
}
func TestIdentityBeforeWritesAndCredentialReads(t *testing.T) {
	f := newFixture(t)
	f.app.Identity = func() (platform.Identity, error) {
		return platform.Identity{Owner: "fixture-owner", Elevated: true}, nil
	}
	if _, e := f.app.Setup(f.o); e == nil {
		t.Fatal("elevated setup accepted")
	}
	if exists(f.o.Home) || f.p.keys != 0 {
		t.Fatal("elevated write")
	}
	f = prepared(t)
	f.app.Identity = func() (platform.Identity, error) { return platform.Identity{Owner: "other"}, nil }
	if _, e := f.app.Start(f.o); e == nil {
		t.Fatal("other owner accepted")
	}
	if f.v.loads != 0 {
		t.Fatal("other owner read key")
	}
}
func TestScopeNarrowAllowedWidenRejected(t *testing.T) {
	f := prepared(t)
	var c map[string]any
	readJSON(configFile(f.o), &c)
	c["custom-setting"] = "preserved"
	c["allowedDirectories"] = []string{filepath.Join(f.o.Allow[0], "narrow")}
	atomicJSON(configFile(f.o), c)
	before, _ := os.ReadFile(configFile(f.o))
	if _, e := f.app.Setup(f.o); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(configFile(f.o))
	if string(before) != string(after) {
		t.Fatal("custom config overwritten")
	}
	c["allowedDirectories"] = []string{filepath.Dir(f.o.Allow[0])}
	atomicJSON(configFile(f.o), c)
	if _, e := f.app.Start(f.o); e == nil {
		t.Fatal("widen accepted")
	}
	if f.v.loads != 0 {
		t.Fatal("key read for widened config")
	}
}
func TestDeviceAndTunnelDuplicateLocksBeforeKey(t *testing.T) {
	for _, kind := range []string{"device", "tunnel", "maintenance"} {
		t.Run(kind, func(t *testing.T) {
			f := prepared(t)
			file := filepath.Join(directory(f.o), "start.lock")
			if kind == "tunnel" {
				file = filepath.Join(f.o.Home, "tunnel-locks", digest(f.o.Tunnel)+".lock")
			}
			if kind == "maintenance" {
				file = filepath.Join(directory(f.o), "maintenance.lock")
			}
			unlock, e := acquire(file, "fixture-owner")
			if e != nil {
				t.Fatal(e)
			}
			defer unlock()
			if _, e = f.app.Start(f.o); e == nil {
				t.Fatal("lock ignored")
			}
			if f.v.loads != 0 {
				t.Fatal("duplicate read key")
			}
		})
	}
}
func TestSameTunnelAcrossDeviceBlocked(t *testing.T) {
	f := prepared(t)
	o := f.o
	o.Device = "llm-02"
	if _, e := f.app.Setup(o); e != nil {
		t.Fatal(e)
	}
	unlock, _ := acquire(filepath.Join(o.Home, "tunnel-locks", digest(o.Tunnel)+".lock"), "fixture-owner")
	defer unlock()
	if _, e := f.app.Start(o); e == nil {
		t.Fatal("same tunnel duplicate")
	}
	if f.v.loads != 0 {
		t.Fatal("key read")
	}
}
func TestUpdateRollbackPreservesConfigAndOldRuntime(t *testing.T) {
	f := prepared(t)
	before, _ := os.ReadFile(configFile(f.o))
	f.r.next = "release-two"
	if e := f.app.Update(f.o, false); e != nil {
		t.Fatal(e)
	}
	var d Device
	readJSON(deviceFile(f.o), &d)
	if d.Release != "release-two" || d.Previous != "release-one" {
		t.Fatal(d)
	}
	if e := f.app.Update(f.o, true); e != nil {
		t.Fatal(e)
	}
	readJSON(deviceFile(f.o), &d)
	if d.Release != "release-one" || !exists(filepath.Join(f.o.Home, "releases", "release-two")) {
		t.Fatal("rollback removed release")
	}
	after, _ := os.ReadFile(configFile(f.o))
	if string(before) != string(after) || f.v.loads != 0 || f.p.keys != 1 {
		t.Fatal("update changed settings or credentials")
	}
}
func TestFailedUpdateKeepsPin(t *testing.T) {
	f := prepared(t)
	before, _ := os.ReadFile(deviceFile(f.o))
	f.r.bad = true
	if e := f.app.Update(f.o, false); e == nil {
		t.Fatal("failure ignored")
	}
	after, _ := os.ReadFile(deviceFile(f.o))
	if string(before) != string(after) {
		t.Fatal("failed update changed state")
	}
}
func TestDoctorDoesNotTouchVault(t *testing.T) {
	f := prepared(t)
	f.v.err = platform.ErrLocked
	reads := f.v.ready
	if _, e := f.app.Doctor(f.o); e != nil {
		t.Fatal(e)
	}
	if f.v.loads != 0 || f.v.ready != reads {
		t.Fatal("doctor touched vault")
	}
}
func TestDefaultDeviceAndCredentialReplacement(t *testing.T) {
	f := prepared(t)
	o, e := Resolve(Options{Home: f.o.Home})
	if e != nil || o.Device != f.o.Device {
		t.Fatal(e)
	}
	if e = f.app.Credentials(o); e != nil {
		t.Fatal(e)
	}
	if f.v.saves != 2 || f.p.consents != 2 {
		t.Fatal("replacement consent missing")
	}
}
func TestRegistrationHandoffAndOptionalBundle(t *testing.T) {
	f := prepared(t)
	root := filepath.Join(directory(f.o), "registration")
	b, _ := os.ReadFile(filepath.Join(root, "tunnel-registration.json"))
	if !strings.Contains(string(b), "account-registration-not-probed") || strings.Contains(string(b), "synthetic-test-key") {
		t.Fatal("registration claim/secret")
	}
	f = newFixture(t)
	f.o.MCPURL = "https://mcp.example.invalid/mcp"
	if _, e := f.app.Setup(f.o); e != nil {
		t.Fatal(e)
	}
	z, e := zip.OpenReader(filepath.Join(directory(f.o), "registration", "xpotato-llm-01.zip"))
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	if len(z.File) != 2 || z.File[0].Name != "xpotato-llm-01/plugin.json" {
		t.Fatal("invalid bundle root")
	}
}
func TestRejectCredentialBearingEndpointAndTraversalDevice(t *testing.T) {
	for _, endpoint := range []string{"https://key@host/mcp", "https://host/mcp?key=secret", "http://host/mcp"} {
		if validateEndpoint(endpoint) == nil {
			t.Fatal("unsafe endpoint")
		}
	}
	f := newFixture(t)
	f.o.Device = "../other"
	if _, e := f.app.Setup(f.o); e == nil {
		t.Fatal("traversal")
	}
	if exists(f.o.Home) {
		t.Fatal("invalid device wrote state")
	}
}
func TestLegacySchemaNotAdopted(t *testing.T) {
	f := newFixture(t)
	atomicJSON(deviceFile(f.o), map[string]any{"schemaVersion": 2, "ownerSid": "fixture"})
	if _, e := f.app.Setup(f.o); e == nil {
		t.Fatal("legacy adopted")
	}
	if f.v.saves != 0 {
		t.Fatal("legacy credential write")
	}
}
