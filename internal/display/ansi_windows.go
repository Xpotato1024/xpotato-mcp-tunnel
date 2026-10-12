//go:build windows

package display

import (
	"golang.org/x/sys/windows"
	"os"
)

// Read capability only; never change the caller's console mode.
func supportsANSI(file *os.File) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(file.Fd()), &mode) == nil && mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0
}
