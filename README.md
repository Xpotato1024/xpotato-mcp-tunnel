# xpotato-mcp-tunnel

> 開発中: Installer と通常経路の全件 runtime 再走査の見直しは [設計・未検証項目](docs/installer-lifecycle.md) を参照してください。未公開 Draft であり、現在の配布は v0.2.10 のままです。

**Xpotato.net / Xpotato1024** — プロジェクト固有コードは **AGPL-3.0-only**。
[正式全文](LICENSE)・[帰属方針](COPYRIGHT)・[対応ソースと第三者依存の条件](docs/licensing.md)。
`xpotato-tunnel about` / `license` / `source` で利用中の版からも確認できます。

0.2.10では[更新処理の世代切替・取消・検証](docs/update-transactions.md)を改善し、Windowsの取得とdigest検証を一回の操作へまとめています。

Windows/Linux amd64で、端末内のDesktop CommanderをOpenAIトンネルにつなぐGoコアです。Laptopで使うものは **Windows amd64用の配布物に入っている `xpotato-tunnel.exe`** です。Go/Node/npmやソースコードの事前導入は不要です。通常ユーザーが一度Setupし、次回から保存済みキーで起動できます。

正式配布は **[v0.2.10](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/tag/v0.2.10)** です。同版の異なるバイナリを上書きしないため、既存0.2.7／0.2.8から更新する場合もこのReleaseのexeを使います。`setup`/`update`は固定版のNode・Commander・Tunnelをネットワークから取得します。配布物は完全オフライン用runtime同梱版ではありません。旧Node CLIは撤去済みです。

**v0.2.9は通常起動のログ設定エラーを修正しています。** 旧v0.2.8で`log level requires 'struct-text' or 'json' log format`と終了した場合も、新exeの`update`で既存設定・保存済みキー・Tunnel IDを維持して更新します。[旧版の障害と一時的な起動方法](docs/operations.md#v028のログ設定エラー)も参照できます。旧版tag/assetsは上書きしません。

## 使い始める・更新する

[導入の入口](docs/quickstart.md) に、入手先の選び方、展開、checksum/version確認、新規Setup、既存環境のupdate、起動・停止をまとめています。

- Windows Laptop: [固定Release v0.2.10](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/tag/v0.2.10)本文の検証ブロックを一回実行。ZIP取得・GitHub側digest照合・安全な展開まで行い、別のSHAファイルは不要です。
- Linux: [v0.2.10 tar.gz](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.10/xpotato-tunnel-0.2.10-linux-amd64.tar.gz)と[checksum](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.10/xpotato-tunnel-0.2.10-linux-amd64.tar.gz.sha256)。
- 公開後は認証なしで取得できます。[Actions](https://github.com/Xpotato1024/xpotato-mcp-tunnel/actions/workflows/windows-tests.yml)の14日期限artifactはレビュー・検証用で、正式導入にはReleaseを選びます。

Windowsは[quickstart](docs/quickstart.md)を読み、固定Releaseの検証ブロックを全体で実行します。APIやdigestの失敗時は止まり、既存の展開先は上書きしません。exe・setup・updateは自動実行しません。

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
