//go:build linux

package platform

import (
	"bufio"
	"errors"
	"github.com/godbus/dbus/v5"
	"os/exec"
	"strings"
	"testing"
)

// This private bus and synthetic service cannot access the desktop's real keys.
// CI supplies dbus-daemon; locally absence is reported as a skipped integration.
type fakeSecret struct {
	locked                      bool
	key                         []byte
	sessionCalls, reads, writes int
}

func (f *fakeSecret) ReadAlias(string) (dbus.ObjectPath, *dbus.Error) {
	return "/fixture/collection", nil
}
func (f *fakeSecret) OpenSession(string, dbus.Variant) (dbus.Variant, dbus.ObjectPath, *dbus.Error) {
	f.sessionCalls++
	return dbus.MakeVariant(""), "/fixture/session", nil
}
func (f *fakeSecret) Get(iface, prop string) (dbus.Variant, *dbus.Error) {
	return dbus.MakeVariant(f.locked), nil
}
func (f *fakeSecret) CreateItem(_ map[string]dbus.Variant, s secret, _ bool) (dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	f.writes++
	f.key = append([]byte(nil), s.Value...)
	return "/fixture/item", "/", nil
}
func (f *fakeSecret) SearchItems(map[string]string) ([]dbus.ObjectPath, *dbus.Error) {
	if len(f.key) == 0 {
		return []dbus.ObjectPath{}, nil
	}
	return []dbus.ObjectPath{"/fixture/item"}, nil
}
func (f *fakeSecret) GetSecret(s dbus.ObjectPath) (secret, *dbus.Error) {
	f.reads++
	return secret{s, []byte{}, f.key, "text/plain"}, nil
}
func (f *fakeSecret) Close() *dbus.Error { return nil }
func privateService(t *testing.T) *fakeSecret {
	t.Helper()
	binary, e := exec.LookPath("dbus-daemon")
	if e != nil {
		t.Skip("dbus-daemon unavailable for private test bus")
	}
	cmd := exec.Command(binary, "--session", "--nofork", "--print-address=1")
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	address, e := bufio.NewReader(stdout).ReadString('\n')
	if e != nil {
		t.Fatal(e)
	}
	address = strings.TrimSpace(address)
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", address)
	c, e := dbus.Connect(address)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { c.Close() })
	reply, e := c.RequestName(service, dbus.NameFlagDoNotQueue)
	if e != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatal("private service registration failed")
	}
	f := &fakeSecret{}
	for _, entry := range []struct {
		path  dbus.ObjectPath
		iface string
	}{{secretRoot, "org.freedesktop.Secret.Service"}, {"/fixture/collection", "org.freedesktop.Secret.Collection"}, {"/fixture/collection", "org.freedesktop.DBus.Properties"}, {"/fixture/item", "org.freedesktop.Secret.Item"}, {"/fixture/item", "org.freedesktop.DBus.Properties"}, {"/fixture/session", "org.freedesktop.Secret.Session"}} {
		if e = c.Export(f, entry.path, entry.iface); e != nil {
			t.Fatal(e)
		}
	}
	return f
}
func TestLinuxPrivateSecretServiceSaveLoad(t *testing.T) {
	f := privateService(t)
	v := NativeVault{}
	if e := v.Ready(); e != nil {
		t.Fatal(e)
	}
	if f.reads != 0 || f.sessionCalls != 0 {
		t.Fatal("readiness accessed secret/session")
	}
	if e := v.Save("fixture-record", []byte("synthetic-key")); e != nil {
		t.Fatal(e)
	}
	key, e := v.Load("fixture-record")
	if e != nil || string(key) != "synthetic-key" {
		t.Fatal(e)
	}
	if f.writes != 1 || f.reads != 1 {
		t.Fatal("unexpected operation count")
	}
}
func TestLinuxLockedCollectionNeverOpensSecretSession(t *testing.T) {
	f := privateService(t)
	f.locked = true
	v := NativeVault{}
	if e := v.Save("fixture", []byte("synthetic-key")); !errors.Is(e, ErrLocked) {
		t.Fatal(e)
	}
	if _, e := v.Load("fixture"); !errors.Is(e, ErrLocked) {
		t.Fatal(e)
	}
	if f.writes != 0 || f.reads != 0 || f.sessionCalls != 0 {
		t.Fatal("locked collection touched")
	}
}
func TestLinuxMissingKeyDoesNotRetrieveSecret(t *testing.T) {
	f := privateService(t)
	if _, e := (NativeVault{}).Load("fixture"); !errors.Is(e, ErrMissing) {
		t.Fatal(e)
	}
	if f.reads != 0 || f.sessionCalls != 0 {
		t.Fatal("missing key opened session")
	}
}
func TestLinuxHeadlessFailsWithoutPlaintextFallback(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	if e := (NativeVault{}).Ready(); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
}
