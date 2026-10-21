package main

import (
	"errors"
	"fmt"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/core"
	"github.com/Xpotato1024/xpotato-mcp-tunnel/internal/managed"
	"golang.org/x/term"
	"os"
)

// The installer and Start menu share the existing consent and masked-key flow.
// Selecting an action is explicit; opening this menu never changes configuration.
func maintenance(o core.Options) (int, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return 1, errors.New("maintenance requires an interactive terminal")
	}
	for {
		fmt.Println("Xpotato Tunnel: 1 初回設定 / 2 起動 / 3 更新 / 4 修復 / 5 簡易診断 / 6 全依存の診断 / q 終了")
		fmt.Println("修復は新しい runtime を取得します。全依存の診断は取得・全件走査を伴う場合があります。既存プロセスは停止しません。")
		choice, err := (terminal{}).Text("操作番号")
		if errors.Is(err, core.ErrSetupCancelled) {
			return 0, nil
		}
		if err != nil {
			return 1, err
		}
		command := ""
		switch choice {
		case "1":
			command = "setup"
		case "2":
			command = "start"
		case "3":
			command = "update"
		case "4":
			command = "repair"
		case "5", "6":
			command = "doctor"
		default:
			continue
		}
		args := []string{command}
		if o.Device != "" {
			args = append(args, "--device", o.Device)
		}
		if choice == "6" {
			args = append(args, "--deep")
		}
		args = append(args, "--home", o.Home)
		var code int
		if command == "start" {
			ctx, stop := operationContext("start")
			selected, resolveErr := core.Resolve(o)
			if resolveErr == nil {
				_, resolveErr = managed.DispatchTarget(ctx, o.Home, selected.Device)
			}
			if resolveErr == nil {
				code, err = dispatch(ctx, args)
			} else {
				// A fresh Setup may predate stable dispatch. The trusted current
				// core still enforces owner/scope and its selected core pin.
				code, err = run(args)
			}
			stop()
		} else {
			code, err = run(args)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "操作失敗:", err)
		} else {
			fmt.Printf("操作完了 (終了コード %d)。\n", code)
		}
	}
}
