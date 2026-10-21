//go:build linux || darwin

package processctrl

import (
	"errors"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/lockfile"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestUnixLauncherFixture(t *testing.T) {
	if os.Getenv("XPOTATO_LAUNCHER_FIXTURE") != "1" {
		return
	}
	release, e := lockfile.Acquire(os.Getenv("XPOTATO_LAUNCHER_LOCK"), "synthetic-owner")
	if e != nil {
		os.Exit(2)
	}
	defer release()
	exe, _ := os.Executable()
	cmd := exec.Command(exe, "-test.run=^TestOwnedTreeFixture$")
	cmd.Env = append(os.Environ(), "XPOTATO_TREE_MODE=parent-wait")
	_, e = Run(cmd, nil, nil, time.Second)
	if e != nil {
		os.Exit(3)
	}
}

func TestLauncherDeathRetainsOwnershipUntilOwnedTreeEnds(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGKILL, syscall.SIGHUP} {
		t.Run(sig.String(), func(t *testing.T) {
			root := t.TempDir()
			beat := filepath.Join(root, "owned")
			sentinel := filepath.Join(root, "unrelated")
			lock := filepath.Join(root, "tunnel.lock")
			exe, _ := os.Executable()
			other := startUnrelatedFixture(t, exe, sentinel)
			launcher := exec.Command(exe, "-test.run=^TestUnixLauncherFixture$")
			launcher.Env = append(os.Environ(), "XPOTATO_LAUNCHER_FIXTURE=1", "XPOTATO_LAUNCHER_LOCK="+lock, "XPOTATO_TREE_HEARTBEAT="+beat)
			if e := launcher.Start(); e != nil {
				t.Fatal(e)
			}
			defer launcher.Process.Kill()
			waitFile(t, beat)
			if e := launcher.Process.Signal(sig); e != nil {
				t.Fatal(e)
			}
			launcher.Wait() // Actual process termination, not an injected Run channel.
			release, e := lockfile.Acquire(lock, "synthetic-owner")
			if e == nil {
				release()
				t.Fatal("launcher death released ownership while owned descendants were still alive")
			}
			if !errors.Is(e, lockfile.ErrBusy) {
				t.Fatal(e)
			}
			deadline := time.Now().Add(10 * time.Second)
			for {
				active, e := lockfile.Active(lock)
				if e != nil {
					t.Fatal(e)
				}
				if !active {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("guardian did not finish owned cleanup")
				}
				time.Sleep(10 * time.Millisecond)
			}
			first, _ := os.ReadFile(beat)
			otherFirst, _ := os.ReadFile(sentinel)
			time.Sleep(150 * time.Millisecond)
			last, _ := os.ReadFile(beat)
			if string(first) != string(last) {
				t.Fatal("owned grandchild survived actual launcher death")
			}
			other.assertAdvance(t, sentinel, otherFirst, deadline)
			release, e = lockfile.Acquire(lock, "synthetic-owner")
			if e != nil {
				t.Fatal(e)
			}
			release()
			t.Log("actual launcher death: owned grandchild stopped, unrelated sentinel lives, lock retained until cleanup and then reusable")
		})
	}
}
