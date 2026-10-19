# 導入・更新の入口

正式版v0.2.9は通常起動のログ設定エラーを修正しています。旧v0.2.8からも、新exeの`update`で既存設定・保存済みキー・Tunnel IDを維持して更新できます。[旧版の障害と一時的な起動方法](operations.md#v028のログ設定エラー)も参照できます。

対応はWindows/Linux amd64です。通常ユーザーの対話端末で実行します。管理者/rootでの起動、OS権限やPATHの変更、自動常駐は行いません。端末からOpenAIへの外向きHTTPSと、本人のOS資格情報保管庫が必要です。

## 入手先と必要なもの

WindowsのLaptopには **Windows amd64版の`xpotato-tunnel.exe`と同じ配布物のガイド** を取得します。Linux用、GitHubの「Source code」、旧npm/npx CLIは選びません。Go/Node/npmを別途入れる必要はありません。ARM64は対象外です。

`setup`/`update`はNode 24.14.0、Commander 0.2.52、Tunnel 0.0.16と固定lockの依存を取得します。初回はnodejs.org、GitHub Releasesとその配信先、registry.npmjs.orgへのHTTPS接続が必要です。完全オフライン用パッケージではありません。起動時にもOpenAIへの外向きHTTPSが必要です。

1. GitHubにログインして[正式Release v0.2.9](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/tag/v0.2.9)を開きます。privateリポジトリへのアクセス権が必要です。
2. Windowsは[ZIP](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.9/xpotato-tunnel-0.2.9-windows-amd64.zip)と[同名の.sha256](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.9/xpotato-tunnel-0.2.9-windows-amd64.zip.sha256)を同じDownloadsフォルダーへ保存します。Linuxは[tar.gz](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.9/xpotato-tunnel-0.2.9-linux-amd64.tar.gz)と[.sha256](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/download/v0.2.9/xpotato-tunnel-0.2.9-linux-amd64.tar.gz.sha256)です。
3. 下記の外側archiveと内部ファイルのchecksum、version、`build-info.json`のcommitをRelease本文のsource commitと照合します。「Source code」のZIP/tar.gzは実行用配布物ではありません。

ReleaseにはActions artifactの14日期限はありません。[Actions](https://github.com/Xpotato1024/xpotato-mcp-tunnel/actions/workflows/windows-tests.yml)は開発・レビュー用です。CI再ビルドは同じ版でもbytesが変わり得るため、正式Releaseの代わりに同版artifactへupdateしないでください。取得できない場合は管理者へ確認します。

## Windowsで展開・確認する

ReleaseのWindows ZIPは一度展開するとexeとガイドが直接出てきます。配布フォルダーと設定・キーの恒久保存先は別です。

先にDownloadsに次の**2ファイルが両方あること**をエクスプローラーで確認します。拡張子も表示し、`.sha256.txt`、`(1)`付き、途中までのダウンロードではないことを確認してください。足りない場合は上のReleaseからそのファイルを保存します。

- `xpotato-tunnel-0.2.9-windows-amd64.zip`
- `xpotato-tunnel-0.2.9-windows-amd64.zip.sha256`

次のブロックを`& {`から最後の引数行まで**まとめて**PowerShellへ貼り付けます。ブロック内の行を抜き出して個別実行しないでください。入力・外側checksumが不正なら展開せず停止し、内部checksumやversionに失敗しても後続の処理へ進みません。`検証完了`と表示された場合だけ、次のsetup/updateへ進みます。

```powershell
& {
    param([string] $ArchivePath, [string] $DestinationPath, [switch] $VerifyExisting)
    $ErrorActionPreference = 'Stop'
    $archiveName = 'xpotato-tunnel-0.2.9-windows-amd64.zip'
    if ([IO.Path]::GetFileName($ArchivePath) -ne $archiveName) {
        throw "Download name must be $archiveName (no (1) suffix)"
    }
    foreach ($inputFile in @($ArchivePath, "$ArchivePath.sha256")) {
        if (-not (Test-Path -LiteralPath $inputFile -PathType Leaf)) {
            throw "Required download missing: $inputFile (check the full extension)"
        }
    }
    $checksumText = [string] (Get-Content -LiteralPath "$ArchivePath.sha256" -Raw)
    $checksum = [regex]::Match($checksumText.Trim(), '^([0-9a-fA-F]{64}) [ *]([^\r\n]+)$')
    if (-not $checksum.Success -or $checksum.Groups[2].Value -ne $archiveName) {
        throw 'Invalid archive checksum file or filename'
    }
    if ((Get-FileHash -LiteralPath $ArchivePath -Algorithm SHA256).Hash -ne $checksum.Groups[1].Value) {
        throw 'Archive SHA256 mismatch; do not extract or run setup'
    }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::OpenRead($ArchivePath)
    try {
        $manifest = $zip.GetEntry('SHA256SUMS')
        if ($null -eq $manifest) { throw 'ZIP checksum manifest missing' }
        $reader = [IO.StreamReader]::new($manifest.Open())
        try { $expectedManifest = $reader.ReadToEnd() } finally { $reader.Dispose() }
    } finally { $zip.Dispose() }
    if (Test-Path -LiteralPath $DestinationPath) {
        if (-not $VerifyExisting -or -not (Test-Path -LiteralPath $DestinationPath -PathType Container)) {
            throw 'Destination exists; keep it. Use another new path, or -VerifyExisting to check without extracting'
        }
    } elseif ($VerifyExisting) {
        throw 'Existing destination not found; nothing was extracted'
    } else {
        Expand-Archive -LiteralPath $ArchivePath -DestinationPath $DestinationPath
    }
    $actualManifest = [IO.File]::ReadAllText((Join-Path $DestinationPath 'SHA256SUMS'))
    if ($actualManifest -cne $expectedManifest) { throw 'Checksum manifest differs from verified ZIP; existing files preserved' }
    $lines = @($expectedManifest -split '\r?\n' | Where-Object { $_.Length -gt 0 })
    if ($lines.Count -eq 0) { throw 'Empty SHA256SUMS' }
    $binaryChecked = $false
    foreach ($line in $lines) {
        $entry = [regex]::Match($line, '^([0-9a-fA-F]{64}) [ *]([a-zA-Z0-9._/-]+)$')
        if (-not $entry.Success) { throw 'Invalid SHA256SUMS' }
        $relative = $entry.Groups[2].Value
        if ([IO.Path]::IsPathRooted($relative) -or ($relative.Split('/') -contains '..')) {
            throw 'Invalid checksum path'
        }
        $file = Join-Path $DestinationPath $relative
        if ((Get-FileHash -LiteralPath $file -Algorithm SHA256).Hash -ne $entry.Groups[1].Value) {
            throw "SHA256 mismatch: $relative"
        }
        if ($relative -eq 'xpotato-tunnel.exe') { $binaryChecked = $true }
    }
    if (-not $binaryChecked) { throw 'Binary checksum missing' }
    $version = & (Join-Path $DestinationPath 'xpotato-tunnel.exe') version
    if ($LASTEXITCODE -ne 0 -or $version -ne '0.2.9') { throw 'version mismatch' }
    Get-Content -LiteralPath (Join-Path $DestinationPath 'build-info.json')
    Set-Location -LiteralPath $DestinationPath
    Write-Host "検証完了: $DestinationPath (version $version)"
} -ArchivePath "$HOME\Downloads\xpotato-tunnel-0.2.9-windows-amd64.zip" -DestinationPath "$HOME\Apps\xpotato-tunnel-0.2.9"
```

既に展開済みなら再展開は不要です。ZIPと`.zip.sha256`を揃え、同じブロックの最後の引数行に`-VerifyExisting`を追加すると、外側archive、検証済みZIP内の一覧と既存`SHA256SUMS`の一致、内部ファイル・versionだけを確認します。既存のファイルは上書き・削除しません。新規展開を選ぶ場合は別の未使用の`-DestinationPath`を指定してください。

途中でエラーになったらsetup/updateへ進まず、不足ファイル・名前・checksumの原因を解決して**ブロック全体**を再実行します。展開後の検証で停止したフォルダーも自動削除しません。表示された`build-info.json`の`version`、`commit`、`platform`がRelease本文と一致し、`dirty`が`false`であることも確認します。checksumは破損の検出であり、同梱checksumだけで発行元を証明するものではありません。管理者が案内したリポジトリのReleaseから取得してください。

Windowsがexeの発行元を未確認と表示した場合、取得元とchecksumを確認して管理者へ相談してください。保護機能や実行ポリシーを変更する必要はありません。

### 旧包装のartifact

旧0.2.7 main `0fc6ceb301f1a4d5da95e377af9309e3ab89d584`は[成功run 38204606931](https://github.com/Xpotato1024/xpotato-mcp-tunnel/actions/runs/38204606931)にありました。有効期限は2026-10-26 04:45 UTCです。新規導入は上記v0.2.9 Releaseを使い、この節は旧包装を受領済みの人向けです。

この旧ZIPだけは中に`xpotato-tunnel-windows-amd64.tar.gz`があります。旧包装を調べる場合は管理者と`SHA256SUMS`、version `0.2.7`、runのcommitと`build-info.txt`を確認してください。上の0.2.9専用ブロックは旧包装には使えません。

## Linuxで展開・確認する

Releaseのtar.gzと.sha256を同じフォルダーへ保存し、そこで確認します。外側のZIPはありません。

```sh
set -e
sha256sum -c xpotato-tunnel-0.2.9-linux-amd64.tar.gz.sha256
mkdir package
tar -xzf xpotato-tunnel-0.2.9-linux-amd64.tar.gz -C package
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
