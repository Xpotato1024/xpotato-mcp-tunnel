package managed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestManagedInstallSmoke(t *testing.T) {
	if os.Getenv("XPOTATO_RUNTIME_SMOKE") != "1" {
		t.Skip("opt-in official downloads and isolated stdio smoke; no key required")
	}
	// Windows CI TEMP uses an 8.3 alias (RUNNER~1). Commander checks the
	// real path against allowedDirectories, so configure the canonical fixture
	// rather than granting an alias that its validator correctly rejects.
	home, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
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
	searchRoot := filepath.Join(home, "search-fixture")
	os.MkdirAll(searchRoot, 0700)
	os.WriteFile(filepath.Join(searchRoot, "needle.txt"), []byte("fixture_unique_search_7d19\n"), 0600)
	configDir := filepath.Join(profile, ".claude-server-commander")
	os.MkdirAll(configDir, 0700)
	os.MkdirAll(filepath.Join(profile, "Temp"), 0700)
	b, _ := json.Marshal(map[string]any{"allowedDirectories": []string{searchRoot}, "blockedCommands": []string{"shutdown"}, "telemetryEnabled": false})
	os.WriteFile(filepath.Join(configDir, "config.json"), b, 0600)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// Explicit synthetic presentation flag exercises the pinned entry's color
	// path even in CI pipes; real launches gate it on the user's terminal.
	cmd := exec.CommandContext(ctx, node, filepath.Join(dir, "stdio.mjs"), "--no-onboarding", "--activity-color")
	var activity bytes.Buffer
	cmd.Stderr = &activity
	cmd.Env = append(cleanEnv(os.Environ(), "OPENAI_API_KEY", "CONTROL_PLANE_API_KEY", "NODE_OPTIONS", "NODE_PATH", "PATH"), "PATH=", "XPOTATO_MCP_PROFILE="+profile, "XPOTATO_MCP_COMMANDER_ENTRY="+filepath.Join(dir, "node_modules", "@wonderwhy-er", "desktop-commander", "dist", "index.js"))
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
	// Verify the packaged binary exists and is selected with no host PATH rg.
	rgProbe := exec.Command(node, "--input-type=module", "-e", `import {rgPath} from '@vscode/ripgrep'; import fs from 'node:fs'; if (!rgPath.includes('node_modules') || !fs.existsSync(rgPath)) process.exit(2);`)
	rgProbe.Dir = dir
	rgProbe.Env = append(cleanEnv(os.Environ(), "PATH", "NODE_OPTIONS", "NODE_PATH", "OPENAI_API_KEY", "CONTROL_PLANE_API_KEY"), "PATH=")
	if e = rgProbe.Run(); e != nil {
		t.Fatal("locked package ripgrep binary unavailable")
	}
	search := request(3, "tools/call", map[string]any{"name": "start_search", "arguments": map[string]any{"path": searchRoot, "pattern": "fixture_unique_search_7d19", "searchType": "content", "literalSearch": true, "maxResults": 10, "timeout_ms": 5000}})
	var toolResult struct {
		Content []struct{ Text string }
		IsError bool `json:"isError"`
	}
	if json.Unmarshal(search["result"], &toolResult) != nil || toolResult.IsError {
		t.Fatalf("hermetic search startup failed on isolated fixture: %s", search["result"])
	}
	text := ""
	for _, c := range toolResult.Content {
		text += c.Text
	}
	session := regexp.MustCompile(`search_[0-9]+_[0-9]+`).FindString(text)
	if session == "" {
		t.Fatal("search session missing")
	}
	matched := false
	for i := 0; i < 30; i++ {
		result := request(4+i, "tools/call", map[string]any{"name": "get_more_search_results", "arguments": map[string]any{"sessionId": session, "offset": 0, "length": 20}})
		if json.Unmarshal(result["result"], &toolResult) != nil || toolResult.IsError {
			t.Fatal("hermetic search failed")
		}
		for _, c := range toolResult.Content {
			if strings.Contains(c.Text, "needle.txt") && strings.Contains(c.Text, "fixture_unique_search_7d19") {
				matched = true
			}
		}
		if matched {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !matched {
		t.Fatal("hermetic search did not return fixture content")
	}
	t.Log("functional content search succeeded with PATH empty and locked package rg")
	cmd.Process.Kill()
	cmd.Wait() // Join stderr copying before examining the display buffer.
	if !strings.Contains(activity.String(), "\x1b[36m[RUN]\x1b[0m [MCP #1] start_search 処理開始") || !strings.Contains(activity.String(), "start_search 応答成功") {
		t.Fatal("pinned SDK tool lifecycle observer did not run")
	}
	if _, e := os.Stat(filepath.Join(configDir, "puppeteer-cache")); e == nil {
		t.Fatal("startup created Chrome cache")
	}
}
