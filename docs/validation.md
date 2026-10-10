# 検証メモ

## 自動・隔離検証

`npm test` は Node の標準 test runner を使用します。ネットワークからの取得は mock、キーは合成値、設定・state はテスト専用の一時ディレクトリです。本人の認証ファイルや既存 Tunnel を使いません。

検証する内容:

- setup の再実行で設定の bytes、本人の編集、custom field を保持
- 許可ルートの空・不存在を拒否、端末間の config 分離
- update の次回起動への適用、旧 runtime/実行中 lock の保持、失敗時の selection 保持
- doctor の読み取り確認、runtime 改変の検出をキー入力前に実施
- 空白付き Windows executable と一つの `--mcp.command` 引数を実際の Node 子プロセスで round-trip
- 子プロセス出力の継承、終了コードの伝達
- profile 分離とキー除去を実際の stdio wrapper 子プロセスで確認
- 多重起動の拒否、終了・失敗・中止後の自分の lock の片付け
- 手動再起動時のキー再入力、config 保持と更新版の選択
- マスク入力、backspace、raw mode の復旧
- `EXDEV` で元ファイル・保存済み設定を保持し、copy/delete に切り替えない

`npm run check` は CLI と各 `.mjs` の構文を確認します。このテストで外部の control plane 認証や本番 Tunnel 接続が成功したとは判定しません。

実インストールの確認は opt-in です。本人が取得した公式 v0.0.16 runtime ZIP のパスを渡します。SHA256 を照合して一時ディレクトリへ展開し、固定版 npm の取得、runtime の version、Commander の initialize/tools/list を確認します。キーと control plane は使いません。初回の stdio 起動は環境によって 20 秒以上かかるため、60 秒でタイムアウトします。

```powershell
npm run smoke:install -- 'C:\Downloads\tunnel-client-runtime-v0.0.16-windows-amd64.zip'
```

## Windows 実機で確認した結果 (2026-10-10)

Node.js v24.14.0 で隔離テスト 18 件が成功し、構文確認・diff check・14 ファイルの UTF-8 検査に問題はありませんでした。承認済み ZIP を使った実インストール smoke は 30,056 ms で成功し、runtime v0.0.16 と Commander 0.2.52 の initialize/tools/list から 26 ツールを確認しました。profile は一時ディレクトリ、API キーは未使用、control plane は未起動です。初回 20 秒の smoke はタイムアウトし、確認時間を延ばして成功を確認しました。

## 本人の受け入れ確認

Draft PR のレビュー後、固定 commit の checkout または private Git の認証済み npx から確認してください。

```powershell
# checkout で確認する場合
node .\bin\cli.mjs --help
node .\bin\cli.mjs setup --device acceptance --home 'C:\Work\Tunnel acceptance' --tunnel-id tunnel_YOUR_TEST_ID --allow 'C:\Work\Test files'
node .\bin\cli.mjs doctor --device acceptance --home 'C:\Work\Tunnel acceptance'
node .\bin\cli.mjs start --device acceptance --home 'C:\Work\Tunnel acceptance'
```

保存先と許可ディレクトリは本人が先に作成し、既存接続と別のテスト Tunnel ID を選んでください。`start` で本人がキーをマスク入力した後、アプリで接続とツール一覧を確認します。同じ Tunnel ID へ切り替える場合は、既存 launcher の停止時刻を本人が選んでください。

専用テストファイルで write/read/edit/read、長い UTF-8 ファイルの offset 読み取り、content search、同一ドライブ move を確認します。別ドライブ move は失敗し、元が残ることを確認します。コマンドは専用の短時間プロセスで出力読み取りと自然終了を確認してください。業務ファイルや資格情報はテストに使いません。

さらに、config の非秘密項目を一つ本人が編集し、setup 再実行・update・手動再起動の後も値が保存されることを確認します。キーは再入力され、保存済みのキーが自動使用されないことを確認します。

実接続、本人の API キー入力、新 CLI への本番切り替えはこの実装作業では実行しません。既存 launcher での接続成功や受け入れ結果を、新 CLI での実接続成功とは扱いません。
