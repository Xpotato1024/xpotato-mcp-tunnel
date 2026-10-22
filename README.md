# xpotato-mcp-tunnel

Windows x64 / Ubuntu 24.04 desktop x64 で、端末内の Desktop Commander を OpenAI トンネルにつなぐアプリです。導入・接続・更新・修復は共通の日本語 GUI、Tunnel core は Go を使用します。Mac は同じ共通実装を維持し、公開を保留しています。

**Xpotato.net / Xpotato1024** — プロジェクト固有コードは **AGPL-3.0-only**。
[正式全文](LICENSE)・[帰属方針](COPYRIGHT)・[対応ソースと第三者依存の条件](docs/licensing.md)。
`xpotato-tunnel about` / `license` / `source` でも利用中の版を確認できます。

## 使い始める・更新する

[v0.2.12 Release](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/tag/v0.2.12) から OS 用の導入ファイルを取得します。公開前のリンクは利用できません。Actions の期限付き artifact は開発・レビュー用です。

| 環境 | 通常導入の入口 |
| --- | --- |
| Windows x64 | [xpotato-tunnel-0.2.12-windows-amd64-setup.exe](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.12/xpotato-tunnel-0.2.12-windows-amd64-setup.exe) |
| Ubuntu 24.04 desktop x64 | [xpotato-tunnel-0.2.12-linux-amd64.deb](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.12/xpotato-tunnel-0.2.12-linux-amd64.deb) |

詳しい [導入・更新・修復・削除](docs/gui-lifecycle.md) を参照してください。Windows の Installer は英語画面、導入後のアプリは日本語です。Windows は既存の WebView2 Evergreen、Ubuntu は WebKitGTK 4.1 とユーザーの Secret Service が前提です。Windows 候補は未署名で、発行者署名はありません。

1. Installer／OS のパッケージ管理から配置し、Xpotato Tunnel を開く。
2. 初めて使う場合は「初期設定・設定変更」で許可範囲と保存内容を確認し、同意後に API キーを非表示で入力する。
3. 既存環境の更新は、同じプロファイル保存先・端末名で「この版で更新」を選ぶ。
4. 「接続を開始」で起動し、停止は「このアプリの接続を停止」。他の既存接続は、その開始元で利用者自身が停止する。

Go/Rust/Node/npm の事前導入は不要です。初回設定・runtime 再取得には通信が必要で、完全オフライン同梱ではありません。通常の導入・更新・修復・起動では、展開済み runtime の全件再読を必須にしません。取得時 SHA/SRI・安全な展開・限定した起動入口と構成の確認を維持し、重い全依存診断は明示操作だけです。

アプリ本体の入れ替えは次版の Installer／deb、runtime の適用は「この版で更新」で行います。バックグラウンドの自動更新・既存プロセスの強制停止・OS権限やサービス設定の変更は行いません。削除後もプロファイル・保存済みキー・runtime は保持します。

[対応ソース](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.12/xpotato-tunnel-0.2.12-source.tar.gz) は開発者向けです。[source の対象と再構築](docs/source-distribution.md)、[checksum](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.12/SHA256SUMS.txt) と [公開物の出自](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.12/release-provenance.json) を別途提供します。GitHub の自動生成 Source code archive は依存を含む対応ソースの代わりではありません。

[v0.2.10 portable 手順](docs/quickstart.md) は同版の履歴として保持します。旧版のtag/assetは上書きしません。旧v0.2.8のログ設定エラーは [運用ガイド](docs/operations.md#v028のログ設定エラー) を参照してください。

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

CIは各OSの検証と候補作成です。Desktop GUI candidates は PR または手動実行で同じ pipeline を使い、Windows/Linux/source の照合済み bundle を作ります。公開は別の明示操作で、push/PR/手動buildからの自動Release・本人端末への導入は行いません。
