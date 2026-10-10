package core

import (
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/lockfile"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

type interruptRuntime struct {
	fakeRuntime
	phase, ready string
}

func (r *interruptRuntime) Install(home string) (string, error) {
	release, e := lockfile.Acquire(filepath.Join(home, "releases", "install.lock"), "fixture-owner")
	if e != nil {
		return "", e
	}
	defer release()
	if r.phase == "download" {
		os.WriteFile(r.ready, nil, 0600)
		for {
			time.Sleep(time.Second)
		}
	}
	return r.fakeRuntime.Install(home)
}

type interruptPrompt struct {
	fakePrompt
	phase, ready string
}

func (p *interruptPrompt) Key() ([]byte, error) {
	if p.phase == "key-input" {
		os.WriteFile(p.ready, nil, 0600)
		for {
			time.Sleep(time.Second)
		}
	}
	return p.fakePrompt.Key()
}
func TestSetupResumesAfterActualProcessTermination(t *testing.T) {
	if home := os.Getenv("XPOTATO_SETUP_FIXTURE"); home != "" {
		phase := os.Getenv("XPOTATO_SETUP_PHASE")
		p := &interruptPrompt{fakePrompt: fakePrompt{yes: true}, phase: phase, ready: filepath.Join(home, "ready")}
		r := &interruptRuntime{phase: phase, ready: filepath.Join(home, "ready")}
		a := App{Identity: func() (platform.Identity, error) { return platform.Identity{Owner: "fixture-owner"}, nil }, Vault: &fakeVault{}, Prompt: p, Runtime: r}
		_, e := a.Setup(Options{Home: home, Device: "fixture", Tunnel: "tunnel_fixture", Allow: []string{home}})
		if e != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	for _, phase := range []string{"download", "key-input"} {
		t.Run(phase, func(t *testing.T) {
			home := t.TempDir()
			exe, _ := os.Executable()
			cmd := exec.Command(exe, "-test.run=^TestSetupResumesAfterActualProcessTermination$")
			cmd.Env = append(os.Environ(), "XPOTATO_SETUP_FIXTURE="+home, "XPOTATO_SETUP_PHASE="+phase)
			if e := cmd.Start(); e != nil {
				t.Fatal(e)
			}
			defer func() { cmd.Process.Kill(); cmd.Wait() }()
			deadline := time.Now().Add(10 * time.Second)
			for !exists(filepath.Join(home, "ready")) {
				if time.Now().After(deadline) {
					t.Fatal("setup helper not ready")
				}
				time.Sleep(10 * time.Millisecond)
			}
			f := newFixture(t)
			f.o = Options{Home: home, Device: "fixture", Tunnel: "tunnel_fixture", Allow: []string{home}}
			if _, e := f.app.Setup(f.o); e == nil {
				t.Fatal("another live setup lock was removed")
			}
			if f.v.saves != 0 {
				t.Fatal("live setup collision saved credential")
			}
			cmd.Process.Kill()
			cmd.Wait()
			if _, e := f.app.Setup(f.o); e != nil {
				t.Fatal(e)
			}
			if f.v.saves != 1 {
				t.Fatal("terminated setup failed to resume")
			}
		})
	}
}
