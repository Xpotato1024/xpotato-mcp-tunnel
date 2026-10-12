# 導入・更新の入口

対応はWindows/Linux amd64です。通常ユーザーの対話端末で実行します。管理者/rootでの起動、OS権限やPATHの変更、自動常駐は行いません。端末からOpenAIへの外向きHTTPSと、本人のOS資格情報保管庫が必要です。

## 配布物を確認する

レビュー・採用済みheadに対応するprivate CI artifactを取得します。ZIPの中のOS別tar.gzを展開し、SHA256SUMSとversionを確認してください。配布exeのフォルダーと設定・キーの恒久保存先は別です。Go/Node/npmの個別導入は不要です。

Windowsの配布フォルダー内で:

```powershell
tar -xzf .\xpotato-tunnel-windows-amd64.tar.gz
Get-FileHash .\xpotato-tunnel.exe -Algorithm SHA256
Get-Content .\SHA256SUMS
.\xpotato-tunnel.exe version
```

Linuxの配布フォルダー内で:

```sh
tar -xzf xpotato-tunnel-linux-amd64.tar.gz
sha256sum -c SHA256SUMS
./xpotato-tunnel version
```

WindowsのSHA256SUMSの`*`はbinary指定です。hashを比較してください。Linuxの実行権限はtarに保持されています。

## 初めて使う

Windowsは`.\xpotato-tunnel.exe setup`、Linuxは`./xpotato-tunnel setup`を実行します。

1. 端末名、Tunnel ID、アクセスを承認するフォルダーを確認。
2. 恒久保存先と短い起動コマンドの保存先を確認。
3. 最終内容とOS保管庫への保存に同意し、Runtime APIキーを非表示入力。

**Tunnel IDは接続の識別子、Runtime APIキーは秘密の資格情報です。** IDをキー欄へ入力しないでください。キーをコマンド/.env/JSONへ書きません。フォルダー承認は端末の操作範囲として保存され、暗黙には拡張しません。必要な範囲だけを承認します。

許可フォルダーはCommanderのファイルツールの確認範囲です。プロセス実行全体を閉じ込めるsandboxではないため、広いルートを承認する前に範囲を確認してください。

| OS | 既定の恒久保存先 | キーの保存 |
|---|---|---|
| Windows | ユーザーのAppData/Local/xpotato-mcp-tunnel-v2 | 同じ通常ユーザーのWindows Credential Manager |
| Linux | XDG_DATA_HOME/xpotato-mcp-tunnel-v2、未指定なら~/.local/share/xpotato-mcp-tunnel-v2 | 解除済みユーザーSecret Service |

Linuxで保管庫が未導入・ロック中・desktop sessionなしの場合は停止します。平文への代替保存や自動Unlockはしません。保存先を自分で指定した場合は以後も同じ`--home`を使います。

完了時に表示される短い起動コマンドを使います。登録済みの同じTunnel IDのプラグインがあれば再利用します。新規の場合だけ本人が[ChatGPT PluginsのTunnel登録](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels#connect-from-chatgpt)で同じIDを選び、認証・権限を確認します。コアはアカウント登録を自動作成・判定しません。

## 既に使っている環境を更新する

**新しく取得したexeから**Windowsは`.\xpotato-tunnel.exe update`、Linuxは`./xpotato-tunnel update`を実行します。古い短縮コマンドのupdateはその古いコア版を使います。自分で設定したhomeの場合だけ同じ`--home`を指定します。

同じdefault device・キー・許可範囲を使い、Setupやキー入力・別device・プラグイン再登録は通常不要です。新runtime/coreと承認済みラッパーを準備し、最後に次回起動用参照を更新します。旧版と設定は保持します。0.2.7からupdateの長い工程もstderrに進捗を表示します。

```text
Pinned runtime ready; applies at next start. Existing process was not restarted.
```

これは保存済み更新の完了です。旧接続は再起動されません。本人が元の端末でCtrl+Cし、子プロセス終了を待ってから短いコマンドを実行します。保存版は短いコマンドの`version`で確認できます。更新失敗時は起動せず、同じ新exeのupdateを再実行します。

## 起動と停止

引数なしまたは`start`でフォアグラウンド起動します。保存済みキーを使い、起動ごとの再入力はありません。通常は接続変化とMCP処理、警告・エラーを表示します。接続成功とMCP操作成功は別の確認です。正常な要求待ちでは定例通知を出しません。

停止は本人が元の起動端末でCtrl+Cし、終了を待ちます。同じTunnel IDのstdio接続は端末をまたいでも同時に1台だけ。device名を変えるだけでは分離できません。2台同時利用には別Tunnel IDと対応登録を使います。スリープ中は応答できず、復帰・回線切替後の回復は通常の再試行に依存します。

[困ったとき](operations.md)、[Setupの仕様](setup-once-core.md)、[.env Setup](setup-env.md)も参照してください。
