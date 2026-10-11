package main

import (
	"bufio"
	"strings"
	"testing"
)

func TestTextReaderKeepsPastedLinesWithoutReadingKeyAhead(t *testing.T) {
	source := strings.NewReader("端末名\nsecond line\nsynthetic-hidden-input\n")
	reader := bufio.NewReader(singleByteReader{source})
	for _, want := range []string{"端末名\n", "second line\n"} {
		line, e := reader.ReadString('\n')
		if e != nil || line != want {
			t.Fatal("pasted line lost", line, e)
		}
	}
	if reader.Buffered() != 0 || source.Len() != len("synthetic-hidden-input\n") {
		t.Fatal("hidden input read ahead")
	}
}
