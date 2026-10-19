# xpotato-mcp-tunnel

Windows/Linux amd64で、端末内のDesktop CommanderをOpenAIトンネルにつなぐGoコアです。Laptopで使うものは **Windows amd64用の配布物に入っている `xpotato-tunnel.exe`** です。Go/Node/npmやソースコードの事前導入は不要です。通常ユーザーが一度Setupし、次回から保存済みキーで起動できます。

正式配布は **[v0.2.8](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/tag/v0.2.8)** です。同版の異なるバイナリを上書きしないため、既存0.2.7から更新する場合もこのReleaseのexeを使います。`setup`/`update`は固定版のNode・Commander・Tunnelをネットワークから取得します。配布物は完全オフライン用runtime同梱版ではありません。旧Node CLIは撤去済みです。

## 使い始める・更新する

[導入の入口](docs/quickstart.md) に、入手先の選び方、展開、checksum/version確認、新規Setup、既存環境のupdate、起動・停止をまとめています。

- Windows Laptop: [v0.2.8 ZIP](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.8/xpotato-tunnel-0.2.8-windows-amd64.zip)と[checksum](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.8/xpotato-tunnel-0.2.8-windows-amd64.zip.sha256)。一度展開するとexeとガイドが出ます。
- Linux: [v0.2.8 tar.gz](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.8/xpotato-tunnel-0.2.8-linux-amd64.tar.gz)と[checksum](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.8/xpotato-tunnel-0.2.8-linux-amd64.tar.gz.sha256)。
- privateリポジトリのためアクセス権とGitHubログインが必要です。[Actions](https://github.com/Xpotato1024/xpotato-mcp-tunnel/actions/workflows/windows-tests.yml)の14日期限artifactはレビュー・検証用で、正式導入にはReleaseを選びます。

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
- [配布物の作成・版付きReleaseの手動手順](docs/distribution.md)

```text
go test -count=1 ./...
go vet ./...
go build -trimpath ./cmd/xpotato-tunnel
node --check internal/managed/assets/stdio.mjs
node --check internal/managed/assets/activity.mjs
node --test test/activity.test.mjs
```

開発用Nodeは24以上。現行MCPアダプターと固定依存lockは`internal/managed/assets`にあります。GoのChrome自動取得抑止、旧core/shimの更新互換、不変runtimeと旧版保持は残しています。Go版のbenchmarkツールと0.2.5測定履歴も保持します。旧npm/npx CLI入口とその専用テストはありません。

CIはWindows/LinuxのGo test/vet、隔離runtime連携、Node activity検証とレビュー用・Release候補の作成のみです。push/PRから本人端末へのデプロイ・公開・接続変更は行いません。
