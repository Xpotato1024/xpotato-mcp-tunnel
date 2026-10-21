package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/display"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/licensing"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/managed"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
	"golang.org/x/term"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

const help = `xpotato-tunnel ` + core.Version + ` (Windows/Linux amd64; macOS amd64/arm64)
  setup [--env-file PATH] [--device NAME] [--tunnel-id ID] [--allow ABSOLUTE_DIR ...]
  maintenance | start | doctor [--deep] | update | repair | rollback | credentials | plugin
  package-check | uninstall-entrypoints (no credentials or runtime scan)
  version | about | license | source (no setup or credentials required)
  --home ABSOLUTE_DIR  (all commands)
  --device NAME       (omit to use setup's default device)
  --activity-details  (start only; conservative argument display on stderr)
  --diagnostics      (start only; internal INFO logs on stderr, raw HTTP off)
  --mcp-url HTTPS_URL (setup only; owner-verified direct MCP endpoint, optional)
  --command-dir DIR  (setup only; optional short command location, local consent)
setup は日本語ウィザードで設定・複数の許可フォルダー・恒久保存先を確認します。
--env-file は非秘密の DEVICE / TUNNEL_ID / ALLOW_DIR_1..N を読み、保存前に確認します。
API キーを .env に保存・読み込みする機能はありません。
Tunnel ID と API キーは別物です。API キーは保存同意の後、非表示で入力します。
start は保存済みキーを使用します。引数なしは start です。
Tunnel のプラグイン登録は本人が ChatGPT Plugins UI で行います。
`

type terminal struct{}

// Avoid read-ahead across the boundary to masked key input. Keeping a reader
// also avoids losing later pasted nonsecret lines between Text calls.
type singleByteReader struct{ io.Reader }

func (r singleByteReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return r.Reader.Read(p)
}

var textInput = bufio.NewReader(singleByteReader{os.Stdin})

func (terminal) Text(label string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("Setup は本人の対話端末で実行してください")
	}
	writeInputPrompt(os.Stdout, label)
	line, e := textInput.ReadString('\n')
	if e != nil {
		return "", core.ErrSetupCancelled
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "q", "cancel", "中断":
		return "", core.ErrSetupCancelled
	}
	return strings.TrimSpace(line), e
}
func (t terminal) Consent(label string) (bool, error) {
	for {
		line, e := t.Text(label)
		if e != nil {
			return false, e
		}
		switch strings.ToLower(line) {
		case "yes", "はい":
			return true, nil
		case "no", "いいえ":
			return false, nil
		default:
			fmt.Println("同意する場合は yes、同意しない場合は no、中断する場合は q を入力してください。")
		}
	}
}
func (terminal) Key() ([]byte, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil, errors.New("interactive terminal required")
	}
	writeInputPrompt(os.Stdout, "Runtime API キー（非表示入力、Ctrl+C で中断。Tunnel ID は入力しないでください）")
	key, e := readKey(int(os.Stdin.Fd()))
	fmt.Println()
	return key, e
}

type repeated []string

func (r *repeated) String() string     { return strings.Join(*r, ",") }
func (r *repeated) Set(s string) error { *r = append(*r, s); return nil }
func parse(args []string) (string, core.Options, error) {
	command := "start"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command, args = args[0], args[1:]
	}
	if command == "help" || (len(args) > 0 && (args[0] == "--help" || args[0] == "-h")) {
		return "help", core.Options{}, nil
	}
	if command == "version" || command == "about" || command == "license" || command == "source" || command == "package-check" {
		return command, core.Options{}, nil
	}
	if !strings.Contains("|maintenance|uninstall-entrypoints|setup|start|doctor|update|repair|rollback|credentials|plugin|", "|"+command+"|") {
		return "", core.Options{}, errors.New("unknown command; use help")
	}
	o := core.Options{}
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	f.SetOutput(new(strings.Builder))
	f.StringVar(&o.Home, "home", "", "home")
	f.StringVar(&o.EnvFile, "env-file", "", "nonsecret setup file")
	f.StringVar(&o.Device, "device", "", "device")
	f.StringVar(&o.Tunnel, "tunnel-id", "", "tunnel")
	f.StringVar(&o.MCPURL, "mcp-url", "", "endpoint")
	f.StringVar(&o.CommandDir, "command-dir", "", "short command directory")
	f.BoolVar(&o.ActivityDetails, "activity-details", false, "explicit tool argument display")
	f.BoolVar(&o.Diagnostics, "diagnostics", false, "explicit internal diagnostic logs")
	f.BoolVar(&o.Deep, "deep", false, "explicit full runtime diagnosis; may download a reference")
	var allow repeated
	f.Var(&allow, "allow", "root")
	if e := f.Parse(args); e != nil || f.NArg() != 0 {
		return "", o, errors.New("invalid options; use help (keys are accepted only by hidden terminal input)")
	}
	o.Allow = allow
	if o.Deep && command != "doctor" {
		return "", o, errors.New("--deep is doctor-only")
	}
	if o.ActivityDetails && command != "start" {
		return "", o, errors.New("--activity-details is start-only")
	}
	if o.Diagnostics && command != "start" {
		return "", o, errors.New("--diagnostics is start-only")
	}
	envSpecified, conflict := false, false
	f.Visit(func(option *flag.Flag) {
		if option.Name == "env-file" {
			envSpecified = true
		}
		if option.Name == "device" || option.Name == "tunnel-id" || option.Name == "allow" {
			conflict = true
		}
	})
	if envSpecified {
		if command != "setup" || conflict || o.EnvFile == "" {
			return "", o, errors.New("--env-file は setup 専用で、--device / --tunnel-id / --allow と併用できません")
		}
		env, e := core.ReadSetupEnv(o.EnvFile)
		if e != nil {
			return "", o, e
		}
		o.Device, o.Tunnel, o.Allow = env.Device, env.Tunnel, env.Allow
	}
	if command != "setup" && (o.Tunnel != "" || o.MCPURL != "" || len(o.Allow) > 0 || o.CommandDir != "") {
		return "", o, errors.New("--tunnel-id/--allow/--mcp-url are setup-only")
	}
	if o.Home == "" {
		var e error
		o.Home, e = core.DefaultHome()
		if e != nil {
			return "", o, e
		}
	}
	if command != "setup" && command != "maintenance" && command != "uninstall-entrypoints" && !(command == "doctor" && o.Device == "" && !o.Deep) {
		var e error
		o, e = core.Resolve(o)
		if e != nil {
			return "", o, e
		}
	}
	return command, o, nil
}
func run(args []string) (int, error) {
	if len(args) == 1 && args[0] == "_gui-bridge" {
		return guiBridge(os.Stdin, os.Stdout)
	}
	if len(args) > 0 && args[0] == "_dispatch" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return dispatch(ctx, args[1:])
	}
	command, o, e := parse(args)
	if e != nil {
		return 1, e
	}
	if command == "help" {
		fmt.Print(help)
		return 0, nil
	}
	if command == "version" {
		fmt.Println(core.Version)
		return 0, nil
	}
	if command == "about" || command == "license" {
		fmt.Print(licensing.Attribution)
		fmt.Println("\nCorresponding source: xpotato-tunnel source")
		if command == "license" {
			fmt.Print("\n" + licensing.Text)
		}
		return 0, nil
	}
	if command == "source" {
		fmt.Println(licensing.JSON(core.Version))
		return 0, nil
	}
	if command == "package-check" {
		return packageCheck()
	}
	if command == "maintenance" {
		return maintenance(o)
	}
	if command == "uninstall-entrypoints" {
		if err := managed.UninstallEntrypoints(o.Home); err != nil {
			return 1, err
		}
		fmt.Println("Owned command entrypoints removed. Profiles, runtime generations and saved credentials retained. Existing processes were not stopped.")
		return 0, nil
	}
	ctx, stop := operationContext(command)
	defer stop()
	runtimeManager := managed.Manager{Context: ctx, ActivityDetails: o.ActivityDetails, Diagnostics: o.Diagnostics, ForceRepair: command == "repair"}
	var report func(string)
	if command == "setup" {
		style := display.Detect(os.Stderr)
		// Keep Setup progress ordered before the next interactive input prompt.
		runtimeManager.Progress = func(message string) { fmt.Fprint(os.Stderr, style.Format(message)) }
	}
	if command == "start" || command == "update" || command == "repair" || command == "doctor" || command == "rollback" {
		style := display.Detect(os.Stderr)
		reporter := display.New(os.Stderr, style)
		defer reporter.Close()
		report = func(message string) { reporter.Report(message) }
		runtimeManager.Progress = report
		runtimeManager.ActivityColor = style.Color
		if command == "start" && o.Diagnostics {
			report("内部診断ログ有効（INFO、HTTP raw無効、認証値をマスク）")
		}
	}
	a := core.App{Context: ctx, PrepareUpdate: runtimeManager.PrepareCommand, InstalledCore: managed.InstalledCore, RunningCore: managed.RunningCore, VerifyCore: managed.VerifyCore, Progress: report, Identity: platform.CurrentIdentity, Vault: platform.NativeVault{}, Prompt: terminal{}, Runtime: runtimeManager, InstallCommand: managed.InstallCommand}
	if command == "setup" {
		a.PreviewCommand = managed.PreviewCommand
		userHome, err := os.UserHomeDir()
		if err != nil {
			return 1, errors.New("ユーザーの保存先を確認できません")
		}
		o, e = a.PrepareSetup(o, os.Stdout, filepath.Join(userHome, ".local", "bin"))
		if e != nil {
			return 1, e
		}
		a.InstallCommand = func(home string) error { return managed.SetupCommand(home, o.CommandDir, terminal{}) }
	}
	switch command {
	case "setup":
		o, e = a.Setup(o)
		if e != nil {
			e = fmt.Errorf("%w\n保存処理中の中断・失敗では、作成済み設定や保存済みキーを保持します。同じ setup で再開できます。", e)
		}
		if e == nil {
			fmt.Printf("Setup を保存しました。次回からの起動: %s\n同じ Tunnel ID の既存プラグインを再利用してください。未登録なら本人が登録します。アカウント側の登録状態は確認していません。\n", managed.StartCommand(o.Home))
		}
	case "start":
		return a.Start(o)
	case "doctor":
		var result map[string]any
		if o.Device == "" {
			if selected, err := core.Resolve(o); err == nil {
				result, e = a.Doctor(selected)
			} else {
				result, e = a.DoctorHome(o.Home)
			}
		} else {
			result, e = a.Doctor(o)
		}
		if result != nil {
			b, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(b))
		}
	case "credentials":
		e = a.Credentials(o)
	case "update", "repair":
		e = a.Update(o, false)
		if e == nil {
			fmt.Println("Pinned runtime ready; applies at next start. Existing process was not restarted.")
		}
	case "rollback":
		e = a.Update(o, true)
	case "plugin":
		e = a.Plugin(o)
	}
	if e != nil {
		return 1, e
	}
	return 0, nil
}
func main() {
	code, e := run(os.Args[1:])
	if e != nil { // Never print native provider errors, stack traces, environment or key bytes.
		fmt.Fprintln(os.Stderr, e.Error())
	}
	if code < 0 {
		code = 1
	}
	os.Exit(code)
}
