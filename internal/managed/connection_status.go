package managed

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type connectionState uint8

const (
	connectionUnconfirmed connectionState = iota
	connectionWaiting
	connectionOnline
	connectionRetrying
	connectionStopped
)

const connectionUnknown = "OpenAI トンネル 接続状態未確認（最新の通信成功を確認できません）"

func (s connectionState) message(reconnected bool) string {
	switch s {
	case connectionWaiting:
		return "OpenAI トンネル 初回接続を待機"
	case connectionOnline:
		if reconnected {
			return "OpenAI トンネル 再接続を確認（要求を待機）"
		}
		return "OpenAI トンネル 接続を確認（要求を待機）"
	case connectionRetrying:
		return "OpenAI トンネル 通信中断・接続エラーを検出（再接続を試行中）"
	case connectionStopped:
		return "OpenAI トンネル 切断（ポーリング停止を確認）"
	default:
		return connectionUnknown
	}
}

// Read only this launch's bounded, passive control-plane snapshot. Never fetch
// an arbitrary URL, follow redirects, use a proxy, or attach a saved key.
func connectionStatus(ctx context.Context, file string) string {
	client, transport := healthClient()
	defer transport.CloseIdleConnections()
	return readConnectionState(ctx, file, client).message(false)
}

func healthClient() (*http.Client, *http.Transport) {
	transport := &http.Transport{Proxy: nil, MaxIdleConns: 1, MaxIdleConnsPerHost: 1, MaxConnsPerHost: 1, IdleConnTimeout: time.Minute}
	return &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect rejected") }}, transport
}

func readConnectionState(ctx context.Context, file string, client *http.Client) connectionState {
	f, err := os.Open(file)
	if err != nil {
		return connectionUnconfirmed
	}
	b, err := io.ReadAll(io.LimitReader(f, 1025))
	f.Close()
	if err != nil || len(b) > 1024 {
		return connectionUnconfirmed
	}
	u, err := url.Parse(strings.TrimSpace(string(b)))
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/healthz") {
		return connectionUnconfirmed
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return connectionUnconfirmed
	}
	u.Path = "/health/control-plane"
	requestCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return connectionUnconfirmed
	}
	resp, err := client.Do(req)
	if err != nil {
		return connectionUnconfirmed
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return connectionUnconfirmed
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 24*1024+1))
	if err != nil || len(data) > 24*1024 {
		return connectionUnconfirmed
	}
	return classifyConnection(data, time.Now())
}

// These are passive observations, not a new probe or proof of MCP success.
// An old success must not survive sleep/clock jumps as a current connection.
func classifyConnection(data []byte, now time.Time) connectionState {
	var snapshot struct {
		Schema     int       `json:"schema_version"`
		Component  string    `json:"component"`
		Status     string    `json:"status"`
		State      string    `json:"state"`
		Limited    bool      `json:"limited"`
		SnapshotAt time.Time `json:"snapshot_at"`
		Details    struct {
			LastAttempt time.Time `json:"last_attempt"`
			LastSuccess time.Time `json:"last_success"`
			Deadline    float64   `json:"deadline_seconds"`
		} `json:"details"`
	}
	if json.Unmarshal(data, &snapshot) != nil || snapshot.Schema != 1 || snapshot.Component != "control-plane" || snapshot.Limited {
		return connectionUnconfirmed
	}
	// A localhost response is generated on this clock. Reject stale/future
	// envelopes and incomplete diagnostics rather than guessing connectivity.
	fresh := func(t time.Time, maximum time.Duration) bool {
		return !t.IsZero() && !t.After(now.Add(time.Second)) && now.Sub(t) <= maximum
	}
	if !fresh(snapshot.SnapshotAt, 5*time.Second) {
		return connectionUnconfirmed
	}
	if snapshot.State == "stopped" {
		return connectionStopped
	}
	if snapshot.Status == "degraded" {
		return connectionRetrying
	}
	if snapshot.Status == "unknown" && (snapshot.State == "starting" || snapshot.State == "polling") {
		return connectionWaiting
	}
	seconds := snapshot.Details.Deadline
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > 600 {
		return connectionUnconfirmed
	}
	deadline := time.Duration(seconds * float64(time.Second))
	if snapshot.Status != "ok" || !fresh(snapshot.Details.LastSuccess, deadline+5*time.Second) {
		return connectionUnconfirmed
	}
	switch snapshot.State {
	case "polling":
		if !fresh(snapshot.Details.LastAttempt, deadline+time.Second) {
			return connectionUnconfirmed
		}
	case "idle", "backpressured":
	default:
		return connectionUnconfirmed
	}
	return connectionOnline
}

type connectionEvents struct {
	last      connectionState
	seen      bool
	wasOnline bool
}

func (e *connectionEvents) observe(state connectionState, report func(string)) {
	if e.seen && state == e.last {
		return
	}
	e.last, e.seen = state, true
	report(state.message(e.wasOnline))
	if state == connectionOnline {
		e.wasOnline = true
	}
}

func monitorConnection(file string, report func(string), interval time.Duration) func() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		client, transport := healthClient()
		defer transport.CloseIdleConnections()
		events := connectionEvents{}
		for {
			state := readConnectionState(ctx, file, client)
			if ctx.Err() != nil {
				return
			}
			events.observe(state, report)
			delay := interval
			if state == connectionOnline {
				delay = 3 * interval
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { cancel(); <-done }) }
}
