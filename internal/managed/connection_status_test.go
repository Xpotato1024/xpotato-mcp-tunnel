package managed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func syntheticHealth(now time.Time, status, state string, lastSuccess, lastAttempt time.Time) []byte {
	b, _ := json.Marshal(map[string]any{"schema_version": 1, "component": "control-plane", "status": status, "state": state, "snapshot_at": now,
		"details": map[string]any{"last_success": lastSuccess, "last_attempt": lastAttempt, "deadline_seconds": 35}})
	return b
}

func TestConnectionStateRequiresPassivePollEvidence(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct{ name, body, want string }{
		{"starting", string(syntheticHealth(now, "unknown", "polling", time.Time{}, now)), "初回接続を待機"},
		{"poll-success", string(syntheticHealth(now, "ok", "polling", now, now)), "接続を確認"},
		{"retry", string(syntheticHealth(now, "degraded", "backoff", now, now)), "エラーを検出"},
		{"stopped", string(syntheticHealth(now, "ok", "stopped", now, now)), "切断"},
		{"missing-freshness", `{"schema_version":1,"component":"control-plane","status":"ok","state":"polling"}`, "未確認"},
		{"invalid-schema", `{"schema_version":99,"component":"control-plane","status":"ok","state":"idle"}`, "未確認"},
		{"other-component", `{"schema_version":1,"component":"mcp","status":"ok","state":"idle"}`, "未確認"},
		{"oversized", strings.Repeat("x", 24*1024+1), "未確認"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/health/control-plane" || r.Header.Get("Authorization") != "" {
					t.Error("unapproved request")
				}
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			file := filepath.Join(t.TempDir(), "health-url.txt")
			os.WriteFile(file, []byte(server.URL), 0600)
			got := connectionStatus(context.Background(), file)
			if !strings.Contains(got, tc.want) || !strings.HasPrefix(got, "OpenAI トンネル") || strings.Contains(got, "control-plane") {
				t.Fatal(got)
			}
		})
	}
}

func TestConnectionStatusRejectsRemoteTargetsAndRedirects(t *testing.T) {
	file := filepath.Join(t.TempDir(), "health-url.txt")
	for _, raw := range []string{"https://127.0.0.1:1234", "http://localhost:1234", "http://example.invalid:1234", "http://user:synthetic-key@127.0.0.1:1234", "http://127.0.0.1:1234/private", "http://127.0.0.1:1234?secret=synthetic", "http://127.0.0.1:1234?", strings.Repeat("x", 1025)} {
		os.WriteFile(file, []byte(raw), 0600)
		if got := connectionStatus(context.Background(), file); got != connectionUnknown {
			t.Fatal(got)
		}
	}
	var followed bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer redirect.Close()
	os.WriteFile(file, []byte(redirect.URL), 0600)
	if got := connectionStatus(context.Background(), file); got != connectionUnknown || followed {
		t.Fatal("redirect followed", got)
	}
}

func TestConnectionFreshnessAcrossSleepAndClockChanges(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		data []byte
		want connectionState
	}{
		{"long-poll-within-deadline", syntheticHealth(now, "ok", "polling", now.Add(-30*time.Second), now.Add(-30*time.Second)), connectionOnline},
		{"sleep-old-success-new-attempt", syntheticHealth(now, "ok", "polling", now.Add(-time.Hour), now), connectionUnconfirmed},
		{"overdue-attempt", syntheticHealth(now, "ok", "polling", now, now.Add(-time.Minute)), connectionUnconfirmed},
		{"old-envelope", syntheticHealth(now.Add(-time.Hour), "ok", "polling", now, now), connectionUnconfirmed},
		{"backwards-clock", syntheticHealth(now.Add(time.Hour), "ok", "polling", now, now), connectionUnconfirmed},
		{"future-success", syntheticHealth(now, "ok", "idle", now.Add(time.Hour), now), connectionUnconfirmed},
		{"old-success-while-tool-busy", syntheticHealth(now, "ok", "backpressured", now.Add(-time.Minute), now), connectionUnconfirmed},
		{"fresh-recovery", syntheticHealth(now, "ok", "polling", now, now), connectionOnline},
		{"untrusted-deadline", []byte(strings.Replace(string(syntheticHealth(now, "ok", "polling", now, now)), `"deadline_seconds":35`, `"deadline_seconds":1e30`, 1)), connectionUnconfirmed},
		{"bad-timestamp", []byte(strings.Replace(string(syntheticHealth(now, "ok", "polling", now, now)), `"last_success":`, `"last_success_invalid":`, 1)), connectionUnconfirmed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyConnection(tc.data, now); got != tc.want {
				t.Fatal(got, tc.want)
			}
		})
	}
}

func TestConnectionEventsHaveNoNormalHeartbeatAndDistinguishReconnect(t *testing.T) {
	events := connectionEvents{}
	var lines []string
	report := func(s string) { lines = append(lines, s) }
	events.observe(connectionWaiting, report)
	events.observe(connectionOnline, report)
	for i := 0; i < 100000; i++ {
		events.observe(connectionOnline, report)
	}
	if len(lines) != 2 {
		t.Fatal("normal waiting produced repeated output", len(lines))
	}
	events.observe(connectionRetrying, report)
	events.observe(connectionRetrying, report)
	events.observe(connectionUnconfirmed, report)
	events.observe(connectionOnline, report)
	events.observe(connectionStopped, report)
	if len(lines) != 6 || !strings.Contains(lines[4], "再接続を確認") || !strings.Contains(lines[5], "切断") {
		t.Fatal(lines)
	}
}

func TestPassiveMonitorIdleDisconnectRecovery(t *testing.T) {
	var mode atomic.Int32
	var samples atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health/control-plane" || r.Header.Get("Authorization") != "" {
			t.Error("unapproved request")
		}
		now := time.Now()
		status, state := "ok", "polling"
		if mode.Load() == 1 {
			status, state = "degraded", "backoff"
		}
		w.Write(syntheticHealth(now, status, state, now, now))
		samples.Add(1)
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "health-url.txt")
	os.WriteFile(file, []byte(server.URL), 0600)
	lines := make(chan string, 10)
	stop := monitorConnection(file, func(s string) { lines <- s }, time.Millisecond)
	defer stop()
	read := func(want string) {
		t.Helper()
		select {
		case s := <-lines:
			if !strings.Contains(s, want) {
				t.Fatal(s)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("missing transition", want)
		}
	}
	read("接続を確認")
	end := time.Now().Add(2 * time.Second)
	for samples.Load() < 25 && time.Now().Before(end) {
		time.Sleep(time.Millisecond)
	}
	if samples.Load() < 25 {
		t.Fatal("monitor did not sample")
	}
	select {
	case s := <-lines:
		t.Fatal("idle output", s)
	default:
	}
	mode.Store(1)
	read("通信中断")
	mode.Store(0)
	read("再接続を確認")
	stop()
	select {
	case s := <-lines:
		t.Fatal("extra output", s)
	default:
	}
}

// Cross the former real 15-second notification boundary, not only accelerated
// samples. This would fail if a normal idle heartbeat were reintroduced.
func TestPassiveMonitorNoIdleHeartbeat(t *testing.T) {
	var samples atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		w.Write(syntheticHealth(now, "ok", "polling", now, now))
		samples.Add(1)
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "health-url.txt")
	os.WriteFile(file, []byte(server.URL), 0600)
	lines := make(chan string, 4)
	stop := monitorConnection(file, func(s string) { lines <- s }, 50*time.Millisecond)
	defer stop()
	select {
	case line := <-lines:
		if !strings.Contains(line, "接続を確認") {
			t.Fatal(line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("missing initial connection")
	}
	timer := time.NewTimer(16 * time.Second)
	defer timer.Stop()
	select {
	case line := <-lines:
		t.Fatal("normal waiting notification", line)
	case <-timer.C:
	}
	if samples.Load() < 50 {
		t.Fatal("passive monitor stopped sampling")
	}
}

func TestConnectionMonitorStopsBeforeCleanup(t *testing.T) {
	var mu sync.Mutex
	var messages []string
	ready := make(chan struct{}, 1)
	stop := monitorConnection(filepath.Join(t.TempDir(), "missing"), func(s string) {
		mu.Lock()
		messages = append(messages, s)
		mu.Unlock()
		select {
		case ready <- struct{}{}:
		default:
		}
	}, time.Millisecond)
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("no unconfirmed status")
	}
	stop()
	stop()
	mu.Lock()
	count := len(messages)
	mu.Unlock()
	time.Sleep(5 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(messages) != count {
		t.Fatal("status leaked after cleanup")
	}
}
