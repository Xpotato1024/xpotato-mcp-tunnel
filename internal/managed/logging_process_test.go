package managed

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
)

func init() {
	if len(os.Args) < 2 || os.Args[1] != "run" {
		return
	}
	profile := os.Getenv("XPOTATO_MCP_PROFILE")
	if _, err := os.Stat(filepath.Join(profile, ".stderr-fixture")); err != nil {
		return
	}
	fmt.Fprintln(os.Stderr, "WARN fixture key="+os.Getenv("CONTROL_PLANE_API_KEY"))
	exe, _ := os.Executable()
	leaf := exec.Command(exe, "-test.run=^TestStderrOwnedLeaf$")
	leaf.Env = append(os.Environ(), "XPOTATO_STDERR_LEAF=1")
	leaf.Stdout, leaf.Stderr = os.Stdout, os.Stderr
	if err := leaf.Start(); err != nil {
		os.Exit(3)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(profile, "leaf-ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			os.Exit(4)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Structured upstream logging uses the runtime's stdout by default. This
	// stream is not the private runtime-to-MCP stdio pipe.
	fmt.Fprintln(os.Stdout, "WARN structured fixture key="+os.Getenv("CONTROL_PLANE_API_KEY"))
	os.Exit(0) // Descendant intentionally retains stderr; owned cleanup must finish.
}

func TestStderrOwnedLeaf(t *testing.T) {
	if os.Getenv("XPOTATO_STDERR_LEAF") != "1" {
		return
	}
	os.WriteFile(filepath.Join(os.Getenv("XPOTATO_MCP_PROFILE"), "leaf-ready"), []byte("synthetic"), 0600)
	time.Sleep(30 * time.Second)
}

func TestManagedLogPipesCannotHoldOwnedCleanupOpen(t *testing.T) {
	home := t.TempDir()
	release := filepath.Join(home, "releases", "fixture")
	profile := filepath.Join(home, "devices", "fixture", "profile")
	os.MkdirAll(release, 0700)
	os.MkdirAll(profile, 0700)
	os.WriteFile(filepath.Join(profile, ".stderr-fixture"), nil, 0600)
	exe, _ := os.Executable()
	if err := stageCopy(exe, filepath.Join(release, executable()), nil); err != nil {
		t.Fatal(err)
	}
	stdout, err := os.CreateTemp(t.TempDir(), "protocol")
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	stderr, err := os.CreateTemp(t.TempDir(), "display")
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdout, stderr
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	key := []byte("synthetic-stderr-fixture-key")
	started := time.Now()
	_, err = (Manager{}).Run(home, core.Device{Name: "fixture", Tunnel: "tunnel_fixture", Release: "fixture"}, key)
	if time.Since(started) > 8*time.Second {
		t.Fatal("stderr descendant prevented cleanup")
	}
	// Windows may report the bounded pipe-drain failure; Linux's guardian can
	// finish its private group first. Either way protocol and key isolation hold.
	stdout.Seek(0, 0)
	protocol, _ := io.ReadAll(stdout)
	stderr.Seek(0, 0)
	logs, _ := io.ReadAll(stderr)
	if len(protocol) != 0 || strings.Contains(string(logs), "synthetic-stderr-fixture-key") || !strings.Contains(string(logs), "WARN fixture") || !strings.Contains(string(logs), "WARN structured fixture") {
		t.Fatal("unsafe or missing output")
	}
	for _, b := range key {
		if b != 0 {
			t.Fatal("launch buffer retained key")
		}
	}
}
