package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const Version = "0.2.0"

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
var tunnelPattern = regexp.MustCompile(`^tunnel_[a-zA-Z0-9_-]+$`)

type Device struct {
	Schema          int      `json:"schemaVersion"`
	Owner           string   `json:"owner"`
	Name            string   `json:"name"`
	Tunnel          string   `json:"tunnelId"`
	Approved        []string `json:"approvedDirectories"`
	Credential      string   `json:"credentialRef"`
	CredentialReady bool     `json:"credentialReady"`
	ConfigReady     bool     `json:"configReady"`
	Release         string   `json:"release"`
	Previous        string   `json:"previousRelease,omitempty"`
	MCPURL          string   `json:"mcpUrl,omitempty"`
}
type Options struct {
	Home, Device, Tunnel, MCPURL string
	Allow                        []string
}
type Prompt interface {
	Text(string) (string, error)
	Key() ([]byte, error)
	Consent(string) (bool, error)
}
type Runtime interface {
	Install(string) (string, error)
	Verify(string, string) error
	Run(string, Device, []byte) (int, error)
}
type App struct {
	Identity       func() (platform.Identity, error)
	Vault          platform.Vault
	Prompt         Prompt
	Runtime        Runtime
	InstallCommand func(string) error
}

func DefaultHome() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", errors.New("user home unavailable")
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(h, "AppData", "Local", "xpotato-mcp-tunnel-v2"), nil
	}
	if x := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(x) {
		return filepath.Join(x, "xpotato-mcp-tunnel-v2"), nil
	}
	return filepath.Join(h, ".local", "share", "xpotato-mcp-tunnel-v2"), nil
}
func digest(s string) string      { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func directory(o Options) string  { return filepath.Join(o.Home, "devices", o.Device) }
func deviceFile(o Options) string { return filepath.Join(directory(o), "device.json") }
func configFile(o Options) string {
	return filepath.Join(directory(o), "profile", ".claude-server-commander", "config.json")
}
func qualified(p string) bool {
	return filepath.IsAbs(p) && (runtime.GOOS != "windows" || filepath.VolumeName(p) != "")
}
func validateOptions(o Options) error {
	if !qualified(o.Home) || !namePattern.MatchString(o.Device) {
		return errors.New("absolute --home and lowercase device name (letters/numbers/hyphens) required")
	}
	if strings.ContainsAny(o.Home, "'\r\n\x00") || (runtime.GOOS == "windows" && strings.ContainsAny(o.Home, "\"%!")) {
		return errors.New("home contains characters unsupported by the native command/shim; choose another absolute home")
	}
	return nil
}
func (a App) identity() (platform.Identity, error) {
	i, err := a.Identity()
	if err != nil || i.Owner == "" {
		return i, errors.New("owner identity could not be verified")
	}
	if i.Elevated {
		return i, errors.New("run in the owner's normal non-admin/non-root terminal")
	}
	return i, nil
}
func readJSON(file string, v any) error {
	b, e := os.ReadFile(file)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func atomicJSON(file string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return atomicFile(file, append(b, '\n'), 0600)
}
func atomicFile(file string, b []byte, mode os.FileMode) error {
	if e := os.MkdirAll(filepath.Dir(file), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(file), ".write-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	return os.Rename(tmp, file)
}
func exists(file string) bool { _, e := os.Stat(file); return e == nil }
func acquire(file string) (func(), error) {
	if e := os.MkdirAll(filepath.Dir(file), 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, errors.New("operation already active or stale lock remains; inspect doctor before retrying")
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	f.Close()
	return func() { os.Remove(file) }, nil
}
func (a App) owned(o Options) (Device, error) {
	var d Device
	if e := validateOptions(o); e != nil {
		return d, e
	}
	i, e := a.identity()
	if e != nil {
		return d, e
	}
	if e = readJSON(deviceFile(o), &d); e != nil {
		return d, errors.New("prepared device not found; run setup (legacy profiles are not adopted)")
	}
	if d.Schema != 3 || d.Name != o.Device || d.Owner != i.Owner || !tunnelPattern.MatchString(d.Tunnel) || len(d.Approved) == 0 || d.Credential != "xpotato-mcp-tunnel:"+digest(d.Owner+"\x00"+o.Home+"\x00"+d.Name+"\x00"+d.Tunnel) {
		return d, errors.New("device owner/schema/credential reference invalid; existing state was preserved")
	}
	for _, p := range d.Approved {
		if !qualified(p) {
			return d, errors.New("approved directory invalid")
		}
	}
	if d.ConfigReady {
		if e = checkConfig(o, d); e != nil {
			return d, e
		}
	}
	return d, nil
}
func checkConfig(o Options, d Device) error {
	var config struct {
		Allowed []string `json:"allowedDirectories"`
	}
	if readJSON(configFile(o), &config) != nil || len(config.Allowed) == 0 {
		return errors.New("Commander must have non-empty absolute allowedDirectories")
	}
	for _, root := range config.Allowed {
		if !qualified(root) {
			return errors.New("Commander allowed directory is not absolute")
		}
		approved := false
		for _, parent := range d.Approved {
			rel, e := filepath.Rel(parent, root)
			if e == nil && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				approved = true
				break
			}
		}
		if !approved {
			return errors.New("Commander scope exceeds setup-approved directories; create a new device for wider access")
		}
	}
	return nil
}
func validKey(key []byte) bool {
	return len(key) > 0 && len(key) <= 2560 && !strings.ContainsAny(string(key), "\x00\r\n")
}
func wipe(key []byte) {
	for i := range key {
		key[i] = 0
	}
}

// No secrets are stored in state or passed as command-line arguments. Partial
// setup is persisted before credential entry, so failures resume locally.
func (a App) Setup(o Options) (Options, error) {
	i, e := a.identity()
	if e != nil {
		return o, e
	}
	if o.Device == "" {
		o.Device, e = a.Prompt.Text("Device name")
		if e != nil {
			return o, e
		}
	}
	if e = validateOptions(o); e != nil {
		return o, e
	}
	var d Device
	if exists(deviceFile(o)) {
		d, e = a.owned(o)
		if e != nil {
			return o, e
		}
		if (o.Tunnel != "" && o.Tunnel != d.Tunnel) || (o.MCPURL != "" && o.MCPURL != d.MCPURL) || (len(o.Allow) > 0 && !equal(o.Allow, d.Approved)) {
			return o, errors.New("setup preserves existing tunnel, endpoint and approved roots; use a new device for changes")
		}
	} else {
		if exists(configFile(o)) {
			return o, errors.New("orphaned config not adopted; use a new device")
		}
		if o.Tunnel == "" {
			o.Tunnel, e = a.Prompt.Text("Tunnel ID from Platform settings")
			if e != nil {
				return o, e
			}
		}
		if !tunnelPattern.MatchString(o.Tunnel) {
			return o, errors.New("valid tunnel_... identifier required")
		}
		if len(o.Allow) == 0 {
			root, err := a.Prompt.Text("Allowed absolute directory (one; repeat --allow for more)")
			if err != nil {
				return o, err
			}
			o.Allow = []string{root}
		}
		for _, p := range o.Allow {
			st, err := os.Stat(p)
			if !qualified(p) || err != nil || !st.IsDir() {
				return o, errors.New("each allowed directory must exist and be absolute")
			}
		}
		if o.MCPURL != "" {
			if e = validateEndpoint(o.MCPURL); e != nil {
				return o, e
			}
		}
		d = Device{Schema: 3, Owner: i.Owner, Name: o.Device, Tunnel: o.Tunnel, Approved: o.Allow, MCPURL: o.MCPURL}
		d.Credential = "xpotato-mcp-tunnel:" + digest(i.Owner+"\x00"+o.Home+"\x00"+o.Device+"\x00"+o.Tunnel)
	}
	unlock, e := acquire(filepath.Join(directory(o), "maintenance.lock"))
	if e != nil {
		return o, e
	}
	defer unlock()
	// Another setup may have completed between the initial read and lock.
	if exists(deviceFile(o)) {
		current, err := a.owned(o)
		if err != nil {
			return o, err
		}
		if (o.Tunnel != "" && o.Tunnel != current.Tunnel) || (o.MCPURL != "" && o.MCPURL != current.MCPURL) || (len(o.Allow) > 0 && !equal(o.Allow, current.Approved)) {
			return o, errors.New("setup state changed; existing device was preserved")
		}
		d = current
	}
	if !d.ConfigReady {
		shell := "/bin/sh"
		if runtime.GOOS == "windows" {
			shell = "powershell.exe"
		}
		config := map[string]any{"allowedDirectories": d.Approved, "blockedCommands": []string{"shutdown", "reboot", "format", "diskpart", "netsh", "runas", "takeown", "reg", "sc"}, "defaultShell": shell, "fileReadLineLimit": 100, "fileWriteLineLimit": 50, "telemetryEnabled": false}
		if !exists(deviceFile(o)) {
			if e = atomicJSON(deviceFile(o), d); e != nil {
				return o, e
			}
		}
		if !exists(configFile(o)) {
			if e = atomicJSON(configFile(o), config); e != nil {
				return o, e
			}
		}
		if e = checkConfig(o, d); e != nil {
			return o, e
		}
		d.ConfigReady = true
		if e = atomicJSON(deviceFile(o), d); e != nil {
			return o, e
		}
	}
	if d.Release == "" {
		d.Release, e = a.Runtime.Install(o.Home)
		if e != nil {
			return o, e
		}
		if e = atomicJSON(deviceFile(o), d); e != nil {
			return o, e
		}
	}
	if e = a.Runtime.Verify(o.Home, d.Release); e != nil {
		return o, e
	}
	if !d.CredentialReady {
		if e = a.saveKey(o, &d); e != nil {
			return o, e
		}
	}
	if e = Registration(o, d); e != nil {
		return o, e
	}
	if a.InstallCommand != nil {
		if e = a.InstallCommand(o.Home); e != nil {
			return o, e
		}
	}
	if e = atomicJSON(filepath.Join(o.Home, "default.json"), map[string]string{"device": o.Device}); e != nil {
		return o, e
	}
	return o, nil
}
func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func (a App) saveKey(o Options, d *Device) error {
	if e := a.Vault.Ready(); e != nil {
		return e
	}
	yes, e := a.Prompt.Consent("Save/replace only device '" + d.Name + "' runtime key in your user credential store? Type yes")
	if e != nil {
		return e
	}
	if !yes {
		return errors.New("credential save declined; setup state kept for retry")
	}
	key, e := a.Prompt.Key()
	defer wipe(key)
	if e != nil {
		return errors.New("masked key input cancelled or unavailable")
	}
	if !validKey(key) {
		return errors.New("runtime key input invalid")
	}
	if e = a.Vault.Save(d.Credential, key); e != nil {
		return e
	}
	d.CredentialReady = true
	return atomicJSON(deviceFile(o), d)
}
func (a App) Credentials(o Options) error {
	d, e := a.owned(o)
	if e != nil {
		return e
	}
	unlock, e := acquire(filepath.Join(directory(o), "maintenance.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	d, e = a.owned(o)
	if e != nil {
		return e
	}
	return a.saveKey(o, &d)
}
func Resolve(o Options) (Options, error) {
	if o.Home == "" {
		var e error
		o.Home, e = DefaultHome()
		if e != nil {
			return o, e
		}
	}
	if o.Device == "" {
		var d struct{ Device string }
		if readJSON(filepath.Join(o.Home, "default.json"), &d) != nil {
			return o, errors.New("no default device; run setup")
		}
		o.Device = d.Device
	}
	return o, validateOptions(o)
}
func (a App) Start(o Options) (int, error) {
	d, e := a.owned(o)
	if e != nil {
		return 1, e
	}
	if !d.ConfigReady || !d.CredentialReady {
		return 1, errors.New("setup incomplete; run setup")
	}
	if e = a.Runtime.Verify(o.Home, d.Release); e != nil {
		return 1, e
	}
	if exists(filepath.Join(directory(o), "maintenance.lock")) {
		return 1, errors.New("setup/update in progress")
	}
	unlock, e := acquire(filepath.Join(directory(o), "start.lock"))
	if e != nil {
		return 1, e
	}
	defer unlock()
	if exists(filepath.Join(directory(o), "maintenance.lock")) {
		return 1, errors.New("setup/update in progress")
	}
	tunlock, e := acquire(filepath.Join(o.Home, "tunnel-locks", digest(d.Tunnel)+".lock"))
	if e != nil {
		return 1, e
	}
	defer tunlock()
	if e = a.Vault.Ready(); e != nil {
		return 1, e
	}
	key, e := a.Vault.Load(d.Credential)
	defer wipe(key)
	if e != nil {
		return 1, e
	}
	if !validKey(key) {
		return 1, errors.New("saved runtime key invalid; run credentials")
	}
	return a.Runtime.Run(o.Home, d, key)
}
func (a App) Update(o Options, rollback bool) error {
	d, e := a.owned(o)
	if e != nil {
		return e
	}
	unlock, e := acquire(filepath.Join(directory(o), "maintenance.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	d, e = a.owned(o)
	if e != nil {
		return e
	}
	var next string
	if rollback {
		next = d.Previous
		if next == "" {
			return errors.New("no previous runtime")
		}
	} else {
		next, e = a.Runtime.Install(o.Home)
		if e != nil {
			return e
		}
	}
	if e = a.Runtime.Verify(o.Home, next); e != nil {
		return e
	}
	if next != d.Release {
		d.Previous, d.Release = d.Release, next
		return atomicJSON(deviceFile(o), d)
	}
	return nil
}
func (a App) Doctor(o Options) (map[string]any, error) {
	d, e := a.owned(o)
	if e != nil {
		return nil, e
	}
	if e = a.Runtime.Verify(o.Home, d.Release); e != nil {
		return nil, e
	}
	return map[string]any{"integrity": "ok", "credentials": "not-read", "connection": "not-probed", "credentialConfigured": d.CredentialReady, "pluginAccountRegistration": "not-probed", "release": d.Release, "locks": map[string]bool{"start": exists(filepath.Join(directory(o), "start.lock")), "maintenance": exists(filepath.Join(directory(o), "maintenance.lock")), "tunnel": exists(filepath.Join(o.Home, "tunnel-locks", digest(d.Tunnel)+".lock"))}}, nil
}
