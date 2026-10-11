package core

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type wizardPrompt struct {
	lines  []string
	labels []string
}

func (p *wizardPrompt) Text(label string) (string, error) {
	p.labels = append(p.labels, label)
	if len(p.lines) == 0 {
		return "", errors.New("unexpected prompt: " + label)
	}
	s := p.lines[0]
	p.lines = p.lines[1:]
	return s, nil
}
func (*wizardPrompt) Key() ([]byte, error) { panic("wizard must never request a key") }
func (*wizardPrompt) Consent(string) (bool, error) {
	panic("wizard must never request credential consent")
}

func preview(t *testing.T, f fixture, lines ...string) (Options, string, error) {
	t.Helper()
	p := &wizardPrompt{lines: lines}
	f.app.Prompt = p
	var out bytes.Buffer
	o, e := f.app.PrepareSetup(f.o, &out, filepath.Join(t.TempDir(), "user bin"))
	if e == nil && len(p.lines) != 0 {
		t.Fatalf("unconsumed input: %v", p.lines)
	}
	return o, out.String(), e
}

func TestWizardNewSetupMultipleDirectories(t *testing.T) {
	f := newFixture(t)
	f.o.Device, f.o.Tunnel, f.o.Allow = "", "", nil
	first, second, third := t.TempDir(), t.TempDir(), t.TempDir()
	o, out, e := preview(t, f, "work-pc", "tunnel_example", "add", first, "add", second, "review", "remove", "9", "remove", "1", "add", third, "done", "yes")
	if e != nil {
		t.Fatal(e)
	}
	if o.Device != "work-pc" || o.Tunnel != "tunnel_example" || !reflect.DeepEqual(o.Allow, []string{second, third}) {
		t.Fatal("wrong reviewed options", o)
	}
	for _, want := range []string{"API キーとは別物", "保存前の確認", "恒久保存先", "Go 本体", "Commander 設定", second, third, "100 行", "50 行", "Secret Service", "Credential Manager"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if _, e := os.Stat(f.o.Home); !os.IsNotExist(e) {
		t.Fatal("preview wrote state")
	}
	if f.v.ready != 0 || f.v.loads != 0 || f.v.saves != 0 || f.r.installs != 0 {
		t.Fatal("preview touched runtime or vault")
	}
}

func TestWizardInvalidInputsAndCorrection(t *testing.T) {
	f := newFixture(t)
	f.o.Device, f.o.Tunnel, f.o.Allow = "INVALID!", "invalid", nil
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	os.WriteFile(file, []byte("fixture"), 0600)
	o, out, e := preview(t, f, "bad name", "work-pc", "synthetic-secret-not-an-id", "tunnel_first", "done", "unknown", "add", "relative", "add", filepath.Join(root, "missing"), "add", file, "add", "\""+root+"\"", "add", root, "done", "", "tunnel", "tunnel_corrected", "device", "corrected-pc", "home", "relative", f.o.Home, "command", "relative", "", "yes")
	if e != nil {
		t.Fatal(e)
	}
	if o.Device != "corrected-pc" || o.Tunnel != "tunnel_corrected" || !reflect.DeepEqual(o.Allow, []string{root}) || o.Home != f.o.Home || o.CommandDir != "" {
		t.Fatal("correction lost", o)
	}
	if strings.Contains(out, "synthetic-secret-not-an-id") {
		t.Fatal("invalid identifier echoed")
	}
	for _, want := range []string{"形式が違います", "追加していません", "追加済み", "一つ以上必要", "選択肢"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing retry guidance %q", want)
		}
	}
}

func TestWizardEditDirectoriesFromSummary(t *testing.T) {
	f := newFixture(t)
	root := t.TempDir()
	o, _, e := preview(t, f, "done", "dirs", "remove", "1", "add", root, "done", "yes")
	if e != nil || !reflect.DeepEqual(o.Allow, []string{root}) {
		t.Fatal("summary editing failed", e, o)
	}
}

func TestWizardInvalidFlagRootCanBeRemoved(t *testing.T) {
	f := newFixture(t)
	f.o.Allow = []string{"relative"}
	root := t.TempDir()
	o, _, e := preview(t, f, "done", "remove", "1", "add", root, "done", "yes")
	if e != nil || !reflect.DeepEqual(o.Allow, []string{root}) {
		t.Fatal("flag correction failed", e)
	}
}

func TestWizardCancellationBeforeAnyMutation(t *testing.T) {
	for _, tc := range []struct {
		name           string
		device, tunnel string
		lines          []string
	}{
		{"device", "", "", []string{"q"}},
		{"tunnel", "work-pc", "", []string{"cancel"}},
		{"root-menu", "work-pc", "tunnel_fixture", []string{"中断"}},
		{"root-input", "work-pc", "tunnel_fixture", []string{"add", "q"}},
		{"root-remove", "work-pc", "tunnel_fixture", []string{"remove", "q"}},
		{"summary", "work-pc", "tunnel_fixture", []string{"done", "no"}},
		{"home", "work-pc", "tunnel_fixture", []string{"done", "home", "q"}},
		{"command", "work-pc", "tunnel_fixture", []string{"done", "command", "q"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.o.Device, f.o.Tunnel = tc.device, tc.tunnel
			_, _, e := preview(t, f, tc.lines...)
			if !errors.Is(e, ErrSetupCancelled) {
				t.Fatal("unclear cancellation", e)
			}
			if _, e := os.Stat(f.o.Home); !os.IsNotExist(e) {
				t.Fatal("cancel wrote files")
			}
			if f.v.ready != 0 || f.r.installs != 0 {
				t.Fatal("cancel performed setup")
			}
		})
	}
}

func TestWizardResumePreservesExistingScopeAndConfig(t *testing.T) {
	f := prepared(t)
	before, _ := os.ReadFile(configFile(f.o))
	o, out, e := preview(t, f, "yes")
	if e != nil {
		t.Fatal(e)
	}
	if o.Tunnel != "tunnel_fixture" || len(o.Allow) != 1 || !strings.Contains(out, "既存の端末") {
		t.Fatal("existing scope changed")
	}
	if _, e := f.app.Setup(o); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(configFile(f.o))
	if !bytes.Equal(before, after) || f.v.saves != 1 || f.v.loads != 0 || f.p.keys != 1 {
		t.Fatal("resume mutated config or credential")
	}
}

type versionedRuntime struct {
	*fakeRuntime
	rejectNew bool
}

func (r versionedRuntime) Verify(home, id string) error {
	if id == "legacy-release" {
		return ErrRuntimePinUnsupported
	}
	if r.rejectNew {
		return errors.New("fixture verification failure")
	}
	return r.fakeRuntime.Verify(home, id)
}

func legacyPartialFixture(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t)
	f.p.yes = false
	if _, e := f.app.Setup(f.o); e == nil {
		t.Fatal("missing fixture decline")
	}
	var d Device
	readJSON(deviceFile(f.o), &d)
	d.Release = "legacy-release"
	atomicJSON(deviceFile(f.o), d)
	f.r.next = "current-release"
	f.app.Runtime = versionedRuntime{fakeRuntime: f.r}
	f.p.yes = true
	return f
}

func TestWizardOldCoreResumePlansExplicitVerifiedRefresh(t *testing.T) {
	f := legacyPartialFixture(t)
	config, _ := os.ReadFile(configFile(f.o))
	o, out, e := preview(t, f, "yes")
	if e != nil || !o.RefreshRuntime || !strings.Contains(out, "実行環境の更新") || f.r.installs != 1 || f.v.saves != 0 {
		t.Fatal("refresh not reviewed before mutations", e)
	}
	if _, e := f.app.Setup(o); e != nil {
		t.Fatal(e)
	}
	var d Device
	readJSON(deviceFile(f.o), &d)
	after, _ := os.ReadFile(configFile(f.o))
	if d.Release != "current-release" || d.Previous != "legacy-release" || f.r.installs != 2 || f.r.runs != 0 || f.v.saves != 1 || !bytes.Equal(config, after) {
		t.Fatal("refresh changed existing settings or runtime process")
	}
}

func TestWizardOldCoreCancelAndFailedRefreshPreservePin(t *testing.T) {
	for _, mode := range []string{"cancel", "verify-failure"} {
		t.Run(mode, func(t *testing.T) {
			f := legacyPartialFixture(t)
			before, _ := os.ReadFile(deviceFile(f.o))
			if mode == "cancel" {
				if _, _, e := preview(t, f, "no"); !errors.Is(e, ErrSetupCancelled) {
					t.Fatal(e)
				}
				if f.r.installs != 1 {
					t.Fatal("cancel installed new runtime")
				}
			} else {
				o, _, e := preview(t, f, "yes")
				if e != nil {
					t.Fatal(e)
				}
				f.app.Runtime = versionedRuntime{fakeRuntime: f.r, rejectNew: true}
				if _, e = f.app.Setup(o); e == nil {
					t.Fatal("unverified refresh accepted")
				}
			}
			after, _ := os.ReadFile(deviceFile(f.o))
			if !bytes.Equal(before, after) || f.v.saves != 0 || f.p.keys != 0 {
				t.Fatal("cancel/failure changed pin or saved key")
			}
		})
	}
}

func TestWizardCorrectsConflictingShortCommandBeforeConfirmation(t *testing.T) {
	f := newFixture(t)
	selected, changed := t.TempDir(), t.TempDir()
	f.o.CommandDir = changed
	f.app.PreviewCommand = func(_ string, requested string) (CommandPlan, error) {
		if requested != "" && requested != selected {
			return CommandPlan{}, ErrCommandDirectoryConflict
		}
		return CommandPlan{Directory: selected}, nil
	}
	o, out, e := preview(t, f, "done", "command", changed, "yes")
	if e != nil || o.CommandDir != "" {
		t.Fatal("conflict not corrected", e)
	}
	summary := out[strings.LastIndex(out, "保存前の確認"):]
	if !strings.Contains(summary, selected) || strings.Contains(summary, changed) || !strings.Contains(summary, "選択・同意済み") {
		t.Fatal("wrong command destination confirmed")
	}
	if f.v.ready != 0 || f.r.installs != 0 {
		t.Fatal("conflict check happened after setup")
	}
}

func TestSetupCancelAfterKeySaveKeepsCredentialAndResumes(t *testing.T) {
	f := newFixture(t)
	f.app.InstallCommand = func(string) error { return ErrSetupCancelled }
	if _, e := f.app.Setup(f.o); !errors.Is(e, ErrSetupCancelled) {
		t.Fatal(e)
	}
	var d Device
	readJSON(deviceFile(f.o), &d)
	if !d.CredentialReady || !d.ConfigReady || f.v.saves != 1 || exists(filepath.Join(f.o.Home, "default.json")) {
		t.Fatal("unexpected partial state")
	}
	f.app.InstallCommand = func(string) error { return nil }
	if _, e := f.app.Setup(f.o); e != nil {
		t.Fatal(e)
	}
	if f.v.saves != 1 || f.p.keys != 1 || f.p.consents != 1 || !exists(filepath.Join(f.o.Home, "default.json")) {
		t.Fatal("resume requested saved key again")
	}
}

func TestRefreshCommandFailureKeepsCompletedDevicePin(t *testing.T) {
	f := prepared(t)
	var d Device
	readJSON(deviceFile(f.o), &d)
	d.Release = "legacy-release"
	atomicJSON(deviceFile(f.o), d)
	before, _ := os.ReadFile(deviceFile(f.o))
	f.r.next = "current-release"
	f.app.Runtime = versionedRuntime{fakeRuntime: f.r}
	o, _, e := preview(t, f, "yes")
	if e != nil {
		t.Fatal(e)
	}
	f.app.InstallCommand = func(string) error { return errors.New("fixture command install failure") }
	if _, e = f.app.Setup(o); e == nil {
		t.Fatal("command installation failure ignored")
	}
	after, _ := os.ReadFile(deviceFile(f.o))
	if !bytes.Equal(before, after) || f.v.saves != 1 || f.p.keys != 1 {
		t.Fatal("failed core installation changed the pin or credential")
	}
	f.app.InstallCommand = func(string) error { return nil }
	if _, e = f.app.Setup(o); e != nil {
		t.Fatal(e)
	}
	readJSON(deviceFile(f.o), &d)
	if d.Release != "current-release" || d.Previous != "legacy-release" || f.v.saves != 1 {
		t.Fatal("retry did not commit pin after command installation")
	}
}
