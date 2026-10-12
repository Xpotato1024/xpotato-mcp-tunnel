# xpotato-mcp-tunnel

Windows/Linux amd64で、端末内のDesktop CommanderをOpenAIトンネルにつなぐGoコアです。通常ユーザーが一度Setupし、次回から保存済みキーで起動できます。Node/Commander/Tunnelは固定runtimeに同梱し、利用者のGo/Node/npm導入は不要です。

採用済みmainは0.2.6（`5d2c32c1`）。このブランチの0.2.7はレビュー候補です。旧Node CLIはGit追跡コードから撤去しました。PC上の旧配布物・バックアップ・設定・資格情報を削除する機能はありません。

## 使い始める・更新する

[導入の入口](docs/quickstart.md) に、配布物の確認、新規Setup、既存環境のupdate、起動・停止をまとめています。配布は現在のprivate CI review artifactです。新しい公開配布先やインストーラは追加していません。

- 初めて使う: レビュー済み配布物を展開し、通常の対話端末で新exeの`setup`を実行。
- 既に使っている: 新しい配布exeの`update`を実行。同じhome/deviceと保存済みキーを再利用。
- 困ったとき: [確認・診断・再開](docs/operations.md)。設定を削除せず、保存版と稼働版を分けて確認。

通常は「OpenAI トンネル」の接続変化とMCP処理イベント、警告・エラーをstderrへ表示し、正常待機の定例通知を出しません。内部INFOは`start --diagnostics`で明示表示します。`--activity-details`による引数表示は別の選択です。色だけに依存せず文字ラベルを併記し、非TTY/NO_COLORでは無色です。

## 仕様と開発

- [Setup・資格情報・更新の詳細](docs/setup-once-core.md)
- [非秘密.env入力と変更確認](docs/setup-env.md)
- [表示と出力上限](docs/runtime-ux.md)
- [隔離回帰と互換性](docs/validation.md)
- [過去の測定と制約](docs/performance-review.md)

```text
go test -count=1 ./...
go vet ./...
go build -trimpath ./cmd/xpotato-tunnel
node --check internal/managed/assets/stdio.mjs
node --check internal/managed/assets/activity.mjs
node --test test/activity.test.mjs
```

開発用Nodeは24以上。現行MCPアダプターと固定依存lockは`internal/managed/assets`にあります。GoのChrome自動取得抑止、旧core/shimの更新互換、不変runtimeと旧版保持は残しています。Go版のbenchmarkツールと0.2.5測定履歴も保持します。旧npm/npx CLI入口とその専用テストはありません。

CIはWindows/LinuxのGo test/vet、隔離runtime連携、Node activity検証とreview package作成のみです。push/PRから本人端末へのデプロイ・公開・接続変更は行いません。
