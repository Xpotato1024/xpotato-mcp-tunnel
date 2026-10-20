package atomicfile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

var oldGeneration = []byte(`{"core":"old","runtime":"old"}`)
var newGeneration = []byte(`{"core":"new","runtime":"new"}`)

func TestPublicationFaultsAndCancellationKeepCompleteOldFile(t *testing.T) {
	for _, step := range []string{"before-create", "before-write", "before-sync", "before-replace"} {
		for _, cancelled := range []bool{false, true} {
			t.Run(step+map[bool]string{true: "-cancel", false: "-error"}[cancelled], func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "device.json")
				os.WriteFile(path, oldGeneration, 0600)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				err := write(ctx, path, newGeneration, 0600, func(s string) error {
					if s == step {
						if cancelled {
							cancel()
							return ctx.Err()
						}
						return errors.New("fixture storage failure")
					}
					return nil
				})
				if err == nil {
					t.Fatal("fault ignored")
				}
				got, _ := os.ReadFile(path)
				if string(got) != string(oldGeneration) {
					t.Fatal("partial/new generation visible before commit")
				}
				if err = Write(context.Background(), path, newGeneration, 0600); err != nil {
					t.Fatal("retry failed", err)
				}
			})
		}
	}
}

func TestAbruptExitPublishesOnlyCompleteOldOrNewGeneration(t *testing.T) {
	if path := os.Getenv("XPOTATO_ATOMIC_FIXTURE"); path != "" {
		step := os.Getenv("XPOTATO_ATOMIC_STEP")
		err := write(context.Background(), path, newGeneration, 0600, func(s string) error {
			if s == step {
				os.WriteFile(path+".ready", []byte("ready"), 0600)
				for {
					time.Sleep(time.Second)
				}
			}
			return nil
		})
		if err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	for _, step := range []string{"before-write", "before-sync", "before-replace", "after-replace"} {
		t.Run(step, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "device.json")
			os.WriteFile(path, oldGeneration, 0600)
			exe, _ := os.Executable()
			cmd := exec.Command(exe, "-test.run=^TestAbruptExitPublishesOnlyCompleteOldOrNewGeneration$")
			cmd.Env = append(os.Environ(), "XPOTATO_ATOMIC_FIXTURE="+path, "XPOTATO_ATOMIC_STEP="+step)
			if e := cmd.Start(); e != nil {
				t.Fatal(e)
			}
			defer func() { cmd.Process.Kill(); cmd.Wait() }()
			deadline := time.Now().Add(10 * time.Second)
			for {
				if _, e := os.Stat(path + ".ready"); e == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("fixture not ready")
				}
				time.Sleep(10 * time.Millisecond)
			}
			cmd.Process.Kill()
			cmd.Wait()
			got, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			var generation map[string]string
			if json.Unmarshal(got, &generation) != nil || generation["core"] != generation["runtime"] {
				t.Fatal("torn generation", string(got))
			}
			want := oldGeneration
			if step == "after-replace" {
				want = newGeneration
			}
			if string(got) != string(want) {
				t.Fatal("wrong side of commit")
			}
			// Orphan .write files from a killed process are never adopted.
			if e = Write(context.Background(), path, newGeneration, 0600); e != nil {
				t.Fatal("recovery failed", e)
			}
		})
	}
}

func TestCancellationAfterPublicationIsCommittedSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "device.json")
	os.WriteFile(path, oldGeneration, 0600)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := write(ctx, path, newGeneration, 0600, func(step string) error {
		if step == "after-replace" {
			cancel()
		}
		return nil
	})
	if err != nil {
		t.Fatal("committed write reported failure", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(newGeneration) {
		t.Fatal("new generation missing")
	}
}
