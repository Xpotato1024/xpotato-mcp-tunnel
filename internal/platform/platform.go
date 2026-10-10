package platform

import "errors"

var (
	ErrUnavailable = errors.New("credential service unavailable; configure an unlocked user credential service, then retry setup/start")
	ErrLocked      = errors.New("credential collection is locked; unlock it in your desktop session, then retry")
	ErrMissing     = errors.New("saved runtime key not found; run credentials to replace this device's key")
	ErrDenied      = errors.New("credential operation failed; no plaintext fallback was used")
)

// Vault accesses only the application's exact owner/device record. Ready reads
// service/collection metadata, never the key. Neither backend enumerates keys.
type Vault interface {
	Ready() error
	Save(string, []byte) error
	Load(string) ([]byte, error)
}

type Identity struct {
	Owner    string
	Elevated bool
}
