package display

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestChildStderrMasksSplitSecretAfterLaunchBufferWipe(t *testing.T) {
	key := []byte("synthetic-runtime-credential-9a7d")
	var output bytes.Buffer
	w := NewChild(&output, false, key)
	clear(key)
	for _, chunk := range []string{"WARN failed key=synthetic-runtime-", "credential-9a7d Authorization: Bear", "er OTHER_SECRET_MARKER\npassword=PASSWORD_MARKER\n[RUN] [MCP #1] list_directory 処理開始\n[OK] [MCP #1] list_directory 応答成功 (1ms)\n"} {
		if n, err := w.Write([]byte(chunk)); err != nil || n != len(chunk) {
			t.Fatal(n, err)
		}
	}
	w.Close()
	s := output.String()
	if strings.Contains(s, "credential-9a7d") || strings.Contains(s, "SECRET_MARKER") || strings.Contains(s, "PASSWORD_MARKER") || !strings.Contains(s, "WARN failed") || !strings.Contains(s, "処理開始") || !strings.Contains(s, "応答成功") {
		t.Fatal(s)
	}
}

func TestChildStderrBoundsLongLinesAndEscapesControls(t *testing.T) {
	var output bytes.Buffer
	w := NewChild(&output, false, nil)
	w.Write([]byte(strings.Repeat("x", maxMessageBytes+100) + "\n"))
	w.Write([]byte("\x1b[31m[ERR]\x1b[0m bad\x1b[2J\r\u202e"))
	w.Write([]byte{0xff})
	w.Close()
	s := output.String()
	if !strings.Contains(s, "長い行を省略") || strings.Contains(s, "xxxx") || strings.ContainsAny(s, "\x1b\r\u202e") || !utf8.ValidString(s) || !strings.Contains(s, "\\u001b[2J") {
		t.Fatal(s)
	}
	for _, line := range strings.Split(s, "\n") {
		if len(line) > maxMessageBytes {
			t.Fatal("output line too large")
		}
	}
}

func TestChildStderrPreservesOnlySafeColors(t *testing.T) {
	var output bytes.Buffer
	w := NewChild(&output, true, nil)
	w.Write([]byte("\x1b[36m[RUN]\x1b[0m event\x1b[?25l\n"))
	w.Close()
	if !strings.HasPrefix(output.String(), "\x1b[36m[RUN]\x1b[0m ") || strings.Contains(output.String(), "\x1b[?25l") || !strings.Contains(output.String(), "\\u001b[?25l") {
		t.Fatal(output.String())
	}
}

func TestChildStderrSlowAndFailedSinkCannotBlockProducer(t *testing.T) {
	sink := &blockedSink{make(chan struct{}, 1), make(chan struct{})}
	w := NewChild(sink, false, nil)
	w.Write([]byte("WARN first\n"))
	select {
	case <-sink.entered:
	case <-time.After(time.Second):
		t.Fatal("writer not started")
	}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			w.Write([]byte(strings.Repeat("x", 3000) + "\n"))
		}
		w.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		close(sink.release)
		t.Fatal("stderr blocked producer/cleanup")
	}
	w.reporter.mu.Lock()
	queued := w.reporter.bytes
	w.reporter.mu.Unlock()
	if queued > maxQueuedBytes || cap(w.partial) > maxMessageBytes {
		t.Fatal("unbounded stderr")
	}
	close(sink.release)
	select {
	case <-w.reporter.done:
	case <-time.After(time.Second):
		t.Fatal("writer remained active")
	}
	w = NewChild(errorSink{}, false, nil)
	w.Write([]byte("WARN failure\n"))
	w.Close()
	if n, err := w.Write([]byte("late\n")); err != nil || n != 5 {
		t.Fatal("display failure reached producer", n, err)
	}
}
