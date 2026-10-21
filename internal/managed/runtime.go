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
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/display"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/licensing"
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

//go:embed assets/stdio.mjs assets/activity.mjs assets/licensing.mjs assets/package-lock.json
var assets embed.FS

const nodeVersion = "24.22.1"
const commanderVersion = "0.3.0"
const tunnelVersion = "0.0.16"
const dependencyLockSHA = "ab831afcf75cbbc4193075599ad69429ae4fe42c12db28c4d3e424f36e77e825"

type Manifest struct{ Platform, NodeURL, NodeSHA, TunnelURL, TunnelSHA, ExecutableSHA string }

func manifest() (Manifest, error) {
	return manifestFor(runtime.GOOS, runtime.GOARCH)
}
func SupportedPlatform() bool { _, err := manifest(); return err == nil }
func manifestFor(goos, arch string) (Manifest, error) {
	if arch != "amd64" && !(goos == "darwin" && arch == "arm64") {
		return Manifest{}, errors.New("unsupported runtime architecture")
	}
	base := "https://github.com/openai/tunnel-client/releases/download/v0.0.16/tunnel-client-runtime-v0.0.16-"
	switch goos {
	case "windows":
		return Manifest{"windows-amd64", "https://nodejs.org/dist/v24.22.1/node-v24.22.1-win-x64.zip", "d2046d28c2c3dcd211ac5145c0bf30e2047ef9dfb306693c8c6e2eccd8b95fa5", base + "windows-amd64.zip", "639fdfe10d91c672d8f8a49386ebf39acaac4e560a88134735b1739a02ddb82d", "19e7970f825e0180546acba4c73ed1163a85f8d6fdd4393ff0854458a642d1b8"}, nil
	case "linux":
		return Manifest{"linux-amd64", "https://nodejs.org/dist/v24.22.1/node-v24.22.1-linux-x64.tar.gz", "eb6728e77c576a35162c122c45ccadf224d89ff00abc8dae46cc2897c6002d7c", base + "linux-amd64.zip", "f8f48017e752810c2c6f02065cdc4ca1ad045b91a6d045cbab144b957fdcbdb1", "35183d94445c4c11a9d2f2aba52d23b3581c3580bf3998226a33a904c85f37d0"}, nil
	case "darwin":
		if arch == "arm64" {
			return Manifest{"darwin-arm64", "https://nodejs.org/dist/v24.22.1/node-v24.22.1-darwin-arm64.tar.gz", "89c682593d76cd44233fa53b1563264e7de9cbb189cad169135c5b4fadfbc0a7", base + "darwin-arm64.zip", "81d230a289ae1b1416738d8150bc879d88a29e8749bb659047a7ef90dfc4acbe", "5864074e05609e7ea5758f4fe86c0be63ac9b8aa4b49b4b9303241484102cb2f"}, nil
		}
		return Manifest{"darwin-amd64", "https://nodejs.org/dist/v24.22.1/node-v24.22.1-darwin-x64.tar.gz", "d132557e7ee2b85af08b0c8c34338e65c6430f2238d01f727728ef42dc83b035", base + "darwin-amd64.zip", "bfd4a63a523ef9e585bbc71d1c008d099fb7560fc419d3dbb24db6f88d3c94ee", "c907b358f6b8fca5b253cf91a0df59823c704ec760d1928fe56bb2b3e5feeb4c"}, nil
	}
	return Manifest{}, errors.New("unsupported OS")
}
func releaseID(m Manifest) string {
	stdio, _ := assets.ReadFile("assets/stdio.mjs")
	activity, _ := assets.ReadFile("assets/activity.mjs")
	license, _ := assets.ReadFile("assets/licensing.mjs")
	// Bump Recipe when extraction, npm flags, or patch semantics change.
	b, _ := json.Marshal(struct {
		Recipe                                int
		Manifest                              Manifest
		Lock, Stdio, Activity, License, Patch string
	}{1, m, dependencyLockSHA, hashBytes(stdio), hashBytes(activity), hashBytes(license), marker + chromeCall.String()})
	return "runtime-v1-" + hashBytes(b)
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
	ctx, cancel := context.WithTimeout(ctx, fileOperationTimeout)
	defer cancel()
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
	Manifest      Manifest
	Files         map[string]string
	SchemaVersion int    `json:"schemaVersion,omitempty"`
	Scope         string `json:"scope,omitempty"`
}
type Manager struct {
	Context          context.Context
	runtimeManifest  func() (Manifest, error)
	populate         func(context.Context, string, Manifest) error
	inventoryStarted func()
	Progress         func(string)
	ActivityDetails  bool
	ActivityColor    bool
	Diagnostics      bool
	ForceRepair      bool
	// GUI bridge pipes must never become runtime stdin/log channels.
	Input       io.Reader
	Log         io.Writer
	HideWindows bool
}

func (m Manager) stage(label string, work func() error) error {
	return runStage(m.Progress, label, progressInterval, func() error {
		if e := m.context().Err(); e != nil {
			return e
		}
		return work()
	})
}

func download(url, file, expected string) error {
	return downloadContext(context.Background(), url, file, expected)
}
func downloadContext(parent context.Context, url, file, expected string) error {
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "GET", url, nil)
	if e != nil {
		return errors.New("runtime download request invalid")
	}
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
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
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, hash), io.LimitReader(resp.Body, 250*1024*1024+1))
	e = copyErr
	closeErr := f.Close()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if n > 250*1024*1024 || hex.EncodeToString(hash.Sum(nil)) != expected {
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
	ctx, cancel := context.WithTimeout(ctx, fileOperationTimeout)
	defer cancel()
	if e := ctx.Err(); e != nil {
		return e
	}
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
	return extractNodeContext(context.Background(), archive, root)
}
func extractNodeContext(parent context.Context, archive, root string) error {
	return extractNodePlatform(parent, archive, root, runtime.GOOS, runtime.GOARCH)
}
func extractNodePlatform(parent context.Context, archive, root, goos, arch string) error {
	ctx, cancel := context.WithTimeout(parent, fileOperationTimeout)
	defer cancel()
	m, err := manifestFor(goos, arch)
	if err != nil {
		return err
	}
	if goos == "windows" {
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
	prefix := strings.TrimSuffix(m.NodeURL[strings.LastIndex(m.NodeURL, "/")+1:], ".tar.gz") + "/"
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
	return walkFileHashesObserved(ctx, root, visit, nil)
}

type InventoryProgress struct{ Files, Bytes int64 }

func walkFileHashesObserved(ctx context.Context, root string, visit func(string, string) error, observe func(InventoryProgress)) error {
	var progress InventoryProgress
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
		n, e := io.CopyBuffer(hash, contextReader{ctx, f}, buffer)
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
		if e = visit(rel, string(encoded[:])); e != nil {
			return e
		}
		progress.Files++
		progress.Bytes += n
		if observe != nil {
			observe(progress)
		}
		return nil
	})
}
func (manager Manager) Install(home string) (string, error) {
	p, e := manager.PrepareRuntime(home, "")
	if e != nil {
		return "", e
	}
	defer p.Close()
	return p.Verify()
}

func (manager Manager) populateRuntime(ctx context.Context, stage string, m Manifest) error {
	var e error
	archive := filepath.Join(stage, "tunnel.zip")
	if e = manager.stage("Tunnel runtime をダウンロード（上限3分）", func() error { return downloadContext(ctx, m.TunnelURL, archive, m.TunnelSHA) }); e != nil {
		return e
	}
	if e = manager.stage("Tunnel runtime を展開", func() error { return extractZIPContext(ctx, archive, stage) }); e != nil {
		return e
	}
	os.Remove(archive)
	h, e := hashFileContext(ctx, filepath.Join(stage, executable()))
	if e != nil || h != m.ExecutableSHA {
		return errors.New("tunnel executable SHA256 mismatch")
	}
	if runtime.GOOS != "windows" {
		if e = os.Chmod(filepath.Join(stage, executable()), 0700); e != nil {
			return e
		}
	}
	archive = filepath.Join(stage, "node.archive")
	if e = manager.stage("Node をダウンロード（上限3分）", func() error { return downloadContext(ctx, m.NodeURL, archive, m.NodeSHA) }); e != nil {
		return e
	}
	if e = manager.stage("Node を展開", func() error { return extractNodeContext(ctx, archive, stage) }); e != nil {
		return e
	}
	os.Remove(archive)
	if e = os.WriteFile(filepath.Join(stage, "package.json"), []byte(`{"private":true,"name":"xpotato-managed-runtime","version":"1.0.0","dependencies":{"@wonderwhy-er/desktop-commander":"0.3.0"},"overrides":{"exceljs":{"uuid":"^11.1.1"},"argparse":"^2.0.1","md-to-pdf":{"chokidar":"^4.0.3"},"proxy-addr":"^2.0.8"}}`), 0600); e != nil {
		return e
	}
	dependencyLock, _ := assets.ReadFile("assets/package-lock.json")
	if hashBytes(dependencyLock) != dependencyLockSHA {
		return errors.New("bundled dependency lock changed")
	}
	if e = os.WriteFile(filepath.Join(stage, "package-lock.json"), dependencyLock, 0600); e != nil {
		return e
	}
	node, npm := nodeFiles(stage)
	npmCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.Command(node, npm, "ci", "--ignore-scripts", "--no-audit", "--no-fund", "--bin-links=false")
	if manager.HideWindows {
		processctrl.HideWindow(cmd)
	}
	cmd.Dir = stage
	// Own cache; no inherited authentication configuration or npm config files.
	cache := filepath.Join(stage, ".npm-cache")
	userConfig := filepath.Join(stage, "empty-npmrc")
	globalConfig := filepath.Join(stage, "empty-global-npmrc")
	if e = os.WriteFile(userConfig, nil, 0600); e != nil {
		return e
	}
	if e = os.WriteFile(globalConfig, nil, 0600); e != nil {
		return e
	}
	cmd.Env = cleanNPMEnv(os.Environ())
	cmd.Env = append(cmd.Env, "NPM_CONFIG_USERCONFIG="+userConfig, "NPM_CONFIG_GLOBALCONFIG="+globalConfig, "NPM_CONFIG_CACHE="+cache, "NPM_CONFIG_REGISTRY=https://registry.npmjs.org/")
	// npm output may contain inherited proxy/private metadata; do not forward it.
	if e = manager.stage("Commander の固定依存をインストール（上限5分）", func() error {
		if err := runOwnedContext(npmCtx, cmd); err != nil {
			if npmCtx.Err() != nil {
				return npmCtx.Err()
			}
			return errors.New("pinned Commander install failed; existing runtime preserved")
		}
		return nil
	}); e != nil {
		return e
	}
	os.RemoveAll(cache)
	os.Remove(userConfig)
	os.Remove(globalConfig)
	entry := filepath.Join(stage, "node_modules", "@wonderwhy-er", "desktop-commander", "dist", "index.js")
	b, e := os.ReadFile(entry)
	if e != nil {
		return e
	}
	b, e = patchCommander(b)
	if e != nil {
		return e
	}
	if e = os.WriteFile(entry, b, 0600); e != nil {
		return e
	}
	stdio, _ := assets.ReadFile("assets/stdio.mjs")
	if e = os.WriteFile(filepath.Join(stage, "stdio.mjs"), stdio, 0600); e != nil {
		return e
	}
	activity, _ := assets.ReadFile("assets/activity.mjs")
	if e = os.WriteFile(filepath.Join(stage, "activity.mjs"), activity, 0600); e != nil {
		return e
	}
	license, _ := assets.ReadFile("assets/licensing.mjs")
	if e = os.WriteFile(filepath.Join(stage, "licensing.mjs"), license, 0600); e != nil {
		return e
	}
	// npm ci already checks the fixed lock's package integrity. Do not reopen
	// every expanded dependency to produce an exhaustive receipt during setup.
	r, e := captureLaunchReceipt(ctx, stage, m)
	if e != nil {
		return e
	}
	receipt, _ := json.Marshal(r)
	if e = os.WriteFile(filepath.Join(stage, "receipt.json"), receipt, 0600); e != nil {
		return e
	}
	return ctx.Err()
}
func (manager Manager) Verify(home, id string) error {
	m, e := manager.manifest()
	if e != nil {
		return e
	}
	if !compatibleRuntimeID(m, id) {
		return core.ErrRuntimePinUnsupported
	}
	return manager.verifyReadiness(manager.context(), filepath.Join(home, "releases", id), m)
}
func verifyDirectory(dir string, m Manifest) error {
	return (Manager{}).verifyDirectory(context.Background(), dir, m)
}
func (manager Manager) verifyDirectory(parent context.Context, dir string, m Manifest) error {
	ctx, cancel := context.WithTimeout(parent, fileOperationTimeout)
	defer cancel()
	if e := ctx.Err(); e != nil {
		return e
	}
	b, e := os.ReadFile(filepath.Join(dir, "receipt.json"))
	if e != nil {
		return errors.New("runtime receipt missing")
	}
	var r Receipt
	if json.Unmarshal(b, &r) != nil || r.Manifest != m {
		return errors.New("runtime receipt manifest mismatch")
	}
	count := 0
	e = manager.inventoryWithTotal(ctx, "保存済み全 runtime ファイルを検証", dir, int64(len(r.Files)), func(p, h string) error {
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
		if ctx.Err() != nil {
			return ctx.Err()
		}
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
	license, _ := assets.ReadFile("assets/licensing.mjs")
	if r.Files["licensing.mjs"] != hashBytes(license) {
		return errors.New("source offer adapter changed")
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
	// Nonsecret build provenance comes from this core, never inherited user input
	// or a reusable runtime generation built by an older core.
	env = append(env, "XPOTATO_MCP_SOURCE_INFO="+licensing.JSON(core.Version))
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
func runtimeArguments(d core.Device, run, command string, diagnostics bool) []string {
	level := "warn"
	if diagnostics {
		level = "info"
	}
	// Tunnel 0.0.16 rejects a non-default level such as warn with unset format.
	return []string{"run", "--profile-dir", filepath.Join(run, "profiles"), "--control-plane.base-url", controlPlaneURL, "--control-plane.url-path", "", "--control-plane.tunnel-id", d.Tunnel, "--control-plane.api-key", "env:CONTROL_PLANE_API_KEY", "--control-plane.max-inflight", "1", "--control-plane.poll-channel", "main", "--mcp.command", command, "--mcp.max-concurrent-requests", "1", "--mcp.stdio-send-initialized-notification", "--health.listen-addr", "127.0.0.1:0", "--health.url-file", filepath.Join(run, "health-url.txt"), "--log.level", level, "--log.format", "struct-text", "--log.file", "", "--log.http-raw-unsafe=false", "--pid.file", filepath.Join(run, "tunnel.pid")}
}
func (manager Manager) Run(home string, d core.Device, key []byte) (int, error) {
	if e := manager.context().Err(); e != nil {
		return 1, e
	}
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
	args := runtimeArguments(d, run, command, manager.Diagnostics)
	cmd := exec.Command(filepath.Join(dir, executable()), args...)
	if manager.HideWindows {
		processctrl.HideWindow(cmd)
	}
	// After the leader exits, a descendant must not hold the log drain open
	// forever and prevent the existing owned-tree cleanup path from running.
	cmd.WaitDelay = time.Second
	cmd.Dir = run
	cmd.Env = runtimeEnvironment(os.Environ(), run, profile, filepath.Join(dir, "node_modules", "@wonderwhy-er", "desktop-commander", "dist", "index.js"), key)
	logOutput := manager.Log
	if logOutput == nil {
		logOutput = os.Stderr
	}
	childLog := display.NewChild(logOutput, manager.ActivityColor, key)
	defer childLog.Close()
	// Structured tunnel-runtime logs use stdout; MCP stdio is a separate pipe
	// owned by that runtime. Sanitize both runtime log streams on parent stderr.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, childLog, childLog
	if manager.Input != nil {
		cmd.Stdin = manager.Input
	}
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupt)
	stopContext := context.AfterFunc(manager.context(), func() {
		select {
		case interrupt <- os.Interrupt:
		default:
		}
	})
	defer stopContext()
	if e = manager.context().Err(); e != nil {
		return 1, e
	}
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
