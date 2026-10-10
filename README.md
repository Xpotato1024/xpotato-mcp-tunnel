# xpotato-mcp-tunnel

Go コア 0.2.0 は Windows／Linux の資格情報保存を分離し、Setup の保存同意・非表示キー入力後は、インストール済みコマンド一つで起動します。PS/cmd/sh は引数を転送するだけです。固定版 Node は Desktop Commander の実行用に管理します。

```text
go build -trimpath ./cmd/xpotato-tunnel
xpotato-tunnel setup
```

通常ユーザーで Setup を実行すると、端末名・Tunnel ID・許可ディレクトリとキー保存同意を確認します。以後は生成した `bin/xpotato-tunnel.cmd`（Windows）または `bin/xpotato-tunnel`（Linux）を起動します。Start で npm/Git を取得したりキーを再入力したりする処理はありません。

詳細と本人の次のコマンドは [Go Setup と起動](docs/setup-once-core.md) を参照してください。Windows はユーザー Credential Manager、Linux は解除済みのユーザー Secret Service を使用します。Linux headless／ロック中には平文へフォールバックせず停止します。

**プラグインのアカウント登録は未自動化です。** Setup は Tunnel ID と登録案内を生成します。本人が検証した直接 HTTPS MCP URL を指定した場合は portable plugin bundle も生成しますが、アップロード・有効化は行いません。公式の Tunnel 登録は [ChatGPT Plugins の手順](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels#connect-from-chatgpt) に従います。

開発検証は `go test -count=1 ./...`、`go vet ./...`、`npm run check`、`npm test` です。実キー・実 Vault・実接続を使わないテストで、旧 Node CLI の検証も保持しています。

## 旧 Node CLI 0.1.1 の記録

以下は PR #1 のプロトタイプの手順です。Go コアの入口は上の案内を使用してください。

Desktop Commander と OpenAI Secure MCP Tunnel を通常ユーザーで利用するための薄い Windows 向け Node CLI です。`setup / start / doctor / update` を提供します。

現版は安全な起動基盤の試作です。目標である「setup で一度キーを入力し接続/プラグインを登録、以後ワンコマンドで追加入力なく起動」は未実装です。現在の start は毎回マスク入力します。資格情報保管と接続登録の設計は次段階で、現版の手順を最終 UX として扱いません。

コードとの差分と最小の拡張案は [次段階の setup 設計](docs/next-setup-design.md) を参照してください。

Windows x64、Node.js 22 以上と付属 npm、Windows PowerShell が必要です。この private リポジトリを取得できる Git の認証は事前に本人が済ませてください。

setup した通常ユーザーを profile の所有者として記録し、管理者実行と別ユーザーによる利用を拒否します。SID はローカル設定に保存し、このリポジトリへ端末固有の SID を埋め込みません。Chrome の初期化時の自動確認・取得・cache 整理は抑止します。

```powershell
# Draft PR のレビュー用ブランチ。運用時はレビュー済み commit SHA に固定する。
npx --yes --package=github:Xpotato1024/xpotato-mcp-tunnel#feat/windows-tunnel-cli xpotato-mcp-tunnel --help

# C:\Work が存在することを確認し、本人の Tunnel ID を指定する。
npx --yes --package=github:Xpotato1024/xpotato-mcp-tunnel#feat/windows-tunnel-cli xpotato-mcp-tunnel setup --device llm-01 --tunnel-id tunnel_YOUR_ID --allow 'C:\Work'
npx --yes --package=github:Xpotato1024/xpotato-mcp-tunnel#feat/windows-tunnel-cli xpotato-mcp-tunnel doctor --device llm-01
npx --yes --package=github:Xpotato1024/xpotato-mcp-tunnel#feat/windows-tunnel-cli xpotato-mcp-tunnel start --device llm-01
```

`start` のキーはターミナルでマスク入力します。API キーをコマンド、設定 JSON、Git、npm パラメータに書く必要はありません。各コマンドの `--home 'C:\別の保存先'` で保存先を変更できます。同じ端末には毎回同じ保存先を指定してください。

キーなしで help/doctor を確認できます。初回の setup は公式 ZIP と npm パッケージを取得するためネットワークを使います。実行中の既存 Tunnel の切り替えは本人がタイミングを選んで行ってください。

| コマンド | 動作 |
| --- | --- |
| `setup` | 固定バージョンを準備し、新規端末設定を作成。既存設定・許可ディレクトリを保持 |
| `start` | 保存済み設定でフォアグラウンド起動。起動のたびにキーを入力 |
| `doctor` | 設定、既知ファイルの SHA256、lock を読み取り確認。接続・認証は試行しない |
| `update` | この CLI 版に同梱された manifest へ更新。設定を保持し、次回起動から適用 |

現在の固定バージョンは **tunnel-client runtime v0.0.16 / Desktop Commander 0.2.52** です。`update` は registry の latest を選びません。新しい固定バージョンは manifest と互換性確認をレビューした CLI commit で導入します。

運用・制限・再起動は [運用メモ](docs/operations.md)、検証範囲と本人の受け入れ確認は [検証メモ](docs/validation.md) を参照してください。

開発時は `npm test` と `npm run check` を実行します。テストは一時ディレクトリ、合成キー、依存インストールの mock を使い、既存の Tunnel を起動・停止しません。

このリポジトリは app ソースと固定 runtime 参照を管理します。端末固有の設定・資格情報・実行ログをコミットしません。Home-Servers へ統合する場合も、アプリ成果物のバージョン参照を接点にします。
