package managed

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// A repair publishes a new immutable generation. It never renames, overwrites
// or removes a generation that an existing process may still be using.
func newRepairRuntimeID(m Manifest) (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return releaseID(m) + "-repair-" + hex.EncodeToString(nonce[:]), nil
}

func isRepairRuntimeID(m Manifest, id string) bool {
	prefix := releaseID(m) + "-repair-"
	if !strings.HasPrefix(id, prefix) {
		return false
	}
	suffix := strings.TrimPrefix(id, prefix)
	decoded, err := hex.DecodeString(suffix)
	return err == nil && len(decoded) == 16 && len(suffix) == 32 && strings.ToLower(suffix) == suffix
}
