//go:build windows

package atomicfile

import "golang.org/x/sys/windows"

func replace(source, destination string) error {
	s, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	d, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(s, d, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
