//go:build !windows

package display

import "os"

func supportsANSI(*os.File) bool { return true }
