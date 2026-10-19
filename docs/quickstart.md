# 導入・更新の入口

対応はWindows/Linux amd64です。通常ユーザーの対話端末で実行します。管理者/rootでの起動、OS権限やPATHの変更、自動常駐は行いません。端末からOpenAIへの外向きHTTPSと、本人のOS資格情報保管庫が必要です。

## 入手先と必要なもの

WindowsのLaptopには **Windows amd64版の`xpotato-tunnel.exe`と同じ配布物のガイド** を取得します。Linux用、GitHubの「Source code」、旧npm/npx CLIは選びません。Go/Node/npmを別途入れる必要はありません。ARM64は対象外です。

`setup`/`update`はNode 24.14.0、Commander 0.2.52、Tunnel 0.0.16と固定lockの依存を取得します。初回はnodejs.org、GitHub Releasesとその配信先、registry.npmjs.orgへのHTTPS接続が必要です。完全オフライン用パッケージではありません。起動時にもOpenAIへの外向きHTTPSが必要です。

1. [Releases](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases)に採用済みの版があれば、その版の`xpotato-tunnel-<version>-windows-amd64-<commit>.zip`を取得します。2026-10-19時点ではRelease/tagは未作成です。
2. 現在はGitHubにログインして[Cross-platform checks](https://github.com/Xpotato1024/xpotato-mcp-tunnel/actions/workflows/windows-tests.yml)を開き、branchが`main`、結果が成功のrunを選びます。runのcommitと、管理者が採用したcommitが一致することを確認します。PRのrunはレビュー専用です。
3. run下部のArtifactsから`xpotato-tunnel-…-windows-amd64-<commit>`をダウンロードします。Linuxなら`linux-amd64`です。versionとcommitを含む新しい名前は本修正の採用後のrunからです。旧runの名前はOSとcommitのみです。

artifactは14日で失効します。ない場合は別OSや別PRのartifactで代用せず、管理者に同じ採用commitの再ビルドか版付きReleaseを依頼してください。GitHubログインできない場合も管理者へ確認します。

## Windowsで展開・確認する

新しいWindows artifactのZIPは一度展開するとexeとガイドが直接出てきます。空のユーザーフォルダーへ展開し、そのフォルダーをPowerShellで開きます。Release用ZIPも同じ構成です。配布フォルダーと設定・キーの恒久保存先は別です。

```powershell
# 例: ダウンロードしたZIPのパスと、新しく作る保存先を指定
Expand-Archive -LiteralPath "$HOME\Downloads\取得したファイル名.zip" -DestinationPath "$HOME\Apps\xpotato-tunnel-取得した版"
Set-Location "$HOME\Apps\xpotato-tunnel-取得した版"

# checksumが一つでも違えば停止。setup前に行う
$ErrorActionPreference = 'Stop'
foreach ($line in Get-Content -LiteralPath .\SHA256SUMS) {
    if ($line -notmatch '^([0-9a-fA-F]{64}) [ *](.+)$') { throw 'Invalid SHA256SUMS' }
    $expected = $Matches[1]
    $file = $Matches[2]
    if ((Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash -ne $expected) {
        throw "SHA256 mismatch: $file"
    }
}
.\xpotato-tunnel.exe version
if ($LASTEXITCODE -ne 0) { throw 'version command failed' }
```

採用済み旧mainの表示は`0.2.7`、このPRのレビュー候補は`0.2.8`です。新包装では`build-info.json`の`version`、`commit`、`platform`が選んだrunと一致し、`dirty`が`false`であることも確認します。checksumは破損の検出であり、同梱checksumだけで発行元を証明するものではありません。管理者が案内したリポジトリ・runから取得してください。

```powershell
Get-Content -LiteralPath .\build-info.json
```

Windowsがexeの発行元を未確認と表示した場合、取得元とchecksumを確認して管理者へ相談してください。保護機能や実行ポリシーを変更する必要はありません。

### 旧包装のartifact

現在の採用済みmain `0fc6ceb301f1a4d5da95e377af9309e3ab89d584`は[成功run 38204606931](https://github.com/Xpotato1024/xpotato-mcp-tunnel/actions/runs/38204606931)の`xpotato-tunnel-windows-amd64-0fc6ceb301f1a4d5da95e377af9309e3ab89d584`です。artifact IDは`11726941695`、有効期限は2026-10-26 04:45 UTCです。期限後はこのリンクから取得できません。

この旧ZIPだけは中に`xpotato-tunnel-windows-amd64.tar.gz`があります。ZIPを空フォルダーへ展開した後、そこで次を実行し、上のchecksum/version確認へ進みます。旧包装には`build-info.json`はなく、runのcommitと`build-info.txt`で確認します。

```powershell
tar -xzf .\xpotato-tunnel-windows-amd64.tar.gz
if ($LASTEXITCODE -ne 0) { throw 'tar extraction failed' }
```

## Linuxで展開・確認する

Linux artifactの外側ZIPを展開してから、中の版・commit付きtar.gzを新しい空フォルダーへ展開します。Releaseではtar.gzを直接取得します。旧包装の名前は`xpotato-tunnel-linux-amd64.tar.gz`です。

```sh
mkdir package
tar -xzf 取得したファイル名.tar.gz -C package
cd package
sha256sum -c SHA256SUMS && ./xpotato-tunnel version
cat build-info.json # 新包装のみ。選んだrunのcommit・platformと照合
```

checksum不一致やversion不一致ならsetupせず、再取得か管理者への確認で解決します。tarにはLinuxの実行権限を保持します。

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
