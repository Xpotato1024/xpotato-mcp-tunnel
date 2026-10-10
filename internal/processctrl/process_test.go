package processctrl

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func waitFile(t *testing.T, p string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if b, e := os.ReadFile(p); e == nil && len(b) > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("owned fixture did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func TestOwnedTreeFixture(t *testing.T) {
	mode := os.Getenv("XPOTATO_TREE_MODE")
	if mode == "" {
		return
	}
	path := os.Getenv("XPOTATO_TREE_HEARTBEAT")
	if mode == "leaf" {
		signal.Ignore(os.Interrupt, syscall.SIGTERM)
		for i := 0; ; i++ {
			os.WriteFile(path, []byte(strconv.Itoa(i)), 0600)
			time.Sleep(20 * time.Millisecond)
		}
	}
	exe, _ := os.Executable()
	child := exec.Command(exe, "-test.run=^TestOwnedTreeFixture$")
	child.Env = append(os.Environ(), "XPOTATO_TREE_MODE=leaf")
	if child.Start() != nil {
		os.Exit(2)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, e := os.Stat(path); e == nil {
			break
		}
		if time.Now().After(deadline) {
			os.Exit(3)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if mode == "parent-exit" {
		os.Exit(0)
	}
	signal.Ignore(os.Interrupt, syscall.SIGTERM)
	for {
		time.Sleep(time.Second)
	}
}
func TestOwnedDescendantsReapedOnInterruptAndParentExit(t *testing.T) {
	for _, mode := range []string{"parent-wait", "parent-exit"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			heartbeat := filepath.Join(root, "owned")
			sentinel := filepath.Join(root, "unrelated")
			exe, _ := os.Executable()
			other := exec.Command(exe, "-test.run=^TestOwnedTreeFixture$")
			other.Env = append(os.Environ(), "XPOTATO_TREE_MODE=leaf", "XPOTATO_TREE_HEARTBEAT="+sentinel)
			if e := other.Start(); e != nil {
				t.Fatal(e)
			}
			defer func() { other.Process.Kill(); other.Wait() }()
			waitFile(t, sentinel)
			cmd := exec.Command(exe, "-test.run=^TestOwnedTreeFixture$")
			cmd.Env = append(os.Environ(), "XPOTATO_TREE_MODE="+mode, "XPOTATO_TREE_HEARTBEAT="+heartbeat)
			signals := make(chan os.Signal, 1)
			done := make(chan error, 1)
			go func() { _, e := Run(cmd, signals, nil, 200*time.Millisecond); done <- e }()
			waitFile(t, heartbeat)
			if mode == "parent-wait" {
				signals <- os.Interrupt
			}
			select {
			case e := <-done:
				if e != nil {
					t.Fatal(e)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("bounded cleanup did not finish")
			}
			first, _ := os.ReadFile(heartbeat)
			otherFirst, _ := os.ReadFile(sentinel)
			time.Sleep(150 * time.Millisecond)
			last, _ := os.ReadFile(heartbeat)
			otherLast, _ := os.ReadFile(sentinel)
			if string(first) != string(last) {
				t.Fatal("owned grandchild remained running")
			}
			if string(otherFirst) == string(otherLast) {
				t.Fatal("unrelated live process was affected")
			}
		})
	}
}
