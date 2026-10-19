package managed

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
)

// The pinned runtime has no validate-only command. This invalid Harpoon limit is
// checked AFTER logging by LoadFromFlagSet, BEFORE runtimeapp.New / any network
// or MCP process starts. Never remove this guard from an offline native probe.
const offlineConfigGuard = "--harpoon.max-response-bytes=0"
const offlineConfigError = "configure tunnel-client runtime: harpoon.max-response-bytes must be positive"

func init() {
	if len(os.Args) < 2 || os.Args[1] != "run" {
		return
	}
	profile := os.Getenv("XPOTATO_MCP_PROFILE")
	if _, err := os.Stat(filepath.Join(profile, ".startup-config-fixture")); err != nil {
		return
	}
	// Manager.Run launched this test-only shim with its real args, environment,
	// working directory and owned-process machinery. Forward them unchanged to
	// the official binary, adding only the pre-network validation stop above.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	native := filepath.Join(filepath.Dir(os.Args[0]), "probe-"+executable())
	args := append(append([]string{}, os.Args[1:]...), offlineConfigGuard)
	cmd := exec.CommandContext(ctx, native, args...)
	output, err := cmd.CombinedOutput()
	if err == nil || strings.TrimSpace(string(output)) != offlineConfigError {
		fmt.Fprintln(os.Stderr, "offline runtime configuration rejected:", strings.TrimSpace(string(output)))
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "offline runtime startup configuration accepted (stopped before network/MCP)")
	os.Exit(0)
}

func TestPinnedRuntimeStartupConfiguration(t *testing.T) {
	native := os.Getenv("XPOTATO_TEST_TUNNEL_RUNTIME")
	if native == "" {
		t.Skip("set XPOTATO_TEST_TUNNEL_RUNTIME to the pinned official binary; no key/network required")
	}
	testPinnedRuntimeStartupConfiguration(t, native)
}

func testPinnedRuntimeStartupConfiguration(t *testing.T, native string) {
	t.Helper()
	m, err := manifest()
	if err != nil {
		t.Fatal(err)
	}
	if hash, err := hashFile(native); err != nil || hash != m.ExecutableSHA {
		t.Fatal("offline probe requires the SHA256-pinned official runtime")
	}
	for _, diagnostics := range []bool{false, true} {
		t.Run(fmt.Sprintf("Manager.Run/diagnostics=%v", diagnostics), func(t *testing.T) {
			home := t.TempDir()
			d := core.Device{Name: "fixture", Tunnel: "tunnel_0123456789abcdef0123456789abcdef", Release: "fixture"}
			release := filepath.Join(home, "releases", d.Release)
			profile := core.ProfilePath(home, d)
			if err := os.MkdirAll(release, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(profile, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(profile, ".startup-config-fixture"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			shim, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			for dest, source := range map[string]string{executable(): shim, "probe-" + executable(): native} {
				if err := stageCopy(source, filepath.Join(release, dest), nil); err != nil {
					t.Fatal(err)
				}
			}
			key := []byte("synthetic-offline-startup-key")
			code, err := (Manager{Diagnostics: diagnostics}).Run(home, d, key)
			if err != nil || code != 0 {
				t.Fatalf("real launch configuration did not reach pre-network guard: exit=%d error=%v", code, err)
			}
			for _, b := range key {
				if b != 0 {
					t.Fatal("launch key buffer was not cleared")
				}
			}
		})
	}
	// Keep the incident and the supported v0.2.8 diagnostic workaround covered
	// by the actual fixed-version parser, rather than duplicating its rules.
	for _, diagnostics := range []bool{false, true} {
		t.Run(fmt.Sprintf("v0.2.8/diagnostics=%v", diagnostics), func(t *testing.T) {
			run := t.TempDir()
			args := runtimeArguments(core.Device{Tunnel: "tunnel_0123456789abcdef0123456789abcdef"}, run, "unused-offline-command", diagnostics)
			for i, arg := range args {
				if arg == "--log.format" {
					args[i+1] = "" // Historical v0.2.8 format, with its real warn/info levels.
				}
			}
			args = append(args, offlineConfigGuard)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, native, args...)
			cmd.Dir = run
			cmd.Env = runtimeEnvironment(os.Environ(), run, run, "unused-offline-entry", []byte("synthetic-offline-startup-key"))
			output, err := cmd.CombinedOutput()
			want := "configure tunnel-client runtime: log level requires 'struct-text' or 'json' log format"
			if diagnostics {
				want = offlineConfigError
			}
			if err == nil || strings.TrimSpace(string(output)) != want {
				t.Fatalf("historical configuration: expected %q, received %q (error %v)", want, output, err)
			}
		})
	}
}
