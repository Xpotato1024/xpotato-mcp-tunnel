package managed

import (
	"fmt"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// Only this synthetic child marker enables the fake Tunnel. No actual service.
func init() {
	if len(os.Args) < 2 || os.Args[1] != "run" {
		return
	}
	profile := os.Getenv("XPOTATO_MCP_PROFILE")
	if _, err := os.Stat(filepath.Join(profile, ".core-perf-fixture")); err != nil {
		return
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		os.Exit(2)
	}
	for i, arg := range os.Args {
		if arg == "--health.url-file" && i+1 < len(os.Args) {
			os.WriteFile(os.Args[i+1], []byte("http://"+ln.Addr().String()), 0600)
		}
	}
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"schema_version":1,"component":"control-plane","status":"ok","state":"polling"}`)
	})}
	go server.Serve(ln)
	fmt.Println("PERF_READY")
	time.Sleep(25 * time.Second)
	server.Close()
	os.Exit(0)
}

type perfVault struct{}

func (perfVault) Ready() error                { return nil }
func (perfVault) Save(string, []byte) error   { return nil }
func (perfVault) Load(string) ([]byte, error) { return []byte("synthetic-perf-key"), nil }

type perfPrompt struct{}

func (perfPrompt) Text(string) (string, error)  { return "", fmt.Errorf("unexpected fixture prompt") }
func (perfPrompt) Key() ([]byte, error)         { return []byte("synthetic-perf-key"), nil }
func (perfPrompt) Consent(string) (bool, error) { return true, nil }

type perfRuntime struct{ manager Manager }

func (p perfRuntime) Install(home string) (string, error) {
	dir := filepath.Join(home, "releases", "fixture")
	os.MkdirAll(filepath.Join(dir, "node"), 0700)
	exe, _ := os.Executable()
	return "fixture", stageCopy(exe, filepath.Join(dir, executable()), nil)
}
func (p perfRuntime) Verify(string, string) error { return nil }
func (p perfRuntime) Run(home string, d core.Device, key []byte) (int, error) {
	return p.manager.Run(home, d, key)
}

func TestCoreIdlePerformanceFixture(t *testing.T) {
	if os.Getenv("XPOTATO_PERF_CORE") != "1" {
		t.Skip("standalone synthetic CPU/RSS fixture")
	}
	home := t.TempDir()
	root := t.TempDir()
	manager := Manager{}
	report := func(s string) { fmt.Fprintln(os.Stderr, s) }
	field := reflect.ValueOf(&manager).Elem().FieldByName("Progress")
	if field.IsValid() {
		field.Set(reflect.ValueOf(report))
	}
	app := core.App{Identity: func() (platform.Identity, error) { return platform.Identity{Owner: "synthetic-perf-owner"}, nil }, Vault: perfVault{}, Prompt: perfPrompt{}, Runtime: perfRuntime{manager}}
	field = reflect.ValueOf(&app).Elem().FieldByName("Progress")
	if field.IsValid() {
		field.Set(reflect.ValueOf(report))
	}
	options := core.Options{Home: home, Device: "fixture", Tunnel: "tunnel_fixture", Allow: []string{root}}
	if _, err := app.Setup(options); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(home, "devices", "fixture", "profile")
	os.WriteFile(filepath.Join(profile, ".core-perf-fixture"), nil, 0600)
	if code, err := app.Start(options); err != nil || code != 0 {
		t.Fatal(code, err)
	}
}
