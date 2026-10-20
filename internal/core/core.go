package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/atomicfile"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/lockfile"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const Version = "0.2.10"

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
var tunnelPattern = regexp.MustCompile(`^tunnel_[a-zA-Z0-9_-]+$`)

var ErrRuntimePinUnsupported = errors.New("このコアで未対応の runtime 版です")
var ErrCommandDirectoryConflict = errors.New("既存の短縮コマンド保存先を変更できません")

type Device struct {
	Core               *CorePin `json:"core,omitempty"`
	PreviousCore       *CorePin `json:"previousCore,omitempty"`
	ProfileRevision    string   `json:"profileRevision,omitempty"`
	CredentialRevision string   `json:"credentialRevision,omitempty"`
	Schema             int      `json:"schemaVersion"`
	Owner              string   `json:"owner"`
	Name               string   `json:"name"`
	Tunnel             string   `json:"tunnelId"`
	Approved           []string `json:"approvedDirectories"`
	Credential         string   `json:"credentialRef"`
	CredentialReady    bool     `json:"credentialReady"`
	ConfigReady        bool     `json:"configReady"`
	Release            string   `json:"release"`
	Previous           string   `json:"previousRelease,omitempty"`
	MCPURL             string   `json:"mcpUrl,omitempty"`
}
type Options struct {
	ActivityDetails                          bool
	Diagnostics                              bool
	EnvFile                                  string
	reviewed                                 *setupReview
	replaceScope                             bool
	previewProfile                           string
	Home, Device, Tunnel, MCPURL, CommandDir string
	Allow                                    []string
	// Set only by the reviewed setup plan, never by a CLI flag.
	RefreshRuntime bool
}
type CommandPlan struct {
	Directory       string
	ConsentRequired bool
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
	Context        context.Context
	PrepareUpdate  func(context.Context, string) (CommandUpdate, error)
	InstalledCore  func(string) (CorePin, error)
	RunningCore    func(context.Context) (CorePin, error)
	VerifyCore     func(context.Context, string, CorePin) error
	writeDevice    func(string, Device) error
	Progress       func(string)
	Identity       func() (platform.Identity, error)
	Vault          platform.Vault
	Prompt         Prompt
	Runtime        Runtime
	InstallCommand func(string) error
	PreviewCommand func(string, string) (CommandPlan, error)
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
	return atomicJSONContext(context.Background(), file, v)
}
func atomicJSONContext(ctx context.Context, file string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return atomicfile.Write(ctx, file, append(b, '\n'), 0600)
}
func atomicFile(file string, b []byte, mode os.FileMode) error {
	return atomicfile.Write(context.Background(), file, b, mode)
}
func exists(file string) bool                    { _, e := os.Stat(file); return e == nil }
func acquire(file, owner string) (func(), error) { return lockfile.Acquire(file, owner) }
func busy(file string) bool                      { active, e := lockfile.Active(file); return active || e != nil }
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
	if d.Schema != 3 || d.Name != o.Device || d.Owner != i.Owner || !tunnelPattern.MatchString(d.Tunnel) || len(d.Approved) == 0 || !validRevisions(d) || d.Credential != credentialReference(o.Home, d) {
		return d, errors.New("device owner/schema/credential reference invalid; existing state was preserved")
	}
	if (d.Core != nil && !d.Core.Valid()) || (d.PreviousCore != nil && !d.PreviousCore.Valid()) {
		return d, errors.New("core generation metadata invalid; existing state preserved")
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
	if readJSON(profileConfig(o.Home, d), &config) != nil || len(config.Allowed) == 0 {
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
			return errors.New("Commander scope exceeds setup-approved directories; review changes with setup")
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
	if o.reviewed != nil && !o.reviewed.matches(o) {
		return o, errors.New("確認後に入力設定が変わりました。Setup で再確認してください")
	}
	if o.reviewed != nil && o.reviewed.changed {
		return a.reconfigure(o)
	}
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
			return o, errors.New("既存設定の変更には Setup の差分確認と明示的な yes が必要です")
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
	unlock, e := acquire(filepath.Join(directory(o), "maintenance.lock"), i.Owner)
	if e != nil {
		return o, e
	}
	defer unlock()
	if o.reviewed != nil && !o.reviewed.unchanged(o) {
		return o, errors.New("確認後に保存済み設定が変わりました。Setup で再確認してください")
	}
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
		if !exists(profileConfig(o.Home, d)) {
			if e = atomicJSON(profileConfig(o.Home, d), config); e != nil {
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
	pendingRelease := ""
	if d.Release == "" || o.RefreshRuntime {
		next, err := a.Runtime.Install(o.Home)
		e = err
		if e != nil {
			return o, e
		}
		if e = a.Runtime.Verify(o.Home, next); e != nil {
			return o, e
		}
		if d.Release != "" && d.Release != next {
			// Keep the old entry point usable until the new core and shims
			// install successfully, matching the explicit Update ordering.
			pendingRelease = next
		} else {
			d.Release = next
			if e = atomicJSON(deviceFile(o), d); e != nil {
				return o, e
			}
		}
	}
	verifyRelease := d.Release
	if pendingRelease != "" {
		verifyRelease = pendingRelease
	}
	if e = a.Runtime.Verify(o.Home, verifyRelease); e != nil {
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
	if pendingRelease != "" {
		d.Previous, d.Release = d.Release, pendingRelease
		d.PreviousCore = d.Core
	}
	if a.InstalledCore != nil {
		pin, err := a.InstalledCore(o.Home)
		if err != nil {
			return o, err
		}
		if d.Core == nil {
			d.Core = &pin
		} else if *d.Core != pin {
			if pendingRelease == "" {
				d.Previous = d.Release
				d.PreviousCore = d.Core
			}
			d.Core = &pin
		}
	}
	if pendingRelease != "" || a.InstalledCore != nil {
		if e = a.saveDevice(o, d); e != nil {
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
	if e := a.saveKeyRecord(d); e != nil {
		return e
	}
	return atomicJSON(deviceFile(o), d)
}
func (a App) saveKeyRecord(d *Device) error {
	if e := a.Vault.Ready(); e != nil {
		return e
	}
	yes, e := a.Prompt.Consent("端末「" + d.Name + "」専用の runtime API キーを、本人の OS 資格情報ストアの一レコードに保存／置換します。他の資格情報は変更しません。保存に同意しますか？ [yes / no]")
	if e != nil {
		return e
	}
	if !yes {
		return errors.New("キーの保存を中断しました。途中の設定は保持され、同じ setup で再開できます")
	}
	key, e := a.Prompt.Key()
	defer wipe(key)
	if e != nil {
		return errors.New("非表示のキー入力を中断したか、入力端末を利用できません")
	}
	if !validKey(key) {
		return errors.New("キー入力が無効です。表示・保存はしていません。同じ setup で再入力できます")
	}
	if e = a.Vault.Save(d.Credential, key); e != nil {
		return e
	}
	d.CredentialReady = true
	return nil
}
func (a App) Credentials(o Options) error {
	d, e := a.owned(o)
	if e != nil {
		return e
	}
	unlock, e := acquire(filepath.Join(directory(o), "maintenance.lock"), d.Owner)
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
func (a App) Start(o Options) (code int, err error) {
	stage := "保存設定・許可範囲・実行ユーザーの確認"
	report := func(message string) {
		stage = message
		if a.Progress != nil {
			a.Progress(message)
		}
	}
	defer func() {
		if err != nil && a.Progress != nil {
			a.Progress(stage + " — Start の処理に失敗しました")
		}
	}()
	report(stage)
	d, e := a.owned(o)
	if e != nil {
		return 1, e
	}
	if !d.ConfigReady || !d.CredentialReady {
		return 1, errors.New("setup incomplete; run setup")
	}
	if busy(filepath.Join(directory(o), "maintenance.lock")) {
		return 1, errors.New("setup/update in progress")
	}
	unlock, e := acquire(filepath.Join(directory(o), "start.lock"), d.Owner)
	if e != nil {
		return 1, e
	}
	defer unlock()
	if busy(filepath.Join(directory(o), "maintenance.lock")) {
		return 1, errors.New("setup/update in progress")
	}
	// Setup may have completed after the initial read but before start.lock.
	// Use the latest identity/scope/credential/profile for every launch.
	d, e = a.owned(o)
	if e != nil {
		return 1, e
	}
	if d.Core != nil && d.Core.Version != Version {
		return 1, errors.New("selected core generation changed; retry the installed short command")
	}
	if d.Core != nil && a.RunningCore != nil {
		pin, err := a.RunningCore(a.context())
		if err != nil {
			return 1, err
		}
		if pin != *d.Core {
			return 1, errors.New("running core SHA256 differs from selected generation; retry the installed short command")
		}
	}
	if e = a.context().Err(); e != nil {
		return 1, e
	}
	if !d.ConfigReady || !d.CredentialReady {
		return 1, errors.New("setup incomplete; run setup")
	}
	report("固定 runtime の整合性を確認")
	if e = a.Runtime.Verify(o.Home, d.Release); e != nil {
		return 1, e
	}
	if e = a.context().Err(); e != nil {
		return 1, e
	}
	report("Tunnel の二重起動を確認")
	tunlock, e := acquire(filepath.Join(o.Home, "tunnel-locks", digest(d.Tunnel)+".lock"), d.Owner)
	if e != nil {
		return 1, e
	}
	defer tunlock()
	report("本人の資格情報ストアの利用可否を確認")
	if e = a.context().Err(); e != nil {
		return 1, e
	}
	if e = a.Vault.Ready(); e != nil {
		return 1, e
	}
	report("この端末の保存済み資格情報を取得（値は表示しません）")
	if e = a.context().Err(); e != nil {
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
	if e = a.context().Err(); e != nil {
		return 1, e
	}
	report("Tunnel 子プロセスを準備")
	return a.Runtime.Run(o.Home, d, key)
}
func (a App) Update(o Options, rollback bool) error {
	d, e := a.owned(o)
	if e != nil {
		return e
	}
	unlock, e := acquire(filepath.Join(directory(o), "maintenance.lock"), d.Owner)
	if e != nil {
		return e
	}
	defer unlock()
	d, e = a.owned(o)
	if e != nil {
		return e
	}
	if a.PrepareUpdate != nil {
		return a.updateGeneration(o, d, rollback)
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
	if !rollback && a.InstallCommand != nil {
		if e = a.InstallCommand(o.Home); e != nil {
			return e
		}
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
	integrity := "not-installed"
	if d.Release != "" {
		integrity = "ok"
		if e = a.Runtime.Verify(o.Home, d.Release); e != nil {
			integrity = "failed"
		}
	}
	return map[string]any{"integrity": integrity, "credentials": "not-read", "connection": "not-probed", "credentialConfigured": d.CredentialReady, "pluginAccountRegistration": "not-probed", "release": d.Release, "locks": map[string]bool{"start": busy(filepath.Join(directory(o), "start.lock")), "maintenance": busy(filepath.Join(directory(o), "maintenance.lock")), "tunnel": busy(filepath.Join(o.Home, "tunnel-locks", digest(d.Tunnel)+".lock")), "install": busy(filepath.Join(o.Home, "releases", "install.lock"))}}, nil
}
func (a App) DoctorHome(home string) (map[string]any, error) {
	if _, e := a.identity(); e != nil {
		return nil, e
	}
	if e := validateOptions(Options{Home: home, Device: "doctor"}); e != nil {
		return nil, e
	}
	devices := []string{}
	entries, e := os.ReadDir(filepath.Join(home, "devices"))
	if e != nil && !os.IsNotExist(e) {
		return nil, e
	}
	for _, entry := range entries {
		if entry.IsDir() && namePattern.MatchString(entry.Name()) {
			devices = append(devices, entry.Name())
		}
	}
	return map[string]any{"credentials": "not-read", "connection": "not-probed", "devices": devices, "defaultDevice": "not-probed", "locks": map[string]bool{"install": busy(filepath.Join(home, "releases", "install.lock"))}}, nil
}
func (a App) Plugin(o Options) error {
	d, e := a.owned(o)
	if e != nil {
		return e
	}
	if e = a.Runtime.Verify(o.Home, d.Release); e != nil {
		return e
	}
	return Registration(o, d)
}
