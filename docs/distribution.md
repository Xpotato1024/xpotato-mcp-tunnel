# 配布候補と版付きRelease

この手順は管理者向けです。利用者は[quickstart](quickstart.md)へ進みます。正式配布は[Release v0.2.10](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/tag/v0.2.10)です。CIは候補を作るだけで、push/PRからRelease、tag、deployを行いません。公開は本人の明示承認後に管理者が手動で行います。

v0.2.9は起動ログ設定を修正した新しい版です。生成名はコアの版から決まり、既存v0.2.8のtag/assetsは変更しません。以後の公開にも採用mainの再検証と、新版tag・Releaseへの本人の明示承認が必要です。

## 配布形式

| 用途 | Windows amd64 | Linux amd64 |
|---|---|---|
| Actionsレビュー | artifact ZIPを一度展開してexeとガイド | artifact ZIP内のtar.gzを展開 |
| 手動Release | `xpotato-tunnel-0.2.10-windows-amd64.zip` | `xpotato-tunnel-0.2.10-linux-amd64.tar.gz` |

内容はコア、quickstartと関連ガイド、非秘密の`setup.env.example`、全ファイルの`SHA256SUMS`、`build-info.json`、Goの`build-info.txt`です。Node/Commander/Tunnel実体や資格情報は含めません。初回setup/updateは固定runtimeを取得するため通信が必要です。runtimeの取得仕様は[setup-once-core](setup-once-core.md)を参照してください。

## ローカルで候補を作る（公開なし）

作業をcommitし、cleanなcheckoutで、対象OSのamd64用Go 1.27.2とPython 3.10以上を使います。利用者側にはどちらも不要です。`dist`はGit管理外です。実端末のsetup/start/updateは実行しません。

```text
go test -count=1 ./...
go vet ./...
python -m unittest discover -s test -p "test_*.py" -v
```

Windows:

```powershell
New-Item -ItemType Directory -Force dist
go build -trimpath -o dist/xpotato-tunnel.exe ./cmd/xpotato-tunnel
python scripts/package-review.py --binary dist/xpotato-tunnel.exe --output dist/package --platform windows-amd64 --release
```

Linux:

```sh
mkdir -p dist
go build -trimpath -o dist/xpotato-tunnel ./cmd/xpotato-tunnel
python3 scripts/package-review.py --binary dist/xpotato-tunnel --output dist/package --platform linux-amd64 --release
```

出力先は新規ディレクトリを指定します。既存候補の上書きは拒否します。組立時に版とGit commit、cleanなbuild情報を照合し、archiveを再展開して全checksum・versionとLinux実行権限を確認します。`--release`で版付きの固定asset名、指定しない場合はcommit付きレビュー名を使います。完全commitは両方とも`build-info.json`と`build-info.txt`に記録します。

archiveはファイル順、commit日時、権限、tar所有者、gzipヘッダーとbuild-info先頭の表示パスを固定します。CIは同じソースのnative buildを2回行ってbinaryを比較し、Release archiveも別出力先へ2回組み立ててbyte一致を確認します。異なるPython/zlibやGo toolchainまでのbyte一致を保証する署名・証明ではありません。再組立にはRelease本文のsource commit・CIと同じtoolchainを使い、既存Releaseのassetは上書きしません。

ソースから再ビルドする場合は`git clone --no-tags`した独立checkoutでRelease本文の完全commitをcheckoutします。CIと同じGo 1.27.2、`CGO_ENABLED=1`、`GOAMD64=v1`と`-trimpath`を使い、同梱build-infoも確認します。tagの有無でGoのmain-module版情報が変わり得るため、同じtreeだけでなくGit履歴・tag状態も揃えます。正式配布には検証済みCIの`release-candidate-...` artifact内のarchiveをそのまま使い、再ビルドで置き換えません。

ActionsのWindows artifactは`content`を直接ZIP化するため二重包装になりません。GitHubが作るZIPのbyte列はローカルRelease候補ZIPと異なるので、外側ZIPのhashを混同しないでください。artifactは14日期限です。長期配布には別途本人によるRelease判断が必要です。

## 手動Releaseの承認手順

以下は管理者が本人の明示承認を得てから行う手順です。PRのmergeだけで自動公開されることはありません。

1. 対象versionと採用済みmainの完全commit SHAを選びます。同じSHAでWindows/Linux両方のGo・Node・runtime smoke・package検証CIが成功し、独立レビューが済んでいることを確認します。
2. 両OSのcleanな同一SHAから上記候補を作り、`build-info.json`のversion/commit/platform/dirty、内部checksum、外側archiveの`.sha256`、展開後versionを確認します。利用者が受け取るZIP/tar.gzを実際に展開して確認します。
3. 既存tag/Releaseがないことを確認し、GitHubで採用SHAを指す版付きtagを指定して、まず**Draft Release**として保存します。既存tagや同名assetがあれば停止し、tag移動や上書きをしません。
4. 両OSのarchiveと各`.sha256`、対応SHA、版、runtimeは初回取得であること、更新手順、既知制約をDraftにそろえます。管理者は公開前に本人の承認範囲とasset・対象SHAを再確認します。
5. **本人が配布公開を明示承認した後だけ**Publishを行います。privateリポジトリのReleaseはアクセス権を持つ人向けで、一般公開にはなりません。公開後にREADMEの現在の入手先を更新します。

この承認は文書で示した運用上の停止点です。GitHub environment protectionが設定済みという意味ではありません。現状は自動公開機能自体を用意せず、意図しない公開を避けます。

## 固定版の取得・検証ブロックを作る

Windows ZIPには `verify-windows.ps1` を内部SHA対象として同梱します。外側ZIPのdigestを自身に入れると循環するため、外側digestはアップロード後のGitHub APIから取得しRelease本文へ固定します。公開後の利用者は本文のブロック一回で認証なしにZIPを取得し、固定tagの実commit・Release/asset ID・名前・URL・状態・サイズ・API digestと公開時の値を照合します。外側SHA成功後だけ同じbytesのhelperを実行し、exeは実行しません。

管理者はDraftへ両OSの候補をアップロードした後、既存の認証済みghとPython標準ライブラリで次を実行します（利用者にgh導入は不要）。`SOURCE_COMMIT` は採用mainの40桁SHAへ置き換えます。tagはannotatedの場合も実commitまで解決します。digestがnull・不一致・asset重複なら停止し、sidecarへfallbackしません。

```text
python scripts/release-guide.py --version 0.2.10 --commit SOURCE_COMMIT --archive dist/release/xpotato-tunnel-0.2.10-windows-amd64.zip --output windows-release-guide.md
```

出力は新規ファイル限定です。失敗や取消後は原因を確認して再実行します。Release本文へ生成ブロックとsource/CI/既知の制限・ライセンス案内を反映し、承認範囲と照合してPublishします。Publish後に本文を再読取し、匿名API・実ZIP・本文ブロックを隔離stagingでPowerShell 5.1/7検証します。自動公開workflowは追加しません。

ライセンスは独自コードのLICENSEと第三者のTHIRD-PARTY-NOTICES.txtを区別し、両方を新配布へ含めます。旧Releaseのarchive/tagは上書きせず、対象archive名・SHAを記載した付属NOTICEと本文への補足を追加します。Node/Commander/Tunnelは上流から別取得し、offline同梱やmirrorは行いません。

## AGPL-3.0-only の対応ソースと公開順序

新規配布には LICENSE、COPYRIGHT、CHANGES.md、THIRD-PARTY-NOTICES.txt、licensing.md を同梱します。
正式な条件・依存物の境界・改変版の表示は [licensing.md](licensing.md) を参照してください。
source bundle 作成は Python 3.12 以上を使用し、clean な配布対象 commit で
`python scripts/package-source.py --output dist/source` を実行し、
同じ commit の Go 依存ソースを vendor した `xpotato-tunnel-VERSION-source.tar.gz` と
その `.sha256` を同じ Release へ置きます。CI の corresponding-source artifact は候補であり、
その upload は Release 発行ではありません。source bundle と両 OS の配布物を照合してから
公開します。Node/Commander/Tunnel の取得済み runtime はこの source bundle に含めません。

private の Actions 枠を追加消費しない場合は、公開前監査・ローカル検証・独立レビューを
完了し、公開範囲の未解決事項について本人の判断を得てから、承認された当 repo のみ public
へ変更します。その後に標準 runner で PR の正確な head を検証し、成功後に merge します。
merge 後の配布対象 main SHA も検証し、その検証済み commit の候補から Release を作成します。
CI を無視した merge や、未実行のチェックを成功扱いする運用にはしません。

private 中にレビュー用修正を push する必要がある場合は、現行 workflow の trigger が
push/pull_request のみであることを再確認し、commit に `[skip ci]` を付けます。チェックは
未実行・Pending として扱います。public 化後は skip 指示のない新しい commit で最終 head の
全チェックを起動し、その head を再レビュー対象として固定します。公開だけで過去の失敗が
成功へ変わるとは扱いません。公開する内容が未判断なら private のまま保持します。
