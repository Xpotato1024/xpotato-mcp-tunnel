package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/managed"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
	"golang.org/x/term"
	"os"
	"strings"
)

const help = `xpotato-tunnel ` + core.Version + ` (Windows/Linux amd64)
  setup [--device NAME] [--tunnel-id ID] [--allow ABSOLUTE_DIR ...]
  start | doctor | update | rollback | credentials | plugin
  --home ABSOLUTE_DIR  (all commands)
  --device NAME       (omit to use setup's default device)
  --mcp-url HTTPS_URL (setup only; owner-verified direct MCP endpoint, optional)
  --command-dir DIR  (setup only; optional short command location, local consent)
Setup requests masked key input and local save consent once.
Start uses the saved key without prompting; no arguments defaults to start.
Tunnel plugin account registration requires the ChatGPT Plugins UI.
`

type terminal struct{}

func (terminal) Text(label string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("interactive terminal required for setup")
	}
	fmt.Print(label + ": ")
	line, e := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line), e
}
func (t terminal) Consent(label string) (bool, error) {
	line, e := t.Text(label)
	return line == "yes", e
}
func (terminal) Key() ([]byte, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil, errors.New("interactive terminal required")
	}
	fmt.Print("Runtime API key (input hidden): ")
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
	if command == "version" {
		return command, core.Options{}, nil
	}
	if !strings.Contains("|setup|start|doctor|update|rollback|credentials|plugin|", "|"+command+"|") {
		return "", core.Options{}, errors.New("unknown command; use help")
	}
	o := core.Options{}
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	f.SetOutput(new(strings.Builder))
	f.StringVar(&o.Home, "home", "", "home")
	f.StringVar(&o.Device, "device", "", "device")
	f.StringVar(&o.Tunnel, "tunnel-id", "", "tunnel")
	f.StringVar(&o.MCPURL, "mcp-url", "", "endpoint")
	f.StringVar(&o.CommandDir, "command-dir", "", "short command directory")
	var allow repeated
	f.Var(&allow, "allow", "root")
	if e := f.Parse(args); e != nil || f.NArg() != 0 {
		return "", o, errors.New("invalid options; use help (keys are accepted only by hidden terminal input)")
	}
	o.Allow = allow
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
	if command != "setup" && !(command == "doctor" && o.Device == "") {
		var e error
		o, e = core.Resolve(o)
		if e != nil {
			return "", o, e
		}
	}
	return command, o, nil
}
func run(args []string) (int, error) {
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
	a := core.App{Identity: platform.CurrentIdentity, Vault: platform.NativeVault{}, Prompt: terminal{}, Runtime: managed.Manager{}, InstallCommand: managed.InstallCommand}
	if command == "setup" {
		a.InstallCommand = func(home string) error { return managed.SetupCommand(home, o.CommandDir, terminal{}) }
	}
	switch command {
	case "setup":
		o, e = a.Setup(o)
		if e == nil {
			fmt.Printf("Setup saved. Start: %s\nReuse your existing plugin for this same Tunnel ID; only create a plugin if none exists. Account registration was not checked.\n", managed.StartCommand(o.Home))
		}
	case "start":
		return a.Start(o)
	case "doctor":
		var result map[string]any
		if o.Device == "" {
			result, e = a.DoctorHome(o.Home)
		} else {
			result, e = a.Doctor(o)
		}
		if e == nil {
			b, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(b))
		}
	case "credentials":
		e = a.Credentials(o)
	case "update":
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
