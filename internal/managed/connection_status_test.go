package managed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConnectionStateRequiresPassivePollEvidence(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"starting", `{"schema_version":1,"component":"control-plane","status":"unknown","state":"polling"}`, "初回ポーリング成功を待機"},
		{"poll-success", `{"schema_version":1,"component":"control-plane","status":"ok","state":"polling"}`, "ポーリング成功を確認"},
		{"retry", `{"schema_version":1,"component":"control-plane","status":"degraded","state":"backoff","reason_code":"synthetic-private-marker"}`, "エラーを検出"},
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
			if !strings.Contains(got, tc.want) || strings.Contains(got, "synthetic-private-marker") {
				t.Fatal(got)
			}
		})
	}
}

func TestConnectionStatusRejectsRemoteTargetsAndRedirects(t *testing.T) {
	file := filepath.Join(t.TempDir(), "health-url.txt")
	for _, raw := range []string{"https://127.0.0.1:1234", "http://localhost:1234", "http://example.invalid:1234", "http://user:synthetic-key@127.0.0.1:1234", "http://127.0.0.1:1234/private", "http://127.0.0.1:1234?secret=synthetic", strings.Repeat("x", 1025)} {
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
