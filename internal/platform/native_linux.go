//go:build linux

package platform

import (
	"context"
	"github.com/godbus/dbus/v5"
	"os"
	"strconv"
	"time"
)

func CurrentIdentity() (Identity, error) {
	return Identity{Owner: "linux:" + strconv.Itoa(os.Getuid()), Elevated: os.Geteuid() == 0 || os.Getuid() != os.Geteuid()}, nil
}

const service = "org.freedesktop.secrets"
const secretRoot dbus.ObjectPath = "/org/freedesktop/secrets"

type secret struct {
	Session           dbus.ObjectPath
	Parameters, Value []byte
	ContentType       string
}
type NativeVault struct{}

// Connect only to the user's Unix session bus. No daemon autolaunch, service
// activation, collection creation, Unlock, or Prompt calls are performed.
func secretConnection() (*dbus.Conn, dbus.ObjectPath, error) {
	address := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if len(address) < 5 || address[:5] != "unix:" {
		return nil, "", ErrUnavailable
	}
	c, err := dbus.Connect(address)
	if err != nil {
		return nil, "", ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var present bool
	if c.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, service).Store(&present) != nil || !present {
		c.Close()
		return nil, "", ErrUnavailable
	}
	var collection dbus.ObjectPath
	if c.Object(service, secretRoot).CallWithContext(ctx, "org.freedesktop.Secret.Service.ReadAlias", 0, "default").Store(&collection) != nil || collection == "/" {
		c.Close()
		return nil, "", ErrUnavailable
	}
	var locked dbus.Variant
	if c.Object(service, collection).CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, "org.freedesktop.Secret.Collection", "Locked").Store(&locked) != nil {
		c.Close()
		return nil, "", ErrUnavailable
	}
	value, valid := locked.Value().(bool)
	if !valid {
		c.Close()
		return nil, "", ErrUnavailable
	}
	if value {
		c.Close()
		return nil, "", ErrLocked
	}
	return c, collection, nil
}
func (NativeVault) Ready() error {
	c, _, err := secretConnection()
	if c != nil {
		c.Close()
	}
	return err
}
func attributes(target string) map[string]string {
	return map[string]string{"application": "xpotato-mcp-tunnel", "record": target}
}
func secretSession(c *dbus.Conn, ctx context.Context) (dbus.ObjectPath, error) {
	var output dbus.Variant
	var session dbus.ObjectPath
	err := c.Object(service, secretRoot).CallWithContext(ctx, "org.freedesktop.Secret.Service.OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&output, &session)
	if err != nil || session == "/" {
		return "", ErrDenied
	}
	return session, nil
}
func (NativeVault) Save(target string, key []byte) error {
	c, collection, err := secretConnection()
	if err != nil {
		return err
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := secretSession(c, ctx)
	if err != nil {
		return err
	}
	defer c.Object(service, session).CallWithContext(ctx, "org.freedesktop.Secret.Session.Close", 0)
	props := map[string]dbus.Variant{"org.freedesktop.Secret.Item.Label": dbus.MakeVariant("Xpotato runtime key"), "org.freedesktop.Secret.Item.Attributes": dbus.MakeVariant(attributes(target))}
	var item, prompt dbus.ObjectPath
	err = c.Object(service, collection).CallWithContext(ctx, "org.freedesktop.Secret.Collection.CreateItem", 0, props, secret{session, []byte{}, key, "text/plain; charset=utf8"}, true).Store(&item, &prompt)
	if err != nil || prompt != "/" || item == "/" {
		return ErrDenied
	}
	return nil
}
func (NativeVault) Load(target string) ([]byte, error) {
	c, collection, err := secretConnection()
	if err != nil {
		return nil, err
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var items []dbus.ObjectPath
	if c.Object(service, collection).CallWithContext(ctx, "org.freedesktop.Secret.Collection.SearchItems", 0, attributes(target)).Store(&items) != nil {
		return nil, ErrDenied
	}
	if len(items) == 0 {
		return nil, ErrMissing
	}
	if len(items) != 1 {
		return nil, ErrDenied
	}
	var locked dbus.Variant
	if c.Object(service, items[0]).CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, "org.freedesktop.Secret.Item", "Locked").Store(&locked) != nil {
		return nil, ErrDenied
	}
	if b, ok := locked.Value().(bool); !ok || b {
		return nil, ErrLocked
	}
	session, err := secretSession(c, ctx)
	if err != nil {
		return nil, err
	}
	defer c.Object(service, session).CallWithContext(ctx, "org.freedesktop.Secret.Session.Close", 0)
	var value secret
	if c.Object(service, items[0]).CallWithContext(ctx, "org.freedesktop.Secret.Item.GetSecret", 0, session).Store(&value) != nil || value.Session != session || len(value.Parameters) != 0 || len(value.Value) == 0 {
		return nil, ErrDenied
	}
	return value.Value, nil
}
