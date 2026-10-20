package managed

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckedCopyRejectsSourceMutationAndCancellation(t *testing.T) {
	for _, mode := range []string{"source-change", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			source, dest := filepath.Join(root, "source"), filepath.Join(root, "core", "new")
			os.WriteFile(source, []byte("preflight bytes"), 0600)
			expected, e := hashFile(source)
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e = stageCopyChecked(ctx, source, dest, expected, func() {
				if mode == "cancel" {
					cancel()
				} else {
					os.WriteFile(source, []byte("changed after preflight"), 0600)
				}
			})
			if e == nil || (mode == "cancel" && !errors.Is(e, context.Canceled)) {
				t.Fatal("copy accepted change/cancel", e)
			}
			if _, e = os.Stat(dest); !os.IsNotExist(e) {
				t.Fatal("unverified destination published")
			}
			entries, _ := os.ReadDir(filepath.Dir(dest))
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".core-stage-") {
					t.Fatal("returned failure leaked own stage")
				}
			}
		})
	}
}

func TestCheckedCopyAbruptExitDoesNotPublishAndResumes(t *testing.T) {
	if dest := os.Getenv("XPOTATO_CHECKED_COPY_FIXTURE"); dest != "" {
		source, _ := os.Executable()
		expected, e := hashFile(source)
		if e != nil {
			os.Exit(2)
		}
		e = stageCopyChecked(context.Background(), source, dest, expected, func() {
			entries, _ := os.ReadDir(filepath.Dir(dest))
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".core-stage-") {
					os.WriteFile(filepath.Join(filepath.Dir(dest), entry.Name()), []byte("partial synthetic copy"), 0600)
				}
			}
			os.WriteFile(dest+".ready", []byte("ready"), 0600)
			for {
				time.Sleep(time.Second)
			}
		})
		if e != nil {
			os.Exit(3)
		}
		os.Exit(0)
	}
	dest := filepath.Join(t.TempDir(), "core", "new")
	source, _ := os.Executable()
	cmd := exec.Command(source, "-test.run=^TestCheckedCopyAbruptExitDoesNotPublishAndResumes$")
	cmd.Env = append(os.Environ(), "XPOTATO_CHECKED_COPY_FIXTURE="+dest)
	if e := cmd.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, e := os.Stat(dest + ".ready"); e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("copy fixture not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, e := os.Stat(dest); !os.IsNotExist(e) {
		t.Fatal("partial core published")
	}
	cmd.Process.Kill()
	cmd.Wait()
	expected, e := hashFile(source)
	if e != nil {
		t.Fatal(e)
	}
	if e = stageCopyChecked(context.Background(), source, dest, expected, nil); e != nil {
		t.Fatal("copy recovery failed", e)
	}
	if got, e := hashFile(dest); e != nil || got != expected {
		t.Fatal("recovered copy differs", e)
	}
}
