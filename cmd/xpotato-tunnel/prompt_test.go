package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestInputPromptHasSeparatePlainInputLine(t *testing.T) {
	var out bytes.Buffer
	label := "進む yes / 修正 device / 中断 no または q"
	writeInputPrompt(&out, label)
	if out.String() != label+"\n> " || strings.ContainsAny(out.String(), "\r\x1b") {
		t.Fatal(out.String())
	}
}
