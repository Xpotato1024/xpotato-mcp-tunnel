package lockfile

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveLockPreservedAndOwnerChecked(t *testing.T) {
	p := filepath.Join(t.TempDir(), "operation.lock")
	release, e := Acquire(p, "fixture-owner")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Acquire(p, "fixture-owner"); !errors.Is(e, ErrBusy) {
		t.Fatal(e)
	}
	active, e := Active(p)
	if e != nil || !active {
		t.Fatal("live lock not active")
	}
	release()
	if _, e = Acquire(p, "other-owner"); !errors.Is(e, ErrOwner) {
		t.Fatal("foreign record overwritten")
	}
	release, e = Acquire(p, "fixture-owner")
	if e != nil {
		t.Fatal(e)
	}
	release()
	active, e = Active(p)
	if e != nil || active {
		t.Fatal("retained file falsely active")
	}
}
func TestLegacyLockNeverDeleted(t *testing.T) {
	p := filepath.Join(t.TempDir(), "legacy.lock")
	original := []byte(`{"launcherPid":123}`)
	os.WriteFile(p, original, 0600)
	if _, e := Acquire(p, "fixture-owner"); !errors.Is(e, ErrOwner) {
		t.Fatal("legacy adopted")
	}
	after, _ := os.ReadFile(p)
	if string(after) != string(original) {
		t.Fatal("legacy file changed")
	}
}
func TestAbruptProcessExitReleasesKernelLock(t *testing.T) {
	if p := os.Getenv("XPOTATO_LOCK_FIXTURE"); p != "" {
		release, e := Acquire(p, "fixture-owner")
		if e != nil {
			os.Exit(2)
		}
		defer release()
		os.WriteFile(p+".ready", nil, 0600)
		for {
			time.Sleep(time.Second)
		}
	}
	p := filepath.Join(t.TempDir(), "interrupted.lock")
	exe, _ := os.Executable()
	cmd := exec.Command(exe, "-test.run=^TestAbruptProcessExitReleasesKernelLock$")
	cmd.Env = append(os.Environ(), "XPOTATO_LOCK_FIXTURE="+p)
	if e := cmd.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, e := os.Stat(p + ".ready"); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, e := Acquire(p, "fixture-owner"); !errors.Is(e, ErrBusy) {
		t.Fatal("live helper lock removed")
	}
	if e := cmd.Process.Kill(); e != nil {
		t.Fatal(e)
	}
	cmd.Wait()
	var release func()
	var e error
	for {
		release, e = Acquire(p, "fixture-owner")
		if e == nil {
			break
		}
		if !errors.Is(e, ErrBusy) || time.Now().After(deadline) {
			t.Fatal("terminated helper still prevents restart")
		}
		time.Sleep(10 * time.Millisecond)
	}
	release()
}
