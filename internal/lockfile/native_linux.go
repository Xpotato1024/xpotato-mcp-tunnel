//go:build linux

package lockfile

import (
	"golang.org/x/sys/unix"
	"os"
)

func openLock(path string) (*os.File, error) {
	fd, e := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return nil, e
	}
	return os.NewFile(uintptr(fd), path), nil
}
func ownedFile(f *os.File) error {
	var st unix.Stat_t
	if unix.Fstat(int(f.Fd()), &st) != nil || int(st.Uid) != os.Geteuid() || st.Mode&unix.S_IFMT != unix.S_IFREG {
		return ErrOwner
	}
	return nil
}
func tryLock(f *os.File) error {
	e := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if e == unix.EWOULDBLOCK || e == unix.EAGAIN {
		return ErrBusy
	}
	return e
}
func unlock(f *os.File) { unix.Flock(int(f.Fd()), unix.LOCK_UN) }
