package core

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

var ErrSetupCancelled = errors.New("Setup を中断しました")

func cancelled(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "q", "cancel", "中断":
		return true
	}
	return false
}

func wizardText(p Prompt, label string) (string, error) {
	s, e := p.Text(label)
	if e != nil {
		return "", e
	}
	if cancelled(s) {
		return "", ErrSetupCancelled
	}
	return strings.TrimSpace(s), nil
}

// PrepareSetup collects only nonsecret options. It does not create files,
// install a runtime, access the vault, or start/stop processes. Setup rechecks
// identity, immutable scope and state under its maintenance lock afterwards.
func (a App) PrepareSetup(o Options, out io.Writer, shortDefault string) (Options, error) {
	if _, e := a.identity(); e != nil {
		return o, e
	}
	fmt.Fprintln(out, "初回 Setup — 日本語ウィザード")
	fmt.Fprintln(out, "用意するもの: Platform で確認した Tunnel ID、runtime API キー、アクセスを許可する既存フォルダー。")
	fmt.Fprintln(out, "本人の通常ユーザー端末で実行します。Windows は Credential Manager、Linux はログインセッションの解除済み Secret Service が必要です。")
	fmt.Fprintln(out, "Tunnel ID は tunnel_ で始まる接続先の識別子です。API キーとは別物です。キーは最後の保存同意の後に非表示で入力します。")
	fmt.Fprintln(out, "許可したフォルダーとその配下が Commander のアクセス範囲になります。必要な範囲だけを選んでください。")
	fmt.Fprintln(out, "初回は固定版の実行環境をダウンロードします。Setup 後も使う恒久保存先は次のとおりです。")
	printLocations(out, o, shortDefault, nil)
	fmt.Fprintln(out, "入力例: 端末名 work-pc / Tunnel ID tunnel_YOUR_ID / Windows C:\\Work / Linux /home/user/Work")
	fmt.Fprintln(out, "どの通常入力でも q / cancel / 中断、または Ctrl+C で中断できます。最終確認まではファイルを書きません。")
	rootsReviewed := false
	for {
		for validateOptions(Options{Home: o.Home, Device: "preview"}) != nil {
			fmt.Fprintln(out, "保存先は完全な絶対パスで指定してください。引用符・改行などシェルに渡せない文字は使えません。")
			var e error
			o.Home, e = wizardText(a.Prompt, "恒久保存先（例 Windows C:\\Users\\user\\AppData\\Local\\xpotato-mcp-tunnel-v2 / Linux /home/user/.local/share/xpotato-mcp-tunnel-v2）")
			if e != nil {
				return o, e
			}
		}
		for !namePattern.MatchString(o.Device) {
			fmt.Fprintln(out, "端末名はこの保存先で設定を識別する名前です。英小文字・数字・ハイフンで 1〜64 文字、先頭は英小文字または数字にしてください。")
			var e error
			o.Device, e = wizardText(a.Prompt, "端末名（例 work-pc）")
			if e != nil {
				return o, e
			}
		}
		existing := exists(deviceFile(o))
		o.RefreshRuntime = false
		if existing {
			d, e := a.owned(o)
			if e != nil {
				return o, e
			}
			if (o.Tunnel != "" && o.Tunnel != d.Tunnel) || (o.MCPURL != "" && o.MCPURL != d.MCPURL) || (len(o.Allow) > 0 && !equal(o.Allow, d.Approved)) {
				fmt.Fprintln(out, "この端末名は登録済みです。指定した変更は適用せず、既存の設定を表示します。別の接続や範囲にする場合は確認画面で端末名を変更してください。")
			}
			o.Tunnel, o.MCPURL, o.Allow = d.Tunnel, d.MCPURL, append([]string(nil), d.Approved...)
			if d.Release != "" {
				err := a.Runtime.Verify(o.Home, d.Release)
				if errors.Is(err, ErrRuntimePinUnsupported) {
					o.RefreshRuntime = true
				} else if err != nil {
					return o, errors.New("保存済み runtime の検証に失敗しました。既存状態は変更していません。doctor で状態を確認してください")
				}
			}
			fmt.Fprintln(out, "既存の端末設定を保持して Setup を再開します。完了済みのキー保存は繰り返しません。")
		} else {
			for !tunnelPattern.MatchString(o.Tunnel) {
				fmt.Fprintln(out, "Platform の Tunnel 設定から ID をコピーしてください。ここに API キーを入力しないでください。")
				var e error
				o.Tunnel, e = wizardText(a.Prompt, "Tunnel ID（tunnel_ で始まる ID）")
				if e != nil {
					return o, e
				}
				if !tunnelPattern.MatchString(o.Tunnel) {
					fmt.Fprintln(out, "Tunnel ID の形式が違います。入力をやり直してください。")
				}
			}
			if !rootsReviewed {
				if e := a.editRoots(&o, out); e != nil {
					return o, e
				}
				rootsReviewed = true
			}
		}
		if o.MCPURL != "" {
			if e := validateEndpoint(o.MCPURL); e != nil {
				return o, errors.New("--mcp-url は本人が検証した直接の HTTPS MCP endpoint を指定してください。値は表示していません")
			}
		}
		if !validCommandDirectory(o.CommandDir) {
			fmt.Fprintln(out, "短縮コマンドの保存先は絶対パスにしてください。引用符・改行・%・! は使用できません。")
			var e error
			o.CommandDir, e = wizardText(a.Prompt, "短縮コマンドの保存先（空欄で既定／既存の選択）")
			if e != nil {
				return o, e
			}
			continue
		}
		var commandPlan *CommandPlan
		if a.PreviewCommand != nil {
			plan, err := a.PreviewCommand(o.Home, o.CommandDir)
			if errors.Is(err, ErrCommandDirectoryConflict) {
				fmt.Fprintln(out, "短縮コマンド先は選択済みです。指定した変更を適用せず、既存の保存先を確認画面に表示します。")
				o.CommandDir = ""
				plan, err = a.PreviewCommand(o.Home, "")
			}
			if err != nil {
				return o, err
			}
			commandPlan = &plan
		}
		fmt.Fprintln(out, "\n保存前の確認（秘密情報は含みません）")
		fmt.Fprintf(out, "端末名: %s\nTunnel ID: %s\n", o.Device, o.Tunnel)
		printLocations(out, o, shortDefault, commandPlan)
		if o.RefreshRuntime {
			fmt.Fprintf(out, "実行環境の更新: 確認の yes 後に、このコア %s の固定 runtime を導入・検証し、端末の参照を切り替えます。旧版と設定は保持し、稼働中の接続は再起動しません。\n", Version)
		}
		fmt.Fprintln(out, "許可フォルダー（承認範囲）:")
		printRoots(out, o.Allow)
		if existing {
			fmt.Fprintln(out, "既存の Commander 設定をバイト単位で保持します。ここに表示した範囲内への既存の絞り込みも保持します。")
		} else {
			shell := "/bin/sh"
			if runtime.GOOS == "windows" {
				shell = "powershell.exe"
			}
			fmt.Fprintf(out, "Commander: shell=%s / 読み取り上限 100 行 / 書き込み上限 50 行 / telemetry 無効\n", shell)
			fmt.Fprintln(out, "実行禁止コマンド: shutdown, reboot, format, diskpart, netsh, runas, takeown, reg, sc")
		}
		if o.MCPURL != "" {
			fmt.Fprintln(out, "直接 MCP endpoint: 指定済み（値は表示しません）")
		}
		fmt.Fprintln(out, "キー保存先: Windows は本人の Credential Manager、Linux は本人の Secret Service。専用レコードへの保存は別途明示同意を求めます。")
		choice, e := wizardText(a.Prompt, "進む yes / 端末名を修正 device / Tunnel ID を修正 tunnel / フォルダーを修正 dirs / 保存先を修正 home / 短縮コマンド先を修正 command / 中断 no または q")
		if e != nil {
			return o, e
		}
		switch strings.ToLower(choice) {
		case "yes", "進む":
			return o, nil
		case "no", "いいえ":
			return o, ErrSetupCancelled
		case "device", "端末名":
			o.Device = ""
		case "home", "保存先":
			o.Home, e = wizardText(a.Prompt, "恒久保存先の絶対パス")
		case "tunnel", "接続先":
			if existing {
				fmt.Fprintln(out, "登録済み端末の Tunnel ID は変更できません。device で別の端末名を選んでください。")
			} else {
				o.Tunnel = ""
			}
		case "dirs", "フォルダー":
			if existing {
				fmt.Fprintln(out, "登録済み端末の承認範囲は変更できません。device で別の端末名を選んでください。")
			} else {
				e = a.editRoots(&o, out)
			}
		case "command", "短縮コマンド":
			o.CommandDir, e = wizardText(a.Prompt, "短縮コマンドの保存先（空欄で既定／既存の選択）")
		default:
			fmt.Fprintln(out, "表示した選択肢のいずれかを入力してください。")
		}
		if e != nil {
			return o, e
		}
	}
}

func validCommandDirectory(p string) bool {
	return p == "" || (qualified(p) && !strings.ContainsAny(p, "\r\n\x00\"%!"))
}

func printLocations(out io.Writer, o Options, shortDefault string, commandPlan *CommandPlan) {
	fmt.Fprintf(out, "恒久保存先: %s\n実行環境: %s\nGo 本体: %s\n起動ラッパー: %s\n", o.Home, filepath.Join(o.Home, "releases"), filepath.Join(o.Home, "core", Version), filepath.Join(o.Home, "bin"))
	if o.Device != "" {
		fmt.Fprintf(out, "端末設定: %s\nCommander 設定: %s\n", deviceFile(o), configFile(o))
	} else {
		fmt.Fprintf(out, "端末設定と Commander 設定: %s 内の端末別フォルダー\n", filepath.Join(o.Home, "devices"))
	}
	if commandPlan != nil {
		if commandPlan.Directory == "" {
			fmt.Fprintln(out, "任意の短縮コマンド: 配置辞退済み。恒久保存先の bin を使用します。")
		} else if commandPlan.ConsentRequired {
			fmt.Fprintf(out, "任意の短縮コマンド保存先: %s（別途同意）\n", commandPlan.Directory)
		} else {
			fmt.Fprintf(out, "任意の短縮コマンド保存先: %s（選択・同意済み、維持）\n", commandPlan.Directory)
		}
	} else if o.CommandDir != "" {
		fmt.Fprintf(out, "任意の短縮コマンド保存先: %s（別途同意）\n", o.CommandDir)
	} else {
		fmt.Fprintf(out, "任意の短縮コマンド: 既存の選択を維持。初回の候補 %s（別途同意）\n", shortDefault)
	}
	fmt.Fprintln(out, "PATH は変更しません。Setup 完了後は配布元に依存しません。恒久保存先には残し続けるフォルダーを選んでください。")
}

func printRoots(out io.Writer, roots []string) {
	if len(roots) == 0 {
		fmt.Fprintln(out, "  （まだありません）")
	}
	for i, p := range roots {
		fmt.Fprintf(out, "  %d. %s\n", i+1, p)
	}
}

func validRoot(p string) bool {
	st, e := os.Stat(p)
	return qualified(p) && e == nil && st.IsDir()
}

func (a App) editRoots(o *Options, out io.Writer) error {
	fmt.Fprintln(out, "許可フォルダーを編集します。1 回に 1 フォルダーを入力します。空白を含むパスもそのまま入力し、外側の引用符は不要です。")
	for {
		printRoots(out, o.Allow)
		choice, e := wizardText(a.Prompt, "追加 add / 削除 remove / 一覧 review / 確定 done / 中断 q")
		if e != nil {
			return e
		}
		switch strings.ToLower(choice) {
		case "add", "追加":
			p, e := wizardText(a.Prompt, "既存フォルダーの絶対パス（例 C:\\Work または /home/user/Work）")
			if e != nil {
				return e
			}
			// Pasted shell-style outer quotes are a common recoverable mistake.
			if len(p) >= 2 && ((p[0] == '"' && p[len(p)-1] == '"') || (p[0] == '\'' && p[len(p)-1] == '\'')) {
				p = p[1 : len(p)-1]
			}
			if !validRoot(p) {
				fmt.Fprintln(out, "存在するフォルダーの完全な絶対パスが必要です。追加していません。add で再入力できます。")
				continue
			}
			p = filepath.Clean(p)
			duplicate := false
			for _, old := range o.Allow {
				if old == p || (runtime.GOOS == "windows" && strings.EqualFold(filepath.Clean(old), p)) {
					duplicate = true
				}
			}
			if duplicate {
				fmt.Fprintln(out, "このフォルダーは追加済みです。")
			} else {
				o.Allow = append(o.Allow, p)
			}
		case "remove", "削除":
			s, e := wizardText(a.Prompt, "削除する番号（空欄で一覧へ戻る）")
			if e != nil {
				return e
			}
			if s == "" {
				continue
			}
			n, e := strconv.Atoi(s)
			if e != nil || n < 1 || n > len(o.Allow) {
				fmt.Fprintln(out, "一覧にある番号を入力してください。削除していません。")
				continue
			}
			o.Allow = append(o.Allow[:n-1], o.Allow[n:]...)
		case "review", "一覧":
		case "done", "確定":
			valid := len(o.Allow) > 0
			for _, p := range o.Allow {
				valid = valid && validRoot(p)
			}
			if valid {
				return nil
			}
			fmt.Fprintln(out, "許可フォルダーは一つ以上必要です。無効なパスは remove で削除し、add で既存の絶対パスを追加してください。")
		default:
			fmt.Fprintln(out, "add / remove / review / done / q のいずれかを入力してください。")
		}
	}
}
