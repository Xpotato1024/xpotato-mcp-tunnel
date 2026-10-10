//go:build windows

package lockfile

import (
	"golang.org/x/sys/windows"
	"os"
	"unsafe"
)

func openLock(path string) (*os.File, error) {
	p, e := windows.UTF16PtrFromString(path)
	if e != nil {
		return nil, e
	}
	h, e := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, e
	}
	return os.NewFile(uintptr(h), path), nil
}
func ownedFile(f *os.File) error {
	h := windows.Handle(f.Fd())
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return ErrOwner
	}
	sd, e := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if e != nil {
		return ErrOwner
	}
	sid, _, e := sd.Owner()
	if e != nil || sid == nil {
		return ErrOwner
	}
	token := windows.GetCurrentProcessToken()
	user, e := token.GetTokenUser()
	if e != nil {
		return ErrOwner
	}
	if sid.Equals(user.User.Sid) {
		return nil
	}
	// CI's elevated token may create files with its default group owner. App
	// commands still reject elevated tokens before reaching this package.
	var size uint32
	windows.GetTokenInformation(token, windows.TokenOwner, nil, 0, &size)
	if size == 0 {
		return ErrOwner
	}
	b := make([]byte, size)
	if windows.GetTokenInformation(token, windows.TokenOwner, &b[0], size, &size) != nil {
		return ErrOwner
	}
	owner := *(**windows.SID)(unsafe.Pointer(&b[0]))
	if owner != nil && sid.Equals(owner) {
		return nil
	}
	return ErrOwner
}
func tryLock(f *os.File) error {
	e := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
	if e == windows.ERROR_LOCK_VIOLATION || e == windows.ERROR_IO_PENDING {
		return ErrBusy
	}
	return e
}
func unlock(f *os.File) { windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &windows.Overlapped{}) }
