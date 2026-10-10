//go:build windows

package platform

import (
	"golang.org/x/sys/windows"
	"unsafe"
)

func CurrentIdentity() (Identity, error) {
	t := windows.GetCurrentProcessToken()
	u, err := t.GetTokenUser()
	if err != nil {
		return Identity{}, err
	}
	return Identity{Owner: "windows:" + u.User.Sid.String(), Elevated: t.IsElevated()}, nil
}

// CREDENTIALW, layout supplied by wincred.h. Generic credentials are scoped to
// the current user's logon account; LOCAL_MACHINE persistence survives logons.
type credential struct {
	Flags, Type             uint32
	Target, Comment         *uint16
	Written                 windows.Filetime
	BlobSize                uint32
	Blob                    *byte
	Persist, AttributeCount uint32
	Attributes              uintptr
	Alias, User             *uint16
}

var advapi = windows.NewLazySystemDLL("advapi32.dll")
var credWrite = advapi.NewProc("CredWriteW")
var credRead = advapi.NewProc("CredReadW")
var credFree = advapi.NewProc("CredFree")

type NativeVault struct{}

func (NativeVault) Ready() error { return nil }
func (NativeVault) Save(target string, key []byte) error {
	if len(key) == 0 || len(key) > 2560 {
		return ErrDenied
	}
	t, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return ErrDenied
	}
	c := credential{Type: 1, Target: t, BlobSize: uint32(len(key)), Blob: &key[0], Persist: 2}
	ok, _, _ := credWrite.Call(uintptr(unsafe.Pointer(&c)), 0)
	if ok == 0 {
		return ErrDenied
	}
	return nil
}
func (NativeVault) Load(target string) ([]byte, error) {
	t, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return nil, ErrDenied
	}
	var c *credential
	ok, _, callErr := credRead.Call(uintptr(unsafe.Pointer(t)), 1, 0, uintptr(unsafe.Pointer(&c)))
	if ok == 0 {
		if callErr == windows.ERROR_NOT_FOUND {
			return nil, ErrMissing
		}
		return nil, ErrDenied
	}
	defer credFree.Call(uintptr(unsafe.Pointer(c)))
	if c == nil || c.BlobSize == 0 || c.BlobSize > 2560 {
		return nil, ErrDenied
	}
	return append([]byte(nil), unsafe.Slice(c.Blob, c.BlobSize)...), nil
}
