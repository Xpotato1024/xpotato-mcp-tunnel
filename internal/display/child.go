package display

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

var secretToken = regexp.MustCompile(`(?i)\bsk[-_][a-z0-9_-]{8,}|\bBearer\s+[^\s"',;]+`)
var credentialField = regexp.MustCompile(`(?i)(api[-_]?key|authorization|password|secret|access[-_]?token|cookie)(["']?\s*[:=]\s*)("[^"\r\n]*"|'[^'\r\n]*'|[^\s,;]+)`)

// A fingerprint masks the exact runtime key without retaining its plaintext
// after Run clears the launch buffer. The short hash is only a fast prefilter;
// SHA-256 confirms every match, including across stderr Write boundaries.
type keyFingerprint struct {
	length, prefixLength int
	prefix               uint64
	digest               [32]byte
}

func shortHash(b []byte) uint64 {
	h := uint64(14695981039346656037)
	for _, v := range b {
		h = (h ^ uint64(v)) * 1099511628211
	}
	return h
}

func fingerprint(key []byte) keyFingerprint {
	if len(key) == 0 || len(key) > maxMessageBytes {
		return keyFingerprint{}
	}
	n := min(16, len(key))
	return keyFingerprint{len(key), n, shortHash(key[:n]), sha256.Sum256(key)}
}

func (k keyFingerprint) redact(b []byte) string {
	var out strings.Builder
	for i := 0; i < len(b); {
		if k.length > 0 && i+k.length <= len(b) && shortHash(b[i:i+k.prefixLength]) == k.prefix && sha256.Sum256(b[i:i+k.length]) == k.digest {
			out.WriteString("[秘密非表示]")
			i += k.length
		} else {
			out.WriteByte(b[i])
			i++
		}
	}
	s := secretToken.ReplaceAllString(out.String(), "[秘密非表示]")
	return credentialField.ReplaceAllString(s, "$1$2[秘密非表示]")
}

// Preserve only our simple SGR colors; cursor/terminal controls and bidi are
// escaped. No-color/pipes also remove SGR rather than leaking ANSI byte codes.
func childText(s string, color bool) string {
	var out strings.Builder
	for len(s) > 0 && out.Len() < maxMessageBytes-64 {
		matched := false
		for _, sgr := range []string{"\x1b[0m", "\x1b[31m", "\x1b[32m", "\x1b[33m", "\x1b[36m"} {
			if strings.HasPrefix(s, sgr) {
				if color {
					out.WriteString(sgr)
				}
				s = s[len(sgr):]
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		r, n := utf8.DecodeRuneInString(s)
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			fmt.Fprintf(&out, "\\u%04x", r)
		} else {
			out.WriteRune(r)
		}
		s = s[n:]
	}
	if len(s) > 0 {
		out.WriteString(" [表示上限で省略]")
	}
	if color {
		out.WriteString("\x1b[0m")
	}
	return out.String() + "\n"
}

// ChildWriter drains only stderr, never protocol stdout/stdin. One partial
// line is bounded to 4KiB; its async queue (including active write) to 64KiB.
// A blocked or failed display cannot stall the child's protocol/cleanup.
type ChildWriter struct {
	mu                sync.Mutex
	reporter          *Reporter
	key               keyFingerprint
	color             bool
	partial           []byte
	oversized, closed bool
	dropped           uint64
}

func NewChild(writer io.Writer, color bool, key []byte) *ChildWriter {
	return &ChildWriter{reporter: New(writer, Style{}), key: fingerprint(key), color: color, partial: make([]byte, 0, maxMessageBytes)}
}

func (w *ChildWriter) emit(line string) {
	if w.dropped > 0 && w.reporter.enqueue(fmt.Sprintf("[WARN] 表示混雑のため内部ログ%d行を省略\n", w.dropped)) {
		w.dropped = 0
	}
	if !w.reporter.enqueue(line) && w.dropped < 1<<30 {
		w.dropped++
	}
}

func (w *ChildWriter) endLine() {
	if w.oversized {
		w.emit("[WARN] 内部ログの長い行を省略（4KiB上限）\n")
	} else if len(w.partial) > 0 {
		w.emit(childText(w.key.redact(bytes.TrimSuffix(w.partial, []byte{'\r'})), w.color))
	}
	clear(w.partial)
	w.partial = w.partial[:0]
	w.oversized = false
}

func (w *ChildWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return len(p), nil
	}
	for _, b := range p {
		if b == '\n' {
			w.endLine()
			continue
		}
		if w.oversized {
			continue
		}
		if len(w.partial) == maxMessageBytes {
			clear(w.partial)
			w.partial = w.partial[:0]
			w.oversized = true
			continue
		}
		w.partial = append(w.partial, b)
	}
	return len(p), nil
}

func (w *ChildWriter) Close() {
	w.mu.Lock()
	if !w.closed {
		w.endLine()
		w.closed = true
		if w.dropped > 0 {
			w.reporter.enqueue(fmt.Sprintf("[WARN] 表示混雑のため内部ログ%d行を省略\n", w.dropped))
		}
		w.key = keyFingerprint{}
	}
	w.mu.Unlock()
	w.reporter.Close()
}
