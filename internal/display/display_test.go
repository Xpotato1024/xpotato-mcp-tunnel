package display

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestColorCapabilityAndPlainLabels(t *testing.T) {
	for _, tc := range []struct {
		tty, vt, noColor bool
		terminal         string
		want             bool
	}{
		{true, true, false, "xterm", true}, {false, true, false, "xterm", false},
		{true, false, false, "", false}, {true, true, true, "xterm", false}, {true, true, false, "dumb", false},
	} {
		if colorAllowed(tc.tty, tc.vt, tc.noColor, tc.terminal) != tc.want {
			t.Fatal(tc)
		}
	}
	t.Setenv("NO_COLOR", "")
	file, err := os.CreateTemp(t.TempDir(), "pipe")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if Detect(file).Color {
		t.Fatal("non TTY or NO_COLOR enabled ANSI")
	}
	plain := (Style{}).Format("OpenAI トンネル 接続を確認（要求を待機）")
	if !strings.HasPrefix(plain, "[OK] ") || strings.Contains(plain, "\x1b") {
		t.Fatal(plain)
	}
	colored := (Style{true}).Format("MCP 処理失敗")
	if !strings.HasPrefix(colored, "\x1b[31m[ERR]\x1b[0m ") || !strings.Contains(colored, "処理失敗") {
		t.Fatal(colored)
	}
}

func TestFixedStatusEscapesControlsAndBoundsBytes(t *testing.T) {
	line := (Style{}).Format("label\x1b[31m\r\n\u202e" + strings.Repeat("あ", 5000))
	if strings.ContainsAny(line, "\x1b\r\u202e") || strings.Count(line, "\n") != 1 || len(line) > maxMessageBytes || !utf8.ValidString(line) {
		t.Fatal("unsafe display")
	}
}

type blockedSink struct{ entered, release chan struct{} }

func (w *blockedSink) Write(p []byte) (int, error) {
	select {
	case w.entered <- struct{}{}:
	default:
	}
	<-w.release
	return len(p), nil
}

func TestBlockedDisplayCannotDelayWorkOrCleanup(t *testing.T) {
	w := &blockedSink{make(chan struct{}, 1), make(chan struct{})}
	r := New(w, Style{})
	r.Report("処理開始")
	select {
	case <-w.entered:
	case <-time.After(time.Second):
		t.Fatal("writer not started")
	}
	finished := make(chan struct{})
	go func() {
		for i := 0; i < 10000; i++ {
			r.Report(strings.Repeat("x", 3000))
		}
		r.Close()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		close(w.release)
		t.Fatal("display blocked work/cleanup")
	}
	r.mu.Lock()
	queued := r.bytes
	r.mu.Unlock()
	if queued > maxQueuedBytes {
		t.Fatal(queued)
	}
	close(w.release)
	select {
	case <-r.done:
	case <-time.After(time.Second):
		t.Fatal("writer did not finish")
	}
}

type errorSink struct{}

func (errorSink) Write([]byte) (int, error) { return 0, errors.New("synthetic EPIPE") }

func TestDisplayFailureAndCloseDoNotAffectResults(t *testing.T) {
	r := New(errorSink{}, Style{})
	r.Report("処理開始")
	r.Close()
	if r.Report("応答成功") {
		t.Fatal("closed reporter accepted output")
	}
	var b bytes.Buffer
	r = New(&b, Style{})
	r.Report("OpenAI トンネル 接続を確認")
	r.Report("OpenAI トンネル 通信中断・接続エラーを検出")
	r.Report("OpenAI トンネル 再接続を確認")
	r.Close()
	if strings.Count(b.String(), "\n") != 3 || !strings.Contains(b.String(), "[ERR]") || strings.Count(b.String(), "[OK]") != 2 {
		t.Fatal(b.String())
	}
}
