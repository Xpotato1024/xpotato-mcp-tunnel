# 配布候補と版付きRelease

この手順は管理者向けです。利用者は[quickstart](quickstart.md)へ進みます。正式配布は[Release v0.2.9](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/tag/v0.2.9)です。CIは候補を作るだけで、push/PRからRelease、tag、deployを行いません。公開は本人の明示承認後に管理者が手動で行います。

v0.2.9は起動ログ設定を修正した新しい版です。生成名はコアの版から決まり、既存v0.2.8のtag/assetsは変更しません。以後の公開にも採用mainの再検証と、新版tag・Releaseへの本人の明示承認が必要です。

## 配布形式

| 用途 | Windows amd64 | Linux amd64 |
|---|---|---|
| Actionsレビュー | artifact ZIPを一度展開してexeとガイド | artifact ZIP内のtar.gzを展開 |
| 手動Release | `xpotato-tunnel-0.2.9-windows-amd64.zip` | `xpotato-tunnel-0.2.9-linux-amd64.tar.gz` |

内容はコア、quickstartと関連ガイド、非秘密の`setup.env.example`、全ファイルの`SHA256SUMS`、`build-info.json`、Goの`build-info.txt`です。Node/Commander/Tunnel実体や資格情報は含めません。初回setup/updateは固定runtimeを取得するため通信が必要です。runtimeの取得仕様は[setup-once-core](setup-once-core.md)を参照してください。

## ローカルで候補を作る（公開なし）

作業をcommitし、cleanなcheckoutで、対象OSのamd64用Go 1.27.2とPython 3.10以上を使います。利用者側にはどちらも不要です。`dist`はGit管理外です。実端末のsetup/start/updateは実行しません。

```text
go test -count=1 ./...
go vet ./...
python -m unittest discover -s test -p test_package.py -v
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
