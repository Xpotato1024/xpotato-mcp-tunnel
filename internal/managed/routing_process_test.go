package managed

import (
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func init() {
	// A copy of this test executable stands in for tunnel-client. It validates
	// received arguments/env then exits; it never makes any HTTP request.
	if len(os.Args) < 2 || os.Args[1] != "run" {
		return
	}
	profile := os.Getenv("XPOTATO_MCP_PROFILE")
	if profile == "" {
		return
	}
	if _, e := os.Stat(filepath.Join(profile, ".routing-fixture")); e != nil {
		return
	}
	for _, name := range []string{"CONTROL_PLANE_BASE_URL", "CONTROL_PLANE_URL_PATH", "TUNNEL_CLIENT_CONFIG", "TUNNEL_CLIENT_PROFILE", "TUNNEL_CLIENT_PROFILE_FILE", "MCP_COMMAND", "MCP_SERVER_URL", "HTTP_PROXY", "HTTPS_PROXY", "SSL_CERT_FILE", "LOG_HTTP_RAW_UNSAFE"} {
		if os.Getenv(name) != "" {
			os.Exit(2)
		}
	}
	base := false
	for i, arg := range os.Args {
		if strings.Contains(arg, "synthetic-routing-key") {
			os.Exit(3)
		}
		if arg == "--control-plane.base-url" && i+1 < len(os.Args) && os.Args[i+1] == "https://api.openai.com" {
			base = true
		}
	}
	if !base || os.Getenv("CONTROL_PLANE_API_KEY") != "synthetic-routing-key" {
		os.Exit(4)
	}
	os.Exit(42)
}
func TestActualRuntimeChildIgnoresMaliciousAmbientRouting(t *testing.T) {
	home := t.TempDir()
	release := filepath.Join(home, "releases", "fixture")
	os.MkdirAll(release, 0700)
	exe, _ := os.Executable()
	if e := stageCopy(exe, filepath.Join(release, executable()), nil); e != nil {
		t.Fatal(e)
	}
	profile := filepath.Join(home, "devices", "fixture", "profile")
	os.MkdirAll(profile, 0700)
	os.WriteFile(filepath.Join(profile, ".routing-fixture"), nil, 0600)
	for _, name := range []string{"CONTROL_PLANE_BASE_URL", "CONTROL_PLANE_URL_PATH", "TUNNEL_CLIENT_CONFIG", "TUNNEL_CLIENT_PROFILE", "TUNNEL_CLIENT_PROFILE_FILE", "MCP_COMMAND", "MCP_SERVER_URL", "HTTP_PROXY", "HTTPS_PROXY", "SSL_CERT_FILE", "LOG_HTTP_RAW_UNSAFE"} {
		t.Setenv(name, "malicious-fixture-not-read")
	}
	key := []byte("synthetic-routing-key")
	var messages []string
	var messageMu sync.Mutex
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	code, e := (Manager{Progress: func(s string) {
		messageMu.Lock()
		defer messageMu.Unlock()
		messages = append(messages, s)
	}}).Run(home, core.Device{Name: "fixture", Tunnel: "tunnel_fixture", Release: "fixture"}, key)
	os.Stdout = oldStdout
	w.Close()
	out, _ := io.ReadAll(r)
	r.Close()
	if len(out) != 0 {
		t.Fatal("human status contaminated protocol stdout")
	}
	joined := strings.Join(messages, "\n")
	if !strings.Contains(joined, "接続はまだ未確認") || !strings.Contains(joined, "OpenAI トンネル 切断") || !strings.Contains(joined, "終了コード 42") || strings.Contains(joined, "synthetic-routing-key") {
		t.Fatal("unsafe or missing launch status")
	}
	if e != nil || code != 42 {
		t.Fatal("actual child routing/credential guard failed")
	}
	for _, b := range key {
		if b != 0 {
			t.Fatal("key buffer not cleared")
		}
	}
}
