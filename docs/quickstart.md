# v0.2.10 portable の初回利用・更新（履歴）

> v0.2.11 の通常導入・更新は [共通 GUI の利用手順](gui-lifecycle.md) を参照してください。以下は公開済み v0.2.10 portable 専用の固定手順です。新しい Installer／deb の手順や現在の通常 runtime 検査へ読み替えないでください。

v0.2.10は更新時の完全検証の重複を減らし、core/runtimeの切替と取消・再開を改善しています。通常Startの完全検証は維持しています。v0.2.9のログ設定修正も含みます。旧v0.2.8からも、新exeの`update`で既存設定・保存済みキー・Tunnel IDを維持して更新できます。[旧版の障害と一時的な起動方法](operations.md#v028のログ設定エラー)も参照できます。

対応はWindows/Linux amd64です。通常ユーザーの対話端末で実行します。管理者/rootでの起動、OS権限やPATHの変更、自動常駐は行いません。端末からOpenAIへの外向きHTTPSと、本人のOS資格情報保管庫が必要です。

## 入手先と必要なもの

WindowsのLaptopには **Windows amd64版の`xpotato-tunnel.exe`と同じ配布物のガイド** を取得します。Linux用、GitHubの「Source code」、旧npm/npx CLIは選びません。Go/Node/npmを別途入れる必要はありません。ARM64は対象外です。

`setup`/`update`はNode 24.22.1、Commander 0.3.0、Tunnel 0.0.16と固定lockの依存を取得します。初回はnodejs.org、GitHub Releasesとその配信先、registry.npmjs.orgへのHTTPS接続が必要です。完全オフライン用パッケージではありません。起動時にもOpenAIへの外向きHTTPSが必要です。

1. [固定Release v0.2.10](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/tag/v0.2.10)を開きます。公開後はGitHubログイン不要です。
2. WindowsはRelease本文の検証ブロック一回で、正しいZIPの取得・digest照合・展開まで行います。下の説明を確認してください。
3. Linuxは[tar.gz](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.10/xpotato-tunnel-0.2.10-linux-amd64.tar.gz)と[.sha256](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.10/xpotato-tunnel-0.2.10-linux-amd64.tar.gz.sha256)を保存し、下のLinux手順で照合します。「Source code」は実行用配布物ではありません。

ReleaseにはActions artifactの14日期限はありません。[Actions](https://github.com/Xpotato1024/xpotato-mcp-tunnel/actions/workflows/windows-tests.yml)は開発・レビュー用です。CI再ビルドは同じ版でもbytesが変わり得るため、正式Releaseの代わりに同版artifactへupdateしないでください。取得できない場合は管理者へ確認します。

## Windowsで取得・検証する

**[固定Release v0.2.10](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/tag/v0.2.10) の「Windows ZIP の検証」ブロック全体を、通常ユーザーのPowerShellへ一回貼り付けます。** このブロックがWindows amd64 ZIPを一つ取得し、検証後に `$HOME\Apps\xpotato-tunnel-0.2.10` へ展開します。SHAファイルを別途ダウンロードする操作は不要です。PowerShell 5.1/7を使えます。Go/Node/npm/gh・GitHubトークン・管理者権限は不要です。

公開リポジトリのAPIから固定tagの実commit、Release ID、asset ID・名前・状態・サイズ・GitHub側 `sha256:` digestを取得し、公開時に固定した値と厳密に照合します。digestが取得できない・一致しない場合は停止し、SHA検証を省略する経路へ切り替えません。Release本文には管理側が実ZIPとAPI digestを照合して生成した値が記載されています。`latest` や可変mainのスクリプトは使いません。

外側ZIPのSHA照合が成功してから、その同じbytes内の `verify-windows.ps1` を読み込んで実行します。安全なパス・重複/大小文字衝突・内部全ファイルのSHA・一覧の欠落/追加・`build-info.json` のversion/commit/platform/dirtyを検査します。exeは自動実行しません。版確認はmetadataと検証済みexeのSHAに基づきます。新規の一時フォルダーを検証してから最終先へ移動し、既存先がある場合は全検証だけを行います。上書き・削除はしません。

`Verified` が出た場合だけ検証完了です。通信失敗、部分取得、SHA不一致、Ctrl+C取消後はエラーを確認して**ブロック全体**を再実行します。取得用の一時ファイルと自分が作成した展開stageだけを片付けます。以前の展開先・設定・鍵には触れません。既存先の内容が異なる場合は止まり、別の未使用展開先を最後の `-DestinationPath` 引数へ指定できます。

ブラウザーで先に[Windows ZIP一つ](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.10/xpotato-tunnel-0.2.10-windows-amd64.zip)を保存することもできます。この場合はReleaseブロック末尾へ `-ArchivePath "$HOME\Downloads\xpotato-tunnel-0.2.10-windows-amd64.zip"` を追加します。名前の `(1)` 付加・途中ダウンロードを避けます。この経路でもAPIによる固定版とdigestの確認を行います。

checksumは破損検出であり、コード署名やWindowsの安全性判定を代替しません。検出・隔離された場合は管理者へ検出内容を確認し、除外・保護無効化・隔離解除で回避しません。検証ブロックはsetup/update/startを実行しません。今回の公開作業にも実端末への導入は含みません。

### 旧包装のartifact

旧0.2.7 main `0fc6ceb301f1a4d5da95e377af9309e3ab89d584`は[成功run 38204606931](https://github.com/Xpotato1024/xpotato-mcp-tunnel/actions/runs/38204606931)にありました。有効期限は2026-10-26 04:45 UTCです。新規導入は上記v0.2.10 Releaseを使い、この節は旧包装を受領済みの人向けです。

この旧ZIPだけは中に`xpotato-tunnel-windows-amd64.tar.gz`があります。旧包装を調べる場合は管理者と`SHA256SUMS`、version `0.2.7`、runのcommitと`build-info.txt`を確認してください。現在のRelease検証ブロックは旧包装には使えません。

## Linuxで展開・確認する

Releaseのtar.gzと.sha256を同じフォルダーへ保存し、そこで確認します。外側のZIPはありません。

```sh
set -e
sha256sum -c xpotato-tunnel-0.2.10-linux-amd64.tar.gz.sha256
mkdir package
tar -xzf xpotato-tunnel-0.2.10-linux-amd64.tar.gz -C package
cd package
sha256sum -c SHA256SUMS && ./xpotato-tunnel version
cat build-info.json # Release本文のcommit・platformと照合
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

## クレジット・ライセンス・対応ソース

Xpotato.net / Xpotato1024 のプロジェクトです。`xpotato-tunnel about`、`license`、`source`
で帰属表示・AGPL-3.0-only 全文・利用中の core の対応ソース情報を確認できます。
第三者依存は元の条件を保持します。同じ Release の source.tar.gz が対応ソースです。
改変・再配布・ネットワーク提供時の案内は [licensing.md](licensing.md) を参照してください。