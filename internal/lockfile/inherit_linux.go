//go:build linux

package lockfile

import (
	"golang.org/x/sys/unix"
	"os"
	"sync"
)

var held = struct {
	sync.Mutex
	files map[*os.File]bool
}{files: make(map[*os.File]bool)}

func track(f *os.File)   { held.Lock(); defer held.Unlock(); held.files[f] = true }
func untrack(f *os.File) { held.Lock(); defer held.Unlock(); delete(held.files, f) }

// InheritHeld duplicates only locks acquired by this process. The Linux
// guardian keeps the same flock descriptions alive across launcher death.
// Callers close these duplicates after exec; runtime children never inherit them.
func InheritHeld() ([]*os.File, error) {
	held.Lock()
	defer held.Unlock()
	var result []*os.File
	for f := range held.files {
		fd, e := unix.FcntlInt(f.Fd(), unix.F_DUPFD_CLOEXEC, 0)
		if e != nil {
			for _, copy := range result {
				copy.Close()
			}
			return nil, e
		}
		result = append(result, os.NewFile(uintptr(fd), "owned-lock"))
	}
	return result, nil
}
