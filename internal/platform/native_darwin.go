//go:build darwin && cgo

package platform

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <stdlib.h>

// Per-process interaction policy only. Never creates/unlocks a keychain,
// changes an ACL, enumerates records, or invokes /usr/bin/security with a key.
static OSStatus xpotato_no_ui(void) { return SecKeychainSetUserInteractionAllowed(false); }
static OSStatus xpotato_keychain(SecKeychainRef *keychain) {
    OSStatus status = SecKeychainCopyDefault(keychain);
    if (status != errSecSuccess) return status;
    SecKeychainStatus flags = 0;
    status = SecKeychainGetStatus(*keychain, &flags);
    if (status == errSecSuccess && !(flags & kSecUnlockStateStatus)) status = errSecInteractionNotAllowed;
    if (status != errSecSuccess) { CFRelease(*keychain); *keychain = NULL; }
    return status;
}
static OSStatus xpotato_ready(void) {
    SecKeychainRef keychain = NULL;
    OSStatus status = xpotato_keychain(&keychain);
    if (keychain) CFRelease(keychain);
    return status;
}
static const char xpotato_service[] = "net.xpotato.tunnel.runtime";
static OSStatus xpotato_save(const char *target, UInt32 targetLen, const void *key, UInt32 keyLen) {
    SecKeychainRef keychain = NULL;
    OSStatus status = xpotato_keychain(&keychain);
    if (status != errSecSuccess) return status;
    SecKeychainItemRef item = NULL;
    status = SecKeychainFindGenericPassword(keychain, sizeof(xpotato_service)-1, xpotato_service, targetLen, target, NULL, NULL, &item);
    if (status == errSecItemNotFound) {
        status = SecKeychainAddGenericPassword(keychain, sizeof(xpotato_service)-1, xpotato_service, targetLen, target, keyLen, key, NULL);
    } else if (status == errSecSuccess) {
        status = SecKeychainItemModifyAttributesAndData(item, NULL, keyLen, key);
    }
    if (item) CFRelease(item);
    CFRelease(keychain);
    return status;
}
static OSStatus xpotato_load(const char *target, UInt32 targetLen, UInt32 *length, void **data) {
    SecKeychainRef keychain = NULL;
    OSStatus status = xpotato_keychain(&keychain);
    if (status != errSecSuccess) return status;
    status = SecKeychainFindGenericPassword(keychain, sizeof(xpotato_service)-1, xpotato_service, targetLen, target, length, data, NULL);
    CFRelease(keychain);
    return status;
}
static void xpotato_free(UInt32 length, void *data) {
    if (!data) return;
    volatile unsigned char *bytes = data;
    for (UInt32 i = 0; i < length; i++) bytes[i] = 0;
    SecKeychainItemFreeContent(NULL, data);
}
*/
import "C"

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

func CurrentIdentity() (Identity, error) {
	return Identity{Owner: "darwin:" + strconv.Itoa(os.Getuid()), Elevated: os.Geteuid() == 0 || os.Getuid() != os.Geteuid()}, nil
}

type NativeVault struct{}

var keychainOnce sync.Once
var keychainPolicy C.OSStatus

func keychainError(status C.OSStatus) error {
	if status == C.errSecSuccess {
		return nil
	}
	if status == C.errSecItemNotFound {
		return ErrMissing
	}
	if status == C.errSecInteractionNotAllowed {
		return ErrLocked
	}
	if status == C.errSecNoDefaultKeychain || status == C.errSecNotAvailable {
		return ErrUnavailable
	}
	return ErrDenied
}
func keychainInit() error {
	keychainOnce.Do(func() { keychainPolicy = C.xpotato_no_ui() })
	return keychainError(keychainPolicy)
}
func (NativeVault) Ready() error {
	if err := keychainInit(); err != nil {
		return err
	}
	return keychainError(C.xpotato_ready())
}
func validKeychainTarget(target string) bool {
	return len(target) <= 1024 && strings.HasPrefix(target, "xpotato-mcp-tunnel:") && !strings.ContainsRune(target, 0)
}
func (NativeVault) Save(target string, key []byte) error {
	if !validKeychainTarget(target) || len(key) == 0 || len(key) > 2560 {
		return ErrDenied
	}
	if err := keychainInit(); err != nil {
		return err
	}
	account := C.CString(target)
	defer C.free(unsafe.Pointer(account))
	status := C.xpotato_save(account, C.UInt32(len(target)), unsafe.Pointer(&key[0]), C.UInt32(len(key)))
	runtime.KeepAlive(key)
	return keychainError(status)
}
func (NativeVault) Load(target string) ([]byte, error) {
	if !validKeychainTarget(target) {
		return nil, ErrDenied
	}
	if err := keychainInit(); err != nil {
		return nil, err
	}
	account := C.CString(target)
	defer C.free(unsafe.Pointer(account))
	var length C.UInt32
	var data unsafe.Pointer
	status := C.xpotato_load(account, C.UInt32(len(target)), &length, &data)
	defer C.xpotato_free(length, data)
	if err := keychainError(status); err != nil {
		return nil, err
	}
	if length == 0 || length > 2560 || data == nil {
		return nil, ErrDenied
	}
	return C.GoBytes(data, C.int(length)), nil
}
