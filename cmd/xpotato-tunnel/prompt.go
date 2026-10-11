package main

import (
	"fmt"
	"io"
)

func writeInputPrompt(out io.Writer, label string) {
	fmt.Fprintln(out, label)
	fmt.Fprint(out, "> ")
}
