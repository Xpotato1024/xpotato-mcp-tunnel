//go:build !linux && !darwin

package lockfile

import "os"

func track(*os.File)   {}
func untrack(*os.File) {}
