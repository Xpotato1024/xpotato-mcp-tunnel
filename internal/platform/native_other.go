//go:build !windows && !linux

package platform

func CurrentIdentity() (Identity, error) { return Identity{}, ErrUnavailable }

type NativeVault struct{}

func (NativeVault) Ready() error                { return ErrUnavailable }
func (NativeVault) Save(string, []byte) error   { return ErrUnavailable }
func (NativeVault) Load(string) ([]byte, error) { return nil, ErrUnavailable }
