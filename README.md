# xpotato-mcp-tunnel

Desktop Commander と OpenAI Secure MCP Tunnel を通常ユーザーで利用するための薄い Windows 向け Node CLI です。`setup / start / doctor / update` を提供します。

Windows x64、Node.js 22 以上と付属 npm、Windows PowerShell が必要です。この private リポジトリを取得できる Git の認証は事前に本人が済ませてください。

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
