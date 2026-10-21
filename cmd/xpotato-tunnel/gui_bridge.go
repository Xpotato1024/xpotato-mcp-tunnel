package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/managed"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/platform"
)

// Private inherited stdin/stdout pipes only. No listener, token, HTTP endpoint,
// command string, arbitrary executable or persisted GUI credential is added.
const guiFrameLimit = 64 * 1024

type guiMessage struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Action string `json:"action,omitempty"`
	Home   string `json:"home,omitempty"`
	Device string `json:"device,omitempty"`
	Prompt uint64 `json:"prompt,omitempty"`
	Value  []byte `json:"value,omitempty"`
}
type guiEvent struct {
	Type      string              `json:"type"`
	ID        string              `json:"id,omitempty"`
	Prompt    uint64              `json:"prompt,omitempty"`
	Kind      string              `json:"kind,omitempty"`
	Message   string              `json:"message,omitempty"`
	Details   string              `json:"details,omitempty"`
	Choices   []core.PromptChoice `json:"choices,omitempty"`
	Version   string              `json:"version,omitempty"`
	Platform  string              `json:"platform,omitempty"`
	Supported bool                `json:"supported,omitempty"`
	Home      string              `json:"home,omitempty"`
	Result    map[string]any      `json:"result,omitempty"`
}
type guiExecute func(context.Context, guiMessage, *guiPrompt) (string, map[string]any, error)

var guiID = regexp.MustCompile(`^[a-zA-Z0-9-]{1,64}$`)

func guiAction(action string) bool {
	switch action {
	case "setup", "start", "update", "repair", "doctor", "deep", "rollback", "credentials", "uninstall-entrypoints":
		return true
	}
	return false
}

func guiBridge(input io.Reader, output io.Writer) (int, error) {
	home, err := core.DefaultHome()
	if err != nil {
		return 1, errors.New("GUI home unavailable")
	}
	return serveGUI(input, output, guiRunOperation, guiEvent{Type: "hello", Version: core.Version,
		Home: home, Platform: runtime.GOOS + "/" + runtime.GOARCH,
		Supported: (runtime.GOOS == "windows" || runtime.GOOS == "linux") && runtime.GOARCH == "amd64"})
}

func serveGUI(input io.Reader, output io.Writer, execute guiExecute, hello guiEvent) (int, error) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	var outputMu sync.Mutex
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	emit := func(event guiEvent) {
		outputMu.Lock()
		defer outputMu.Unlock()
		if encoder.Encode(event) != nil {
			stop()
		}
	}
	emit(hello)
	incoming := make(chan guiMessage)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 4096), guiFrameLimit)
		for scanner.Scan() {
			frame := scanner.Bytes()
			var message guiMessage
			err := json.Unmarshal(frame, &message)
			clear(frame) // numeric byte arrays avoid immutable Go secret strings
			if err != nil {
				clear(message.Value)
				return
			}
			select {
			case incoming <- message:
			case <-ctx.Done():
				clear(message.Value)
				return
			}
		}
	}()
	type completion struct {
		id, home  string
		result    map[string]any
		err       error
		cancelled bool
	}
	completed := make(chan completion, 2)
	type operation struct {
		prompt     *guiPrompt
		cancel     context.CancelFunc
		connection bool
	}
	active := map[string]operation{}
	cancelAll := func() {
		for _, operation := range active {
			operation.cancel()
		}
	}
	closing := false
	contextDone := ctx.Done()
	for {
		select {
		case <-readDone:
			readDone = nil
			closing = true
			if len(active) == 0 {
				return 0, nil
			}
			cancelAll()
		case <-contextDone:
			if len(active) == 0 {
				return 0, nil
			}
			closing = true
			cancelAll()
			// Disable this permanently-ready channel while the owned operation
			// drains. Completion, not process enumeration/kill, ends shutdown.
			contextDone = nil
		case result := <-completed:
			operation := active[result.id]
			operation.cancel()
			for draining := true; draining; {
				select {
				case reply := <-operation.prompt.replies:
					clear(reply.Value)
				default:
					draining = false
				}
			}
			delete(active, result.id)
			kind, message := "complete", "操作が完了しました。"
			if result.cancelled {
				kind, message = "cancelled", "中断しました。作成済みの状態は保持されます。"
			} else if errors.Is(result.err, context.DeadlineExceeded) {
				kind, message = "timeout", "処理期限に達しました。作成済みの状態を保持しています。通信と保存先を確認して再試行できます。"
			} else if result.err != nil {
				kind, message = "failed", "操作に失敗しました。保存済みの状態を保持しています。設定・診断を確認してください。起動で失敗する場合は、このアプリで更新してから再試行してください。"
				if errors.Is(result.err, platform.ErrLocked) {
					message = "OS の資格情報ストアがロックされています。通常のログインセッションで解除してから再試行してください。"
				}
				if errors.Is(result.err, platform.ErrUnavailable) {
					message = "OS の資格情報ストアを利用できません。通常のログインセッションを確認してください。"
				}
			}
			emit(guiEvent{Type: kind, ID: result.id, Message: message, Home: result.home, Result: result.result})
			if closing && len(active) == 0 {
				return 0, nil
			}
		case message := <-incoming:
			switch message.Type {
			case "shutdown":
				clear(message.Value)
				closing = true
				if len(active) == 0 {
					return 0, nil
				}
				cancelAll()
			case "cancel":
				clear(message.Value)
				if operation, ok := active[message.ID]; ok {
					operation.cancel()
				}
			case "reply":
				operation, ok := active[message.ID]
				if !ok || message.Prompt == 0 || message.Prompt != operation.prompt.sequence.Load() || len(message.Value) > 8192 {
					clear(message.Value)
					continue
				}
				select {
				case operation.prompt.replies <- message:
				default:
					clear(message.Value)
				}
			case "operation":
				clear(message.Value)
				_, duplicate := active[message.ID]
				busy := duplicate
				for _, operation := range active {
					if operation.connection == (message.Action == "start") {
						busy = true
					}
				}
				if closing || busy || !guiID.MatchString(message.ID) || !guiAction(message.Action) || len(message.Home) > 4096 || len(message.Device) > 64 {
					emit(guiEvent{Type: "rejected", ID: message.ID, Message: "実行中の操作、または入力を確認してください。"})
					continue
				}
				if !hello.Supported {
					emit(guiEvent{Type: "failed", ID: message.ID, Message: "この OS/CPU の runtime は未対応です。設定や資格情報は変更していません。"})
					continue
				}
				operationContext, operationCancel := context.WithCancel(ctx)
				prompt := &guiPrompt{ctx: operationContext, id: message.ID, emit: emit, replies: make(chan guiMessage, 1)}
				active[message.ID] = operation{prompt, operationCancel, message.Action == "start"}
				emit(guiEvent{Type: "working", ID: message.ID, Message: "操作を開始しました。"})
				go func(request guiMessage, prompt *guiPrompt) {
					home, result, err := execute(prompt.ctx, request, prompt)
					completed <- completion{request.ID, home, result, err, prompt.ctx.Err() != nil || errors.Is(err, core.ErrSetupCancelled)}
				}(message, prompt)
			default:
				clear(message.Value)
			}
		}
	}
}

type guiPrompt struct {
	ctx      context.Context
	id       string
	emit     func(guiEvent)
	replies  chan guiMessage
	sequence atomic.Uint64
	notes    strings.Builder
}

func (p *guiPrompt) Write(data []byte) (int, error) {
	// Wizard output contains reviewed nonsecret paths/scope only. Keep a bounded
	// current-step summary; never forward runtime logs or replies to the UI.
	if p.notes.Len()+len(data) <= 16000 {
		p.notes.Write(data)
	}
	return len(data), nil
}
func (p *guiPrompt) ask(kind, label string, choices []core.PromptChoice) ([]byte, error) {
	sequence := p.sequence.Add(1)
	p.emit(guiEvent{Type: "prompt", ID: p.id, Prompt: sequence, Kind: kind, Message: label, Details: p.notes.String(), Choices: choices})
	p.notes.Reset()
	for {
		select {
		case <-p.ctx.Done():
			return nil, core.ErrSetupCancelled
		case reply := <-p.replies:
			if reply.Prompt != sequence {
				clear(reply.Value)
				continue
			}
			return reply.Value, nil
		}
	}
}
func (p *guiPrompt) Text(label string) (string, error) {
	value, err := p.ask("text", label, nil)
	defer clear(value)
	return strings.TrimSpace(string(value)), err
}
func (p *guiPrompt) Choose(label string, choices []core.PromptChoice) (string, error) {
	value, err := p.ask("choice", label, choices)
	defer clear(value)
	return string(value), err
}
func (p *guiPrompt) Consent(label string) (bool, error) {
	value, err := p.ask("consent", label, []core.PromptChoice{{Value: "yes", Label: "同意する"}, {Value: "no", Label: "同意しない"}})
	defer clear(value)
	return string(value) == "yes", err
}
func (p *guiPrompt) Key() ([]byte, error) {
	return p.ask("secret", "この端末の runtime API キー", nil)
}

func guiRunOperation(ctx context.Context, request guiMessage, prompt *guiPrompt) (string, map[string]any, error) {
	o := core.Options{Home: request.Home, Device: request.Device, Deep: request.Action == "deep"}
	if o.Home == "" {
		var err error
		o.Home, err = core.DefaultHome()
		if err != nil {
			return "", nil, err
		}
	}
	progress := func(message string) { prompt.emit(guiEvent{Type: "progress", ID: request.ID, Message: message}) }
	manager := managed.Manager{Context: ctx, ForceRepair: request.Action == "repair", Progress: progress, Input: strings.NewReader(""), Log: io.Discard, HideWindows: true}
	a := core.App{Context: ctx, PrepareUpdate: manager.PrepareCommand, InstalledCore: managed.InstalledCore,
		RunningCore: managed.RunningCore, VerifyCore: managed.VerifyCore, Progress: progress,
		Identity: platform.CurrentIdentity, Vault: platform.NativeVault{}, Prompt: prompt, Runtime: manager, InstallCommand: managed.InstallCommand}
	if request.Action == "setup" {
		a.PreviewCommand = managed.PreviewCommand
		home, err := os.UserHomeDir()
		if err != nil {
			return o.Home, nil, err
		}
		o, err = a.PrepareSetup(o, prompt, filepath.Join(home, ".local", "bin"))
		if err != nil {
			return o.Home, nil, err
		}
		a.InstallCommand = func(home string) error { return managed.SetupCommand(home, o.CommandDir, prompt) }
		o, err = a.Setup(o)
		return o.Home, nil, err
	}
	if request.Action == "uninstall-entrypoints" {
		yes, err := prompt.Consent("この保存先の起動コマンドを削除します。プロファイル・保存済みキー・runtime と既存プロセスは保持します。アプリ本体は OS のアプリ削除から取り除いてください。")
		if err != nil || !yes {
			return o.Home, nil, core.ErrSetupCancelled
		}
		return o.Home, nil, managed.UninstallEntrypoints(o.Home)
	}
	if request.Action == "doctor" && o.Device == "" {
		if selected, err := core.Resolve(o); err == nil {
			result, err := a.Doctor(selected)
			return o.Home, result, err
		}
		result, err := a.DoctorHome(o.Home)
		return o.Home, result, err
	}
	var err error
	o, err = core.Resolve(o)
	if err != nil {
		return o.Home, nil, err
	}
	switch request.Action {
	case "start":
		code, err := a.Start(o)
		if code != 0 && err == nil {
			err = errors.New("owned runtime exited unsuccessfully")
		}
		return o.Home, nil, err
	case "update", "repair":
		err = a.Update(o, false)
	case "rollback":
		err = a.Update(o, true)
	case "credentials":
		err = a.Credentials(o)
	case "doctor", "deep":
		result, err := a.Doctor(o)
		return o.Home, result, err
	default:
		err = errors.New("unsupported GUI action")
	}
	return o.Home, nil, err
}
