package managed

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/lockfile"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/processctrl"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
)

//go:embed assets/stdio.mjs assets/activity.mjs assets/package-lock.json
var assets embed.FS

const nodeVersion = "24.14.0"
const commanderVersion = "0.2.52"
const tunnelVersion = "0.0.16"
const dependencyLockSHA = "24a35c65bbc5ba7996ed70505bc60795f44e9d29f924451da0409f01a639d65d"

type Manifest struct{ Platform, NodeURL, NodeSHA, TunnelURL, TunnelSHA, ExecutableSHA string }

func manifest() (Manifest, error) {
	if runtime.GOARCH != "amd64" {
		return Manifest{}, errors.New("this runtime bundle supports Windows/Linux amd64 only")
	}
	base := "https://github.com/openai/tunnel-client/releases/download/v0.0.16/tunnel-client-runtime-v0.0.16-"
	switch runtime.GOOS {
	case "windows":
		return Manifest{"windows-amd64", "https://nodejs.org/dist/v24.14.0/node-v24.14.0-win-x64.zip", "313fa40c0d7b18575821de8cb17483031fe07d95de5994f6f435f3b345f85c66", base + "windows-amd64.zip", "639fdfe10d91c672d8f8a49386ebf39acaac4e560a88134735b1739a02ddb82d", "19e7970f825e0180546acba4c73ed1163a85f8d6fdd4393ff0854458a642d1b8"}, nil
	case "linux":
		return Manifest{"linux-amd64", "https://nodejs.org/dist/v24.14.0/node-v24.14.0-linux-x64.tar.gz", "dbf5b8665dec15e59e6359a517fefb47b23fdb9152d8def975b9bca3dfc6d355", base + "linux-amd64.zip", "f8f48017e752810c2c6f02065cdc4ca1ad045b91a6d045cbab144b957fdcbdb1", "35183d94445c4c11a9d2f2aba52d23b3581c3580bf3998226a33a904c85f37d0"}, nil
	}
	return Manifest{}, errors.New("unsupported OS")
}
func releaseID(m Manifest) string {
	return "core-" + core.Version + "-" + m.Platform + "-tunnel-" + tunnelVersion + "-dc-" + commanderVersion + "-node-" + nodeVersion
}
func executable() string {
	if runtime.GOOS == "windows" {
		return "tunnel-client-runtime.exe"
	}
	return "tunnel-client-runtime"
}
func nodeFiles(dir string) (string, string) {
	if runtime.GOOS == "windows" {
		return filepath.Join(dir, "node", "node.exe"), filepath.Join(dir, "node", "node_modules", "npm", "bin", "npm-cli.js")
	}
	return filepath.Join(dir, "node", "bin", "node"), filepath.Join(dir, "node", "lib", "node_modules", "npm", "bin", "npm-cli.js")
}
func hashFile(file string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fileOperationTimeout)
	defer cancel()
	return hashFileContext(ctx, file)
}
func hashFileContext(ctx context.Context, file string) (string, error) {
	if e := ctx.Err(); e != nil {
		return "", e
	}
	f, e := os.Open(file)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, contextReader{ctx, f}); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func hashBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

type Receipt struct {
	Manifest Manifest
	Files    map[string]string
}
type Manager struct {
	Progress        func(string)
	ActivityDetails bool
	ActivityColor   bool
}

func (m Manager) stage(label string, work func() error) error {
	return runStage(m.Progress, label, progressInterval, work)
}

func download(url, file, expected string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "GET", url, nil)
	if e != nil {
		return errors.New("runtime download request invalid")
	}
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		return errors.New("official runtime download failed; retry setup")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("official runtime download did not return HTTP 200")
	}
	f, e := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = io.Copy(f, io.LimitReader(resp.Body, 250*1024*1024+1))
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	h, e := hashFileContext(ctx, file)
	if errors.Is(e, context.DeadlineExceeded) {
		return errors.New("official archive download/hash exceeded its 3 minute deadline; existing runtime preserved")
	}
	if e != nil || h != expected {
		return errors.New("official archive SHA256 mismatch; existing runtime preserved")
	}
	return nil
}
func safeArchivePath(root, name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") || strings.Contains(name, ":") || strings.HasPrefix(name, "/") {
		return "", errors.New("unsafe archive path")
	}
	rel := filepath.FromSlash(name)
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", errors.New("archive path escapes destination")
	}
	return filepath.Join(root, clean), nil
}
func writeEntry(dest string, r io.Reader, mode os.FileMode) error {
	return writeEntryContext(context.Background(), dest, r, mode)
}
func writeEntryContext(ctx context.Context, dest string, r io.Reader, mode os.FileMode) error {
	if e := os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
	if e != nil {
		return e
	}
	_, e = io.Copy(f, contextReader{ctx, r})
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}
func extractZIP(archive, root string) error {
	ctx, cancel := context.WithTimeout(context.Background(), fileOperationTimeout)
	defer cancel()
	return extractZIPContext(ctx, archive, root)
}
func extractZIPContext(ctx context.Context, archive, root string) error {
	z, e := zip.OpenReader(archive)
	if e != nil {
		return e
	}
	defer z.Close()
	for _, entry := range z.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		dest, err := safeArchivePath(root, entry.Name)
		if err != nil {
			return err
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return errors.New("symlink ZIP entries not accepted")
		}
		if entry.FileInfo().IsDir() {
			if err = os.MkdirAll(dest, 0700); err != nil {
				return err
			}
			continue
		}
		r, err := entry.Open()
		if err != nil {
			return err
		}
		err = writeEntryContext(ctx, dest, r, entry.Mode())
		r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
func extractNode(archive, root string) error {
	ctx, cancel := context.WithTimeout(context.Background(), fileOperationTimeout)
	defer cancel()
	if runtime.GOOS == "windows" {
		if e := extractZIPContext(ctx, archive, root); e != nil {
			return e
		}
		return os.Rename(filepath.Join(root, "node-v"+nodeVersion+"-win-x64"), filepath.Join(root, "node"))
	}
	f, e := os.Open(archive)
	if e != nil {
		return e
	}
	defer f.Close()
	gz, e := gzip.NewReader(f)
	if e != nil {
		return e
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	prefix := "node-v" + nodeVersion + "-linux-x64/"
	for {
		if e := ctx.Err(); e != nil {
			return e
		}
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		if h.Name == prefix || h.Name == strings.TrimSuffix(prefix, "/") {
			continue
		}
		if !strings.HasPrefix(h.Name, prefix) {
			return errors.New("unexpected Node archive root")
		}
		dest, e := safeArchivePath(filepath.Join(root, "node"), strings.TrimPrefix(h.Name, prefix))
		if e != nil {
			return e
		}
		switch h.Typeflag {
		case tar.TypeDir:
			e = os.MkdirAll(dest, 0700)
		case tar.TypeReg:
			e = writeEntryContext(ctx, dest, tr, os.FileMode(h.Mode))
		case tar.TypeSymlink: // Node CLI aliases are unused; never extract links.
			continue
		default:
			return errors.New("unsupported Node archive entry")
		}
		if e != nil {
			return e
		}
	}
	return nil
}

const marker = "// xpotato: startup Chrome check/download/cache cleanup suppressed."

var chromeCall = regexp.MustCompile(`(?m)^([ \t]*)// Preemptively check/download Chrome for PDF generation \(runs in background\)\r?\n[ \t]*ensureChromeAvailable\(\);`)

func patchCommander(b []byte) ([]byte, error) {
	if len(chromeCall.FindAllIndex(b, -1)) != 1 {
		return nil, errors.New("pinned Commander startup changed; Chrome suppression not applied")
	}
	return chromeCall.ReplaceAll(b, []byte("$1"+marker)), nil
}
func filesReceipt(root string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fileOperationTimeout)
	defer cancel()
	return filesReceiptContext(ctx, root)
}
func filesReceiptContext(ctx context.Context, root string) (map[string]string, error) {
	files := map[string]string{}
	e := walkFileHashes(ctx, root, func(rel, hash string) error {
		files[rel] = hash
		return nil
	})
	return files, e
}
func walkFileHashes(ctx context.Context, root string, visit func(string, string) error) error {
	hash := sha256.New()
	buffer := make([]byte, 32*1024)
	return filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("runtime link not accepted")
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		if rel == "receipt.json" {
			return nil
		}
		f, e := os.Open(p)
		if e != nil {
			return e
		}
		hash.Reset()
		_, e = io.CopyBuffer(hash, contextReader{ctx, f}, buffer)
		closeErr := f.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		var sum [sha256.Size]byte
		var encoded [sha256.Size * 2]byte
		hex.Encode(encoded[:], hash.Sum(sum[:0]))
		return visit(rel, string(encoded[:]))
	})
}
func (manager Manager) Install(home string) (string, error) {
	m, e := manifest()
	if e != nil {
		return "", e
	}
	id := releaseID(m)
	dest := filepath.Join(home, "releases", id)
	if _, e = os.Stat(dest); e == nil {
		return id, manager.Verify(home, id)
	}
	releases := filepath.Dir(dest)
	if e = os.MkdirAll(releases, 0700); e != nil {
		return "", e
	}
	lock := filepath.Join(releases, "install.lock")
	i, e := platform.CurrentIdentity()
	if e != nil {
		return "", errors.New("install lock owner unavailable")
	}
	unlock, e := lockfile.Acquire(lock, i.Owner)
	if e != nil {
		return "", e
	}
	defer unlock()
	stage, e := os.MkdirTemp(releases, ".stage-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(stage) // only this owned temporary child
	archive := filepath.Join(stage, "tunnel.zip")
	if e = manager.stage("Tunnel runtime をダウンロード（上限3分）", func() error { return download(m.TunnelURL, archive, m.TunnelSHA) }); e != nil {
		return "", e
	}
	if e = manager.stage("Tunnel runtime を展開", func() error { return extractZIP(archive, stage) }); e != nil {
		return "", e
	}
	os.Remove(archive)
	h, e := hashFile(filepath.Join(stage, executable()))
	if e != nil || h != m.ExecutableSHA {
		return "", errors.New("tunnel executable SHA256 mismatch")
	}
	if runtime.GOOS == "linux" {
		if e = os.Chmod(filepath.Join(stage, executable()), 0700); e != nil {
			return "", e
		}
	}
	archive = filepath.Join(stage, "node.archive")
	if e = manager.stage("Node をダウンロード（上限3分）", func() error { return download(m.NodeURL, archive, m.NodeSHA) }); e != nil {
		return "", e
	}
	if e = manager.stage("Node を展開", func() error { return extractNode(archive, stage) }); e != nil {
		return "", e
	}
	os.Remove(archive)
	if e = os.WriteFile(filepath.Join(stage, "package.json"), []byte(`{"private":true,"name":"xpotato-managed-runtime","version":"1.0.0","dependencies":{"@wonderwhy-er/desktop-commander":"0.2.52"}}`), 0600); e != nil {
		return "", e
	}
	dependencyLock, _ := assets.ReadFile("assets/package-lock.json")
	if hashBytes(dependencyLock) != dependencyLockSHA {
		return "", errors.New("bundled dependency lock changed")
	}
	if e = os.WriteFile(filepath.Join(stage, "package-lock.json"), dependencyLock, 0600); e != nil {
		return "", e
	}
	node, npm := nodeFiles(stage)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, npm, "ci", "--ignore-scripts", "--no-audit", "--no-fund", "--bin-links=false")
	cmd.Dir = stage
	// Own cache; no inherited authentication configuration or npm config files.
	cache := filepath.Join(stage, ".npm-cache")
	userConfig := filepath.Join(stage, "empty-npmrc")
	globalConfig := filepath.Join(stage, "empty-global-npmrc")
	if e = os.WriteFile(userConfig, nil, 0600); e != nil {
		return "", e
	}
	if e = os.WriteFile(globalConfig, nil, 0600); e != nil {
		return "", e
	}
	cmd.Env = cleanNPMEnv(os.Environ())
	cmd.Env = append(cmd.Env, "NPM_CONFIG_USERCONFIG="+userConfig, "NPM_CONFIG_GLOBALCONFIG="+globalConfig, "NPM_CONFIG_CACHE="+cache, "NPM_CONFIG_REGISTRY=https://registry.npmjs.org/")
	// npm output may contain inherited proxy/private metadata; do not forward it.
	if e = manager.stage("Commander の固定依存をインストール（上限5分）", func() error {
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return errors.New("5分の期限を超えました。既存 runtime は保持しています")
			}
			return errors.New("pinned Commander install failed; existing runtime preserved")
		}
		return nil
	}); e != nil {
		return "", e
	}
	os.RemoveAll(cache)
	os.Remove(userConfig)
	os.Remove(globalConfig)
	entry := filepath.Join(stage, "node_modules", "@wonderwhy-er", "desktop-commander", "dist", "index.js")
	b, e := os.ReadFile(entry)
	if e != nil {
		return "", e
	}
	b, e = patchCommander(b)
	if e != nil {
		return "", e
	}
	if e = os.WriteFile(entry, b, 0600); e != nil {
		return "", e
	}
	stdio, _ := assets.ReadFile("assets/stdio.mjs")
	if e = os.WriteFile(filepath.Join(stage, "stdio.mjs"), stdio, 0600); e != nil {
		return "", e
	}
	activity, _ := assets.ReadFile("assets/activity.mjs")
	if e = os.WriteFile(filepath.Join(stage, "activity.mjs"), activity, 0600); e != nil {
		return "", e
	}
	var files map[string]string
	if e = manager.stage("全 runtime ファイルの SHA-256 を記録", func() error {
		var err error
		files, err = filesReceipt(stage)
		return err
	}); e != nil {
		return "", e
	}
	receipt, _ := json.Marshal(Receipt{m, files})
	if e = os.WriteFile(filepath.Join(stage, "receipt.json"), receipt, 0600); e != nil {
		return "", e
	}
	if e = manager.stage("展開した全 runtime ファイルを検証", func() error { return verifyDirectory(stage, m) }); e != nil {
		return "", e
	}
	if e = os.Rename(stage, dest); e != nil {
		return "", e
	}
	return id, nil
}
func (manager Manager) Verify(home, id string) error {
	m, e := manifest()
	if e != nil {
		return e
	}
	if id != releaseID(m) {
		return core.ErrRuntimePinUnsupported
	}
	return manager.stage("保存済み全 runtime ファイルを検証", func() error { return verifyDirectory(filepath.Join(home, "releases", id), m) })
}
func verifyDirectory(dir string, m Manifest) error {
	b, e := os.ReadFile(filepath.Join(dir, "receipt.json"))
	if e != nil {
		return errors.New("runtime receipt missing")
	}
	var r Receipt
	if json.Unmarshal(b, &r) != nil || r.Manifest != m {
		return errors.New("runtime receipt manifest mismatch")
	}
	ctx, cancel := context.WithTimeout(context.Background(), fileOperationTimeout)
	defer cancel()
	count := 0
	e = walkFileHashes(ctx, dir, func(p, h string) error {
		count++
		if r.Files[p] != h {
			return errors.New("runtime integrity check failed")
		}
		return nil
	})
	if errors.Is(e, context.DeadlineExceeded) {
		return errors.New("runtime verification exceeded its 10 minute deadline; existing runtime preserved")
	}
	if e != nil {
		return errors.New("runtime integrity or file inventory check failed")
	}
	if count != len(r.Files) {
		return errors.New("runtime file inventory changed")
	}
	if r.Files[executable()] != m.ExecutableSHA {
		return errors.New("official tunnel executable changed")
	}
	if r.Files["package-lock.json"] != dependencyLockSHA {
		return errors.New("reviewed dependency lock changed")
	}
	stdio, _ := assets.ReadFile("assets/stdio.mjs")
	if r.Files["stdio.mjs"] != hashBytes(stdio) {
		return errors.New("stdio adapter changed")
	}
	activity, _ := assets.ReadFile("assets/activity.mjs")
	if r.Files["activity.mjs"] != hashBytes(activity) {
		return errors.New("tool activity observer changed")
	}
	b, e = os.ReadFile(filepath.Join(dir, "node_modules", "@wonderwhy-er", "desktop-commander", "dist", "index.js"))
	if e != nil || strings.Count(string(b), marker) != 1 || chromeCall.Match(b) {
		return errors.New("Commander Chrome startup suppression missing")
	}
	return nil
}
func cleanEnv(env []string, names ...string) []string {
	result := make([]string, 0, len(env))
	for _, line := range env {
		key, _, _ := strings.Cut(line, "=")
		keep := true
		for _, n := range names {
			if strings.EqualFold(key, n) {
				keep = false
				break
			}
		}
		if keep {
			result = append(result, line)
		}
	}
	return result
}
func cleanNPMEnv(env []string) []string {
	filtered := cleanEnv(env, "NODE_OPTIONS", "NODE_PATH", "NODE_COMPILE_CACHE", "NPM_TOKEN", "NODE_AUTH_TOKEN", "CONTROL_PLANE_API_KEY", "OPENAI_API_KEY")
	result := []string{}
	for _, line := range filtered {
		key, _, _ := strings.Cut(line, "=")
		if !strings.HasPrefix(strings.ToLower(key), "npm_config_") {
			result = append(result, line)
		}
	}
	return result
}
func quote(p string) (string, error) {
	if strings.ContainsAny(p, "'\x00\r\n") {
		return "", errors.New("runtime paths containing quotes/newlines unsupported")
	}
	return "'" + filepath.ToSlash(p) + "'", nil
}
func Command(node, stdio string) (string, error) {
	n, e := quote(node)
	if e != nil {
		return "", e
	}
	s, e := quote(stdio)
	if e != nil {
		return "", e
	}
	return n + " " + s + " --no-onboarding", nil
}

const controlPlaneURL = "https://api.openai.com"

func runtimeEnvironment(base []string, run, profile, entry string, key []byte) []string {
	allowed := map[string]bool{"PATH": true, "SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true, "PATHEXT": true, "OS": true, "PROCESSOR_ARCHITECTURE": true, "LANG": true, "LC_ALL": true, "LC_CTYPE": true, "TZ": true}
	env := []string{}
	for _, line := range base {
		name, _, _ := strings.Cut(line, "=")
		if allowed[strings.ToUpper(name)] {
			env = append(env, line)
		}
	}
	for name, value := range map[string]string{"HOME": run, "USERPROFILE": run, "APPDATA": filepath.Join(run, "Roaming"), "LOCALAPPDATA": filepath.Join(run, "Local"), "XDG_CONFIG_HOME": filepath.Join(run, "config"), "XDG_CACHE_HOME": filepath.Join(run, "cache"), "XDG_DATA_HOME": filepath.Join(run, "data"), "TEMP": filepath.Join(run, "tmp"), "TMP": filepath.Join(run, "tmp"), "TMPDIR": filepath.Join(run, "tmp"), "XPOTATO_MCP_PROFILE": profile, "XPOTATO_MCP_COMMANDER_ENTRY": entry, "CONTROL_PLANE_API_KEY": string(key)} {
		env = append(env, name+"="+value)
	}
	return env
}
func runtimeArguments(d core.Device, run, command string) []string {
	return []string{"run", "--profile-dir", filepath.Join(run, "profiles"), "--control-plane.base-url", controlPlaneURL, "--control-plane.url-path", "", "--control-plane.tunnel-id", d.Tunnel, "--control-plane.api-key", "env:CONTROL_PLANE_API_KEY", "--control-plane.max-inflight", "1", "--control-plane.poll-channel", "main", "--mcp.command", command, "--mcp.max-concurrent-requests", "1", "--mcp.stdio-send-initialized-notification", "--health.listen-addr", "127.0.0.1:0", "--health.url-file", filepath.Join(run, "health-url.txt"), "--log.level", "info", "--log.format", "", "--log.file", "", "--log.http-raw-unsafe=false", "--pid.file", filepath.Join(run, "tunnel.pid")}
}
func (manager Manager) Run(home string, d core.Device, key []byte) (int, error) {
	dir := filepath.Join(home, "releases", d.Release)
	node, _ := nodeFiles(dir)
	command, e := Command(node, filepath.Join(dir, "stdio.mjs"))
	if e != nil {
		return 1, e
	}
	if manager.ActivityDetails {
		command += " --activity-details"
	}
	// Presentation follows the launcher's stderr capability/NO_COLOR decision;
	// the Node child's stderr is a protocol-managed pipe, not the user's TTY.
	if manager.ActivityColor {
		command += " --activity-color"
	}
	deviceDir := filepath.Join(home, "devices", d.Name)
	profile := core.ProfilePath(home, d)
	for _, sub := range []string{"Temp", "AppData/Roaming", "AppData/Local", ".config", ".cache", ".local/share"} {
		if e = os.MkdirAll(filepath.Join(profile, filepath.FromSlash(sub)), 0700); e != nil {
			return 1, e
		}
	}
	run, e := os.MkdirTemp(deviceDir, "run-")
	if e != nil {
		return 1, e
	}
	for _, sub := range []string{"profiles", "config", "cache", "data", "tmp", "Roaming", "Local"} {
		if e = os.MkdirAll(filepath.Join(run, sub), 0700); e != nil {
			return 1, e
		}
	}
	args := runtimeArguments(d, run, command)
	cmd := exec.Command(filepath.Join(dir, executable()), args...)
	cmd.Dir = run
	cmd.Env = runtimeEnvironment(os.Environ(), run, profile, filepath.Join(dir, "node_modules", "@wonderwhy-er", "desktop-commander", "dist", "index.js"), key)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupt)
	clear := func() {
		cmd.Env = nil
		for i := range key {
			key[i] = 0
		}
	}
	defer clear()
	stopStatus := func() {}
	defer func() { stopStatus() }()
	code, e := processctrl.RunWithStatus(cmd, interrupt, clear, 5*time.Second, func(state string) {
		if manager.Progress == nil {
			return
		}
		switch state {
		case "running":
			manager.Progress("OpenAI トンネルの子プロセスを起動（接続はまだ未確認）")
			stopStatus = monitorConnection(filepath.Join(run, "health-url.txt"), manager.Progress, progressInterval)
		case "stopping":
			stopStatus()
			manager.Progress("停止要求を受けました。この起動の子プロセスを停止中（猶予5秒）")
		case "cleanup":
			stopStatus()
			manager.Progress("この起動の子プロセスを終了・回収中")
		case "stopped":
			manager.Progress("OpenAI トンネル 切断（この起動の子プロセスが終了）")
		}
	})
	if e != nil {
		if manager.Progress != nil {
			manager.Progress("Tunnel 子プロセスの起動または回収に失敗しました")
		}
		return 1, errors.New("owned tunnel launch/cleanup failed")
	}
	if code != 0 && manager.Progress != nil {
		manager.Progress(fmt.Sprintf("Tunnel 子プロセスが終了コード %d で終了しました", code))
	}
	return code, nil
}
