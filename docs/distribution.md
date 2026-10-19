# 配布候補と版付きRelease

この手順は管理者向けです。利用者は[quickstart](quickstart.md)へ進みます。2026-10-19時点でRelease/tagは未発行です。CIは候補を作るだけで、push/PRからRelease、tag、deployを行いません。自動公開workflowはありません。

## 配布形式

| 用途 | Windows amd64 | Linux amd64 |
|---|---|---|
| Actionsレビュー | artifact ZIPを一度展開してexeとガイド | artifact ZIP内のtar.gzを展開 |
| 将来の手動Release | `xpotato-tunnel-<version>-windows-amd64-<commit>.zip` | 同じ命名の`linux-amd64` tar.gz |

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
python scripts/package-review.py --binary dist/xpotato-tunnel.exe --output dist/package --platform windows-amd64
```

Linux:

```sh
mkdir -p dist
go build -trimpath -o dist/xpotato-tunnel ./cmd/xpotato-tunnel
python3 scripts/package-review.py --binary dist/xpotato-tunnel --output dist/package --platform linux-amd64
```

出力先は新規ディレクトリを指定します。既存候補の上書きは拒否します。組立時に版とGit commit、cleanなbuild情報を照合し、archiveを再展開して全checksum・versionとLinux実行権限を確認します。`dist/package`に版・commit付きarchiveと外側の`.sha256`、`content`ができます。異なるソースの同版バイナリは拒否します。これは署名や再現ビルドの証明ではありません。

ActionsのWindows artifactは`content`を直接ZIP化するため二重包装になりません。GitHubが作るZIPのbyte列はローカルRelease候補ZIPと異なるので、外側ZIPのhashを混同しないでください。artifactは14日期限です。長期配布には別途本人によるRelease判断が必要です。

## 将来の手動Releaseの承認手順

以下は管理者が別途明示承認してから行う手順です。このPRの作成やmergeでは実行しません。

1. 対象versionと採用済みmainの完全commit SHAを選びます。同じSHAでWindows/Linux両方のGo・Node・runtime smoke・package検証CIが成功し、独立レビューが済んでいることを確認します。
2. 両OSのcleanな同一SHAから上記候補を作り、`build-info.json`のversion/commit/platform/dirty、内部checksum、外側archiveの`.sha256`、展開後versionを確認します。利用者が受け取るZIP/tar.gzを実際に展開して確認します。
3. 既存tag/Releaseがないことを確認し、GitHubのRelease作成UIで採用SHAを指す版付きtag（例`v0.2.7`）を選び、まず**Draft Release**として保存します。異なるcommitに既存tagがある場合は停止し、tag移動や上書きをしません。
4. 両OSのarchiveと各`.sha256`、対応SHA、版、runtimeは初回取得であること、更新手順、既知制約をDraftにそろえます。公開前に本人がassetと対象SHAを再確認します。
5. **本人が配布公開を明示承認した後だけ**Publishを行います。privateリポジトリのReleaseはアクセス権を持つ人向けで、一般公開にはなりません。公開後にREADMEの現在の入手先を更新します。

この承認は文書で示した運用上の停止点です。GitHub environment protectionが設定済みという意味ではありません。現状は自動公開機能自体を用意せず、意図しない公開を避けます。
