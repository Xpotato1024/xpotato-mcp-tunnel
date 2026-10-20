package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/managed"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
)

// Only copied Go test executables in new fixture homes enter this branch.
func init() {
	if os.Getenv("XPOTATO_DISPATCH_FIXTURE") != "1" || len(os.Args) < 2 {
		return
	}
	if os.Args[1] == "_dispatch" {
		code, e := run(os.Args[1:])
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
		}
		os.Exit(code)
	}
	if os.Args[1] == "version" {
		fmt.Println(strings.Join(os.Args, "|"))
		os.Exit(42)
	}
	if os.Args[1] == "setup" {
		_, stop := operationContext("setup")
		defer stop()
		text, e := (terminal{}).Text("dispatch input fixture")
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(2)
		}
		fmt.Println("TEXT_OK|" + text)
		os.Exit(42)
	}
}

type dispatchPrompt struct{}

func (dispatchPrompt) Text(string) (string, error) { return "", errors.New("unexpected prompt") }
func (dispatchPrompt) Key() ([]byte, error) {
	return nil, errors.New("fixture cannot access credentials")
}
func (dispatchPrompt) Consent(string) (bool, error) { return false, nil }

func TestStableWrappersDispatchSelectedGenerationAndPreserveArguments(t *testing.T) {
	home := filepath.Join(t.TempDir(), "Fixture Home With Spaces")
	if e := managed.SetupCommand(home, "", dispatchPrompt{}); e != nil {
		t.Fatal(e)
	}
	p, e := managed.PrepareCommandUpdate(context.Background(), home)
	if e != nil {
		t.Fatal(e)
	}
	if e = p.Migrate(); e != nil {
		t.Fatal(e)
	}
	pin := p.Current()
	p.Close()
	pin.Version = "0.2.6"
	name := "xpotato-tunnel"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	selected := filepath.Join(home, "core", pin.Version, name)
	os.MkdirAll(filepath.Dir(selected), 0700)
	source, _ := os.Executable()
	b, e := os.ReadFile(source)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(selected, b, 0700); e != nil {
		t.Fatal(e)
	}
	i, e := platform.CurrentIdentity()
	if e != nil {
		t.Fatal(e)
	}
	d := core.Device{Schema: 3, Name: "fixture", Owner: i.Owner, Core: &pin}
	dir := filepath.Join(home, "devices", d.Name)
	os.MkdirAll(dir, 0700)
	b, _ = json.Marshal(d)
	os.WriteFile(filepath.Join(dir, "device.json"), b, 0600)
	b, _ = json.Marshal(map[string]string{"device": d.Name})
	os.WriteFile(filepath.Join(home, "default.json"), b, 0600)
	kinds := []string{"sh"}
	if runtime.GOOS == "windows" {
		kinds = []string{"ps1", "cmd"}
	}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			var cmd *exec.Cmd
			wrapper := filepath.Join(home, "bin", "xpotato-tunnel")
			switch kind {
			case "ps1":
				cmd = exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-File", wrapper+".ps1", "version", "two words")
			case "cmd":
				cmd = dispatchFixtureCMD(wrapper + ".cmd")
			default:
				cmd = exec.Command(wrapper, "version", "two words")
			}
			cmd.Env = append(os.Environ(), "XPOTATO_DISPATCH_FIXTURE=1")
			out, e := cmd.CombinedOutput()
			exit, ok := e.(*exec.ExitError)
			if !ok || exit.ExitCode() != 42 || !strings.Contains(string(out), selected+"|version|two words|--home|"+home) {
				t.Fatalf("dispatch argument/exit/core mismatch: %v; %s", e, out)
			}
		})
	}
}

func TestDispatchArgumentsPreserveForwardingAndUseBoundHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "Home With Spaces")
	args := []string{"start", "--device", "fixture", "--diagnostics", "--home", home}
	gotHome, gotArgs, e := dispatchArguments(args)
	if e != nil || gotHome != home || !reflect.DeepEqual(gotArgs, args) {
		t.Fatal("dispatch changed native args", e)
	}
	for _, bad := range [][]string{nil, {"version"}, {"start", "--home", "relative"}, {"start", "--home", home, "extra"}} {
		if _, _, e = dispatchArguments(bad); e == nil {
			t.Fatal("unbound dispatch accepted", bad)
		}
	}
}

func TestCancelledDispatchDoesNotStartAnyChild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	home := t.TempDir()
	if _, e := dispatch(ctx, []string{"version", "--home", home}); e != context.Canceled {
		t.Fatal("dispatch cancellation lost", e)
	}
	entries, _ := os.ReadDir(home)
	if len(entries) != 0 {
		t.Fatal("dispatch created installation")
	}
}
