package core

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
)

var profileRevisionPattern = regexp.MustCompile("^[a-f0-9]{64}$")
var credentialRevisionPattern = regexp.MustCompile("^[a-f0-9]{32}$")

func validRevisions(d Device) bool {
	return (d.ProfileRevision == "" || profileRevisionPattern.MatchString(d.ProfileRevision)) &&
		(d.CredentialRevision == "" || credentialRevisionPattern.MatchString(d.CredentialRevision))
}

func ProfilePath(home string, d Device) string {
	if d.ProfileRevision != "" {
		return filepath.Join(home, "devices", d.Name, "profiles", d.ProfileRevision)
	}
	return filepath.Join(home, "devices", d.Name, "profile")
}
func profileConfig(home string, d Device) string {
	return filepath.Join(ProfilePath(home, d), ".claude-server-commander", "config.json")
}
func credentialReference(home string, d Device) string {
	s := d.Owner + "\x00" + home + "\x00" + d.Name + "\x00" + d.Tunnel
	if d.CredentialRevision != "" {
		s += "\x00" + d.CredentialRevision
	}
	return "xpotato-mcp-tunnel:" + digest(s)
}

// Unexported capability binds confirmation to both current state and the
// desired nonsecret values. Neither CLI flags nor an env file can grant it.
type setupReview struct {
	home, device                         string
	before, next                         Device
	deviceBytes, configBytes, nextConfig []byte
	effective                            []string
	changed, scopeChanged                bool
}

func reviewExisting(o Options, d Device) (*setupReview, error) {
	r := &setupReview{home: o.Home, device: o.Device, before: d, next: d}
	var e error
	r.deviceBytes, e = os.ReadFile(deviceFile(o))
	if e != nil {
		return nil, e
	}
	var captured Device
	if json.Unmarshal(r.deviceBytes, &captured) != nil || !reflect.DeepEqual(captured, d) {
		return nil, errors.New("確認中に端末設定が変わりました。Setup で再確認してください")
	}
	// Incomplete configurations are resumed by the ordinary Setup path.
	if !d.ConfigReady {
		return nil, errors.New("既存の設定作成が未完了です。同じ値で Setup を完了してから変更してください")
	}
	r.configBytes, e = os.ReadFile(profileConfig(o.Home, d))
	if e != nil {
		return nil, e
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(r.configBytes, &config) != nil || json.Unmarshal(config["allowedDirectories"], &r.effective) != nil {
		return nil, errors.New("既存の非秘密 Commander 設定を確認できません")
	}
	r.next.Tunnel, r.next.MCPURL = o.Tunnel, o.MCPURL
	r.next.Approved = append([]string(nil), o.Allow...)
	r.scopeChanged = (o.EnvFile != "" || o.replaceScope || !equal(o.Allow, d.Approved)) && !equal(o.Allow, r.effective)
	if r.scopeChanged {
		config["allowedDirectories"], e = json.Marshal(o.Allow)
		if e != nil {
			return nil, e
		}
		r.nextConfig, e = json.MarshalIndent(config, "", "  ")
		if e != nil {
			return nil, e
		}
		r.nextConfig = append(r.nextConfig, '\n')
		r.next.ProfileRevision = digest(string(r.nextConfig))
	}
	r.changed = r.scopeChanged || o.Tunnel != d.Tunnel || o.MCPURL != d.MCPURL || !equal(o.Allow, d.Approved)
	return r, nil
}

func (r *setupReview) print(out io.Writer) {
	if !r.changed {
		return
	}
	fmt.Fprintln(out, "既存端末の変更差分（yes までは保存しません）:")
	fmt.Fprintf(out, "  Tunnel ID: %s → %s\n", r.before.Tunnel, r.next.Tunnel)
	fmt.Fprintln(out, "  変更前の実際の許可フォルダー:")
	printRoots(out, r.effective)
	fmt.Fprintln(out, "  変更後の承認フォルダー:")
	printRoots(out, r.next.Approved)
	if r.scopeChanged {
		fmt.Fprintln(out, "  Commander の実際の範囲も上の変更後一覧に切り替えます。範囲の追加・拡張を含めて確認してください。")
	} else {
		fmt.Fprintln(out, "  Commander の既存の絞り込みは保持します。")
	}
	if r.before.Tunnel != r.next.Tunnel {
		fmt.Fprintln(out, "  Tunnel ID を変更するため、専用の新しいキー保存先への同意と非表示入力が必要です。以前のキーは保持します。")
	}
	fmt.Fprintln(out, "  起動済みなら変更を拒否します。Setup は接続を起動・停止しません。")
}

func (r *setupReview) matches(o Options) bool {
	return o.Home == r.home && o.Device == r.device && o.Tunnel == r.next.Tunnel &&
		o.MCPURL == r.next.MCPURL && equal(o.Allow, r.next.Approved)
}
func (r *setupReview) unchanged(o Options) bool {
	d, e := os.ReadFile(deviceFile(o))
	c, ce := os.ReadFile(profileConfig(o.Home, r.before))
	return e == nil && ce == nil && bytes.Equal(d, r.deviceBytes) && bytes.Equal(c, r.configBytes)
}

func (a App) reconfigure(o Options) (Options, error) {
	r := o.reviewed
	if !r.matches(o) {
		return o, errors.New("確認後に入力設定が変わりました。保存せず Setup で再確認してください")
	}
	if e := validateRegistration(r.next); e != nil {
		return o, e
	}
	i, e := a.identity()
	if e != nil {
		return o, e
	}
	unlock, e := acquire(filepath.Join(directory(o), "maintenance.lock"), i.Owner)
	if e != nil {
		return o, e
	}
	defer unlock()
	startUnlock, e := acquire(filepath.Join(directory(o), "start.lock"), i.Owner)
	if e != nil {
		return o, errors.New("端末が起動中か起動確認できません。変更していません")
	}
	defer startUnlock()
	d, e := a.owned(o)
	if e != nil {
		return o, e
	}
	if !r.unchanged(o) {
		return o, errors.New("確認後に保存済み設定が変わりました。変更せず Setup で再確認してください")
	}
	ids := []string{d.Tunnel}
	if r.next.Tunnel != d.Tunnel {
		ids = append(ids, r.next.Tunnel)
	}
	sort.Strings(ids)
	var releases []func()
	defer func() {
		for n := len(releases) - 1; n >= 0; n-- {
			releases[n]()
		}
	}()
	for _, id := range ids {
		release, err := acquire(filepath.Join(o.Home, "tunnel-locks", digest(id)+".lock"), i.Owner)
		if err != nil {
			return o, errors.New("変更前または変更後の Tunnel が使用中です。変更していません")
		}
		releases = append(releases, release)
	}
	next := r.next
	if r.scopeChanged {
		file := profileConfig(o.Home, next)
		if b, err := os.ReadFile(file); err == nil {
			if !bytes.Equal(b, r.nextConfig) {
				return o, errors.New("変更先 profile が既に異なる内容です。旧設定を保持しました")
			}
		} else if !os.IsNotExist(err) {
			return o, err
		} else if e = atomicFile(file, r.nextConfig, 0600); e != nil {
			return o, e
		}
	}
	if o.RefreshRuntime || next.Release == "" {
		release, err := a.Runtime.Install(o.Home)
		if err != nil {
			return o, err
		}
		if e = a.Runtime.Verify(o.Home, release); e != nil {
			return o, e
		}
		if release != next.Release {
			next.Previous, next.Release = next.Release, release
		}
	}
	if e = a.Runtime.Verify(o.Home, next.Release); e != nil {
		return o, e
	}
	if next.Tunnel != d.Tunnel {
		nonce := make([]byte, 16)
		if _, e = rand.Read(nonce); e != nil {
			return o, e
		}
		next.CredentialRevision = hex.EncodeToString(nonce)
		next.Credential = credentialReference(o.Home, next)
		next.CredentialReady = false
	}
	if !next.CredentialReady {
		// Save only the new record. Do not publish the device/profile yet.
		if e = a.saveKeyRecord(&next); e != nil {
			return o, e
		}
	}
	if a.InstallCommand != nil {
		if e = a.InstallCommand(o.Home); e != nil {
			return o, e
		}
	}
	if a.InstalledCore != nil {
		pin, err := a.InstalledCore(o.Home)
		if err != nil {
			return o, err
		}
		if next.Core == nil || *next.Core != pin || next.Release != d.Release {
			next.Previous = d.Release
			next.PreviousCore = d.Core
			next.Core = &pin
		}
	}
	if !r.unchanged(o) {
		return o, errors.New("保存処理中に旧設定が変わりました。旧端末の参照は保持しました")
	}
	// Single atomic publication: old profile and credential records remain.
	if e = a.saveDevice(o, next); e != nil {
		return o, e
	}
	if e = Registration(o, next); e != nil {
		return o, errors.New("端末設定の更新は保存済みですが、登録案内の保存が完了していません。同じ Setup で再開してください")
	}
	if e = atomicJSON(filepath.Join(o.Home, "default.json"), map[string]string{"device": o.Device}); e != nil {
		return o, errors.New("端末設定の更新は保存済みですが、既定端末の保存が完了していません。同じ Setup で再開してください")
	}
	return o, nil
}
