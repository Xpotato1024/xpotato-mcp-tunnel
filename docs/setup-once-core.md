# Go コア: Setup と起動

0.2.0 の入口は `cmd/xpotato-tunnel` です。PS/cmd/sh は保存済みの Go 実行ファイルへ引数を渡すだけで、キー保存・設定保持・導入・起動は Go 側が担当します。Desktop Commander の実行だけに固定版 Node を使用します。旧 Node CLI と 30 テストは互換性の証拠として残しています。

## 本人による導入

レビュー済みのソースを Go 1.27.2 でビルドした実行ファイルを使います。この PR は配布リリースや自動インストーラーを公開しません。通常ユーザーのターミナルで実行してください。

```powershell
# Windows: C:\Work は事前に本人が作成した、許可するディレクトリ。
.\xpotato-tunnel.exe setup --device llm-01 --tunnel-id tunnel_YOUR_ID --allow 'C:\Work'

# Setup が生成する cmd の絶対パスで、一コマンド起動。
& "$HOME\AppData\Local\xpotato-mcp-tunnel-v2\bin\xpotato-tunnel.cmd"
```

```sh
# Linux: ログインユーザーの Secret Service が起動・解除されていること。
./xpotato-tunnel setup --device llm-02 --tunnel-id tunnel_YOUR_ID --allow "$HOME/Work"
"$HOME/.local/share/xpotato-mcp-tunnel-v2/bin/xpotato-tunnel"
```

Linux で絶対パスの `XDG_DATA_HOME` を設定している場合、保存先はその配下です。両 OS とも `--home ABSOLUTE_DIR` で保存先を指定できます。PATH、PowerShell 実行ポリシー、シェル設定、サービスは変更しません。既存の稼働接続を切り替える操作は本人が行います。

引数を省略して `setup` を実行すると端末名、Tunnel ID、許可ディレクトリを尋ねます。複数ルートは `--allow` を繰り返します。保存同意で `yes` を入力した後に、runtime API キーを非表示入力します。コマンド引数や JSON にキーを入れません。保存先はこのアプリ・所有者・保存場所・端末・Tunnel に対応する一つのレコードです。エージェントによる実際の保存や取得は実施していません。

## OS ごとの資格情報

| OS | バックエンド | 起動条件 |
| --- | --- | --- |
| Windows amd64 | Windows Credential Manager のユーザー Generic credential、logon 間で保持 | Setup を実行した通常ユーザー。管理者トークンを拒否 |
| Linux amd64 | ユーザー Unix session D-Bus 上の Secret Service、default collection | 同じ UID、非 root、既存 collection が解除済み |

Linux ではキーリングの作成、サービスの自動起動、Unlock/Prompt 呼び出しを行いません。未導入、セッションなし、ロック中、レコードなしを区別して停止し、平文ファイルにフォールバックしません。鍵の転送は Secret Service の `plain` session をユーザー Unix IPC 上で使います。headless 環境で無条件に無人起動できる設計ではありません。Windows の本物の Vault と Linux デスクトップの本物のキーリングは、本人の受け入れ確認が必要です。

Start は入力を要求しません。先に本人・非昇格・許可範囲・runtime 整合性を確認し、二重起動ロックを取ってからキーを取得します。キーは Tunnel 子プロセスの環境へ渡し、Desktop Commander の import 前に取り除きます。Go の byte buffer は消去しますが、OS/Go の内部コピーや同じユーザー権限からの完全な秘匿は保証しません。

## 再開・更新・確認

Setup は途中状態を保持します。保存を断ったり、ネットワーク／Vault で失敗した場合、同じ端末名で再実行します。完了済みのキー保存は繰り返しません。保存と状態書き込みの間で失敗した場合は、次回もう一度同じレコードへの保存同意が必要です。既存 Commander 設定はバイト単位で保持し、許可ルートの縮小は認め、拡大は新しい端末の Setup を要求します。旧 schema 2 や孤立 profile を自動移行しません。

| コマンド | 動作 |
| --- | --- |
| 引数なし／`start` | default 端末を保存済みキーでフォアグラウンド起動 |
| `doctor` | runtime／設定と lock の存在を確認。Vault、実接続、アカウント登録を検査しない |
| `credentials` | この端末の一レコードを、本人の新しい保存同意と非表示入力で置換 |
| `update` | コアに組み込んだ固定 runtime を導入し、次回起動用の参照を更新 |
| `rollback` | 保存済みの前 runtime へ参照を戻す。現コアが認識する版に限る |
| `plugin` | 登録用情報／指定済み直接 MCP endpoint の bundle を再生成 |

今回の固定版は Go コア 0.2.0、Node 24.14.0、Tunnel runtime 0.0.16、Desktop Commander 0.2.52 です。導入済みの Start は npm/Git／パッケージ取得を行いません。各 runtime は別ディレクトリへ導入し、失敗時は既存参照を保持、旧版は削除しません。現行 0.2.0 に別 runtime 版は同梱していないため、初回 `rollback` には前版がありません。コア更新の自動取得、コア自体の自動 rollback は未実装です。新しいコアの Setup が同版の異なる実行ファイルを検出した場合、上書きせず停止します。

二重起動は同じ保存先の端末単位と Tunnel ID 単位で防止します。別保存先、旧起動スクリプト、別ホストのプロセスは検知できません。異常終了の lock は自動削除や他プロセス停止をせず、本人による確認が必要です。

## プラグイン登録の残り

Setup は `devices/<device>/registration/tunnel-registration.json` に Tunnel ID と公式プラグイン画面への案内を生成します。アカウント上の登録状態は **account-registration-pending** と記録します。公式手順は、Tunnel を起動して ChatGPT Plugins で Add custom MCP server → Tunnel → ID の選択 → 認証・権限の確認 → Create as a plugin です。コードからの account 登録 API は確認できておらず、Setup だけでアカウント登録完了とは言いません。

本人が別途検証した直接 HTTPS MCP endpoint を `--mcp-url` に指定した場合だけ、AgentPlugins 1.0 の `plugin.json`／`mcp.json` を一つの plugin root に入れた ZIP も生成します。Tunnel ID から URL を推測しません。キー、userinfo、query を含む URL は拒否します。bundle 生成はアップロードや有効化を意味しません。

この制約により「Setup だけでプラグインまで自動作成」はまだ完全には満たしていません。実装済みなのはローカル Setup、保存済みキーによる一コマンド起動、登録情報／任意の直接接続 bundle 生成です。

## 開発検証

```text
go test -count=1 ./...
go vet ./...
go build -trimpath ./cmd/xpotato-tunnel
npm run check
npm test
```

Go の通常テストは fake Vault、合成キー、一時 profile を使用します。Linux CI は専用の一時 D-Bus daemon と fake Secret Service を使い、本物の desktop bus を参照しません。`XPOTATO_RUNTIME_SMOKE=1 go test -count=1 -run TestManagedInstallSmoke -v ./internal/managed` は公式依存を一時領域へ導入し、隔離 stdio の initialize/tools-list を検証します。キー入力、Tunnel control-plane 接続、実 Vault 操作はありません。

公式資料: [Secure MCP Tunnel](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels)、[Windows CredWriteW](https://learn.microsoft.com/en-us/windows/win32/api/wincred/nf-wincred-credwritew)、[Secret Service](https://specifications.freedesktop.org/secret-service/latest/)。
