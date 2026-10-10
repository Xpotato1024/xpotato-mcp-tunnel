package managed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestManagedInstallSmoke(t *testing.T) {
	if os.Getenv("XPOTATO_RUNTIME_SMOKE") != "1" {
		t.Skip("opt-in official downloads and isolated stdio smoke; no key required")
	}
	home := t.TempDir()
	m := Manager{}
	id, e := m.Install(home)
	if e != nil {
		t.Fatal(e)
	}
	if e = m.Verify(home, id); e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(home, "releases", id)
	node, _ := nodeFiles(dir)
	for _, entry := range []struct {
		file string
		args []string
	}{{node, []string{"--version"}}, {filepath.Join(dir, executable()), []string{"--version"}}} {
		cmd := exec.Command(entry.file, entry.args...)
		if e = cmd.Run(); e != nil {
			t.Fatalf("version smoke failed: %v", e)
		}
	}
	profile := filepath.Join(home, "smoke-profile")
	configDir := filepath.Join(profile, ".claude-server-commander")
	os.MkdirAll(configDir, 0700)
	os.MkdirAll(filepath.Join(profile, "Temp"), 0700)
	b, _ := json.Marshal(map[string]any{"allowedDirectories": []string{home}, "blockedCommands": []string{"shutdown"}, "telemetryEnabled": false})
	os.WriteFile(filepath.Join(configDir, "config.json"), b, 0600)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, filepath.Join(dir, "stdio.mjs"), "--no-onboarding")
	cmd.Env = append(cleanEnv(os.Environ(), "OPENAI_API_KEY", "CONTROL_PLANE_API_KEY", "NODE_OPTIONS", "NODE_PATH"), "XPOTATO_MCP_PROFILE="+profile, "XPOTATO_MCP_COMMANDER_ENTRY="+filepath.Join(dir, "node_modules", "@wonderwhy-er", "desktop-commander", "dist", "index.js"))
	stdin, e := cmd.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	decoder := json.NewDecoder(stdout)
	request := func(id int, method string, params any) map[string]json.RawMessage {
		t.Helper()
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		fmt.Fprintln(stdin, string(b))
		for {
			var msg map[string]json.RawMessage
			if e := decoder.Decode(&msg); e != nil {
				t.Fatal("isolated MCP response unavailable")
			}
			var responseID int
			json.Unmarshal(msg["id"], &responseID)
			if responseID == id {
				if msg["error"] != nil {
					t.Fatal("isolated MCP returned an error")
				}
				return msg
			}
		}
	}
	request(1, "initialize", map[string]any{"protocolVersion": "2024-11-05", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "xpotato-smoke", "version": "0"}})
	fmt.Fprintln(stdin, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	result := request(2, "tools/list", map[string]any{})
	var tools struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if json.Unmarshal(result["result"], &tools) != nil || len(tools.Tools) == 0 {
		t.Fatal("empty tools list")
	}
	t.Logf("isolated initialize/tools-list succeeded; toolCount=%d; no credential or control-plane connection", len(tools.Tools))
	if _, e := os.Stat(filepath.Join(configDir, "puppeteer-cache")); e == nil {
		t.Fatal("startup created Chrome cache")
	}
}
