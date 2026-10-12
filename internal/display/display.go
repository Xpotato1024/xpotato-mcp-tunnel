// Package display renders fixed status labels on stderr without delaying work.
package display

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/term"
)

const maxMessageBytes = 4096
const maxQueuedBytes = 64 * 1024

type Style struct{ Color bool }

func colorAllowed(tty, virtualTerminal, noColor bool, terminal string) bool {
	return tty && virtualTerminal && !noColor && terminal != "dumb"
}

func Detect(file *os.File) Style {
	_, noColor := os.LookupEnv("NO_COLOR")
	return Style{colorAllowed(term.IsTerminal(int(file.Fd())), supportsANSI(file), noColor, os.Getenv("TERM"))}
}

// ASCII markers have stable width in Windows PowerShell, pipes and old fonts.
// Labels carry the meaning even when color or symbols cannot be rendered.
func (s Style) Format(message string) string {
	tag, color := "INFO", "36"
	switch {
	case strings.Contains(message, "失敗"), strings.Contains(message, "エラー"):
		tag, color = "ERR", "31"
	case strings.Contains(message, "切断"), strings.Contains(message, "停止"):
		tag, color = "OFF", "33"
	case strings.Contains(message, "未確認"), strings.Contains(message, "待機") && !strings.Contains(message, "接続を確認"):
		tag, color = "WAIT", "33"
	case strings.Contains(message, "接続を確認"), strings.Contains(message, "完了"):
		tag, color = "OK", "32"
	}
	var b strings.Builder
	for _, r := range message {
		if b.Len() >= maxMessageBytes-64 {
			break
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			fmt.Fprintf(&b, "\\u%04x", r)
		} else {
			b.WriteRune(r)
		}
	}
	prefix := "[" + tag + "]"
	if s.Color {
		prefix = "\x1b[" + color + "m" + prefix + "\x1b[0m"
	}
	return prefix + " " + b.String() + "\n"
}

type Reporter struct {
	mu               sync.Mutex
	queue            chan string
	done             chan struct{}
	bytes            int
	closed, disabled bool
	style            Style
}

func New(writer io.Writer, style Style) *Reporter {
	r := &Reporter{queue: make(chan string, 64), done: make(chan struct{}), style: style}
	go func() {
		defer close(r.done)
		for line := range r.queue {
			r.mu.Lock()
			disabled := r.disabled
			r.mu.Unlock()
			if !disabled {
				if n, err := io.WriteString(writer, line); err != nil || n != len(line) {
					r.mu.Lock()
					r.disabled = true
					r.mu.Unlock()
				}
			}
			r.mu.Lock()
			r.bytes -= len(line)
			r.mu.Unlock()
		}
	}()
	return r
}

// Best effort: one bounded writer goroutine, no waiting when the sink is slow.
func (r *Reporter) Report(message string) bool {
	return r.enqueue(r.style.Format(message))
}

func (r *Reporter) enqueue(line string) bool {
	if len(line) > maxMessageBytes || !utf8.ValidString(line) {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.disabled || r.bytes+len(line) > maxQueuedBytes {
		return false
	}
	select {
	case r.queue <- line:
		r.bytes += len(line)
		return true
	default:
		return false
	}
}

// A blocked display must not hold up process cleanup or CLI exit.
func (r *Reporter) Close() {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.queue)
	}
	r.mu.Unlock()
	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-r.done:
	case <-timer.C:
	}
}
