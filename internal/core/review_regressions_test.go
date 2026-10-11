package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type notReadyPrompt struct{ fakePrompt }

func (*notReadyPrompt) Consent(string) (bool, error) { return false, nil }
func TestLegacyCutoverFlagDoesNotGateSetupOrStart(t *testing.T) {
	f := prepared(t)
	var d map[string]any
	readJSON(deviceFile(f.o), &d)
	d["cutoverReady"] = false // Older schema-3 state remains compatible.
	atomicJSON(deviceFile(f.o), d)
	f.app.Prompt = &notReadyPrompt{}
	if _, e := f.app.Setup(f.o); e != nil {
		t.Fatal(e)
	}
	if code, e := f.app.Start(f.o); e != nil || code != 42 {
		t.Fatal("legacy flag gated start", e)
	}
	if f.v.saves != 1 {
		t.Fatal("existing credential replaced")
	}
}
func TestDoctorReportsPartialSetupAndInstallLock(t *testing.T) {
	f := newFixture(t)
	f.r.bad = true
	if _, e := f.app.Setup(f.o); e == nil {
		t.Fatal("fixture failure missing")
	}
	release, e := acquire(filepath.Join(f.o.Home, "releases", "install.lock"), "fixture-owner")
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	report, e := f.app.Doctor(f.o)
	if e != nil || report["integrity"] != "not-installed" {
		t.Fatal("partial doctor failed")
	}
	if !report["locks"].(map[string]bool)["install"] {
		t.Fatal("install lock omitted")
	}
	homeReport, e := f.app.DoctorHome(f.o.Home)
	if e != nil || len(homeReport["devices"].([]string)) != 1 {
		t.Fatal("no-default doctor failed")
	}
	if f.v.loads != 0 || f.v.saves != 0 {
		t.Fatal("doctor accessed vault")
	}
}
func TestExplicitCoreUpdateFailureKeepsDevicePin(t *testing.T) {
	f := prepared(t)
	before, _ := os.ReadFile(deviceFile(f.o))
	f.r.next = "release-two"
	f.app.InstallCommand = func(string) error { return errors.New("fixture install interrupted") }
	if e := f.app.Update(f.o, false); e == nil {
		t.Fatal("command install failure ignored")
	}
	after, _ := os.ReadFile(deviceFile(f.o))
	if string(before) != string(after) {
		t.Fatal("pin changed after core install failure")
	}
	calls := 0
	f.app.InstallCommand = func(string) error { calls++; return nil }
	if e := f.app.Update(f.o, false); e != nil {
		t.Fatal(e)
	}
	if calls != 1 || f.v.loads != 0 {
		t.Fatal("explicit update did not refresh command or read key")
	}
}
