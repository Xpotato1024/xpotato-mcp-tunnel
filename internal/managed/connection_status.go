package managed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const connectionUnknown = "接続状態は未確認（ローカル状態の公開を待機）"

// Read only this launch's bounded, passive control-plane snapshot. Never fetch
// an arbitrary URL, follow redirects, use a proxy, or attach a saved key.
func connectionStatus(ctx context.Context, file string) string {
	client, transport := healthClient()
	defer transport.CloseIdleConnections()
	return connectionStatusWithClient(ctx, file, client)
}

func healthClient() (*http.Client, *http.Transport) {
	transport := &http.Transport{Proxy: nil, MaxIdleConns: 1, MaxIdleConnsPerHost: 1, MaxConnsPerHost: 1, IdleConnTimeout: time.Minute}
	return &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect rejected") }}, transport
}

func connectionStatusWithClient(ctx context.Context, file string, client *http.Client) string {
	f, err := os.Open(file)
	if err != nil {
		return connectionUnknown
	}
	b, err := io.ReadAll(io.LimitReader(f, 1025))
	f.Close()
	if err != nil || len(b) > 1024 {
		return connectionUnknown
	}
	u, err := url.Parse(strings.TrimSpace(string(b)))
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/healthz") {
		return connectionUnknown
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return connectionUnknown
	}
	u.Path = "/health/control-plane"
	requestCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return connectionUnknown
	}
	resp, err := client.Do(req)
	if err != nil {
		return connectionUnknown
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return connectionUnknown
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 24*1024+1))
	if err != nil || len(data) > 24*1024 {
		return connectionUnknown
	}
	var snapshot struct {
		Schema    int    `json:"schema_version"`
		Component string `json:"component"`
		Status    string `json:"status"`
		State     string `json:"state"`
		Limited   bool   `json:"limited"`
	}
	if json.Unmarshal(data, &snapshot) != nil || snapshot.Schema != 1 || snapshot.Component != "control-plane" || snapshot.Limited {
		return connectionUnknown
	}
	switch {
	case snapshot.State == "stopped":
		return "control-plane のポーリング停止を確認"
	case snapshot.Status == "degraded":
		return "control-plane への接続でエラーを検出（runtime が再試行）"
	case snapshot.Status == "ok" && (snapshot.State == "idle" || snapshot.State == "polling" || snapshot.State == "backpressured"):
		return "control-plane のポーリング成功を確認（MCP 操作の成功は別途確認）"
	default:
		return "control-plane の初回ポーリング成功を待機"
	}
}

func monitorConnection(file string, report func(string), interval time.Duration) func() {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		client, transport := healthClient()
		defer transport.CloseIdleConnections()
		started := time.Now()
		last, lastReport := "", time.Time{}
		for {
			message := connectionStatusWithClient(ctx, file, client)
			if ctx.Err() != nil {
				return
			}
			if message != last || time.Since(lastReport) >= 15*time.Second {
				report(fmt.Sprintf("%s — 子プロセス稼働中（経過 %s）", message, time.Since(started).Truncate(time.Second)))
				last, lastReport = message, time.Now()
			}
			delay := interval
			if strings.Contains(message, "ポーリング成功を確認") {
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
