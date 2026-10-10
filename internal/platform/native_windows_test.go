//go:build windows

package platform

import (
	"strings"
	"testing"
	"unsafe"
)

func TestWindowsTokenMetadataWithoutCredentials(t *testing.T) {
	i, e := CurrentIdentity()
	if e != nil || !strings.HasPrefix(i.Owner, "windows:S-1-") {
		t.Fatal("current token metadata unavailable")
	}
	t.Logf("token metadata verified; elevated=%t (no credential access)", i.Elevated)
}
func TestWindowsCredentialABIWithoutVaultAccess(t *testing.T) {
	var c credential
	if unsafe.Sizeof(c) != 80 || unsafe.Offsetof(c.Blob) != 40 || unsafe.Offsetof(c.Persist) != 48 || unsafe.Offsetof(c.User) != 72 {
		t.Fatal("CREDENTIALW amd64 layout mismatch")
	}
}
