# Installer と runtime 検査の設計（0.2.11 Draft）

これは未公開の変更案です。公開済み v0.2.10 の asset/tag は変更しません。
Tunnel core は Go を維持します。Laptop への導入や既存プロセスの停止はこの開発作業に含みません。
全OSの一般利用者向け入口は共通GUIを目標とし、現PRのターミナル保守画面は途中段階です。
[各OSの配布・GUI・署名・未対応範囲](installer-platforms.md) を参照してください。

## 利用者の入口

Windows の通常導入は版付き `xpotato-tunnel-VERSION-windows-amd64-setup.exe` 一つを入口とします。
ユーザー単位でアプリを配置し、最後に保守メニューを開きます。Setup / Start / Update / Repair / Doctor を選べます。
Setup は既存の許可フォルダー確認と、同意後の非表示キー入力を使います。Installer が新しい認証方式や同意済み扱いを追加することはありません。
Go/Node/npm の事前導入は不要ですが、固定 runtime の初回取得には通信が必要です。

Start は保存済みの core/runtime の選択に従います。停止は起動したターミナルで Ctrl+C です。
Update / Repair / Installer は既存プロセスを停止せず、更新は次の起動から適用します。
Repair は同じ固定レシピから新しい世代を取得します。壊れた世代の上書き、rename、削除はしません。
途中で失敗すると既存の選択を維持し、自分が作った一時 stage だけを片付けます。

アンインストールは Windows の「インストールされているアプリ」から行います。
ユーザー所有かつ記録した SHA と一致する起動用ラッパーと、この版のアプリファイルを削除します。
変更されたラッパーは保持してエラーを返します。プロファイル、runtime 世代、保存済みキーは保持します。
別の `--home` で作ったラッパーは、その home を指定した明示的 `uninstall-entrypoints` で扱います。
資格情報の削除・アクセス権変更・OS設定・サービス登録・PATH変更・自動起動は追加しません。
旧版のアプリディレクトリは実行中・rollback の可能性があるため自動で掃除しません。

## 技術選択

| 案 | 既存 Go core との関係 | 判断 |
| --- | --- | --- |
| Go 独自 Installer | 現行コードを再利用できるが、GUI・登録・削除・更新処理を自作 | OS向けの外側を自作する保守を避ける |
| Rust Installer | core を呼べるが、別 toolchain と独自 lifecycle 実装が増える | 今回の問題に対する利点が小さい |
| Tauri | Rust と Web UI、Windows配布は NSIS/MSI、WebView2 の扱いも必要 | 全OS共通GUIの第一候補。現PRでは未実装 |
| WiX MSI / Burn | Windows Installer の lifecycle を利用できる | MSI/component/bundle の管理が現状の用途には重い |
| Inno Setup | ユーザー単位の一体型 Installer を作れる | 有力候補。既定のアプリ終了機能を無効化する設計が必要 |
| **NSIS + Go** | NSIS は小さなユーザー単位の配置・削除、Go が既存の Setup/更新を担当 | **Windows外装の選択。共通GUIは別層** |

NSIS は `RequestExecutionLevel user`、zlib 圧縮、任意 plugin なしです。
CRC は破損検知であり、発行者認証ではありません。Installer 内の core digest はビルド時に固定し、
既存 exe を呼ぶ前にも OS 付属 PowerShell で照合します。ExecutionPolicy は変更しません。
署名や外部 digest を含む取得時の発行者確認は、公開前の別の決定事項です。

根拠: [NSIS scripting](https://nsis.sourceforge.io/Docs/Chapter4.html)、
[NSIS license](https://nsis.sourceforge.io/License)、
[Tauri Windows installer](https://v2.tauri.app/distribute/windows-installer/)、
[WiX Burn](https://docs.firegiant.com/wix/tools/burn/)、
[Inno per-user](https://jrsoftware.org/ishelp/topic_setup_privilegesrequired.htm)、
[Inno application closing](https://jrsoftware.org/ishelp/topic_setup_closeapplications.htm)。
保守量についての判断はこのプロジェクトの規模を踏まえた設計判断です。

## 通常導入・更新に全ファイル検査を置かない

v0.2.10 では選択された runtime の展開済み全ファイルを再走査していました。
実機報告では 9分59秒、25379 files / 273380639 bytes の後、10分の期限で更新失敗しました。
表示には既存 runtime 保持とありましたが、この開発作業で実機内部を検査した事実はありません。

| 段階 | 通常経路で行うこと | 行わないこと |
| --- | --- | --- |
| 配布 core | 展開中のNSIS CRC、固定 core digest と起動/version確認。payload全hashは候補作成時/明示診断 | 導入後のdocs/payload全件再hash、保存済み runtime の全走査 |
| 新規取得 | Tunnel/Node archive を書き込みながら固定 SHA256 と照合。固定 lock による npm ci / SRI、ignore-scripts | archive を hash のため再度全読み込み |
| 展開 | 安全なパスと通常ファイルを確認して必要な展開を行う | 展開後全依存の receipt 作成・二重全件 hash |
| 新規導入・更新・起動 | 起動入口と構成7ファイルの存在・通常ファイル・hash、組込み pin、Commander patch、所有者・scope・core pin 確認 | file count に比例する余分な全依存走査 |
| `doctor --deep` | 明示操作で全ファイル比較。旧 receipt はその全件記録、新 receipt は固定取得元から一時 reference を作り比較 | 自動呼び出し・資格情報読み取り |
| `repair` | 新規取得と同じ archive/SRI と起動入口確認で新世代を作る | 旧全件走査を前提にすること・既存プロセスの停止 |

新 receipt は `schemaVersion: 2`, `scope: launch-files` として範囲を明示します。
既存の全件 receipt は移行のために書き換えず、そのうち起動入口だけを通常検査で読みます。
起動入口検査は30秒、archive取得は各3分、npmは5分の既存上限を使います。
npm が正常に動いたことと固定版の取得が最小の起動・版確認となります。ネット接続や利用者の資格情報を試す probe は追加しません。
deep 診断は10分上限で、一時 reference の取得・展開・全走査という重い処理を含みます。

これは保存後の全 transitive dependency の改ざん・欠損を毎回検出する保証ではありません。
必須の取得整合性、パス検査、owner/scope、固定 core と重要な起動入口の検査は維持します。
同じ利用者が書き換えられる receipt は署名ではありません。推移依存の保存後変更の検査責務は明示的 deep 診断、
正常な固定内容への復帰は repair が担います。起動失敗の際にも勝手に deep 診断や再取得を開始しません。

進捗は `files 3/7` のように同じ単位で表し、byte総量が不明なら
`files 25379; bytes 273380639 (totals unknown)` のように別項目にします。
各段階の開始・処理中・完了・中断・タイムアウト・失敗を明示します。
段階の完了と更新全体の完了を区別し、CLI は最後に成功を表示するか非ゼロで失敗します。

## 配布内の分類

通常利用者は Installer 一つから進みます。portable ZIP は上級者用の代替として残します。
ZIP root は exe、短い README、検証 metadata/helper、非秘密の設定例、`docs/`、`licenses/` です。
docs と performance-results は `docs/`、LICENSE/COPYRIGHT/NOTICE/CHANGES は `licenses/` に置きます。
法的表示と対応 source の導線は残します。公開済み v0.2.10 の quickstart はその版の資料として残します。

## ローカル候補の作成と公開境界

`package-installer.py` は確認済み `package-review.py` の Windows content、別途レビューした
NSIS compiler の絶対パスと SHA256、その distribution の license ファイルを受け取ります。
toolchain の自動 download/install、Release、tag push、配布公開は行いません。

```text
python scripts/package-installer.py --content dist/package/content --output dist/installer --makensis ABSOLUTE_MAKENSIS_EXE --compiler-sha256 REVIEWED_SHA256 --nsis-license REVIEWED_LICENSE_FILE
```

開発候補は公式 NSIS 3.13 portable ZIP の取得bytesを `installer/nsis-toolchain.json` の SHA256 に固定しています。
`fetch-installer-tool.py --output NEW_DIRECTORY` で固定ZIPを取得・検査・展開し、OSへinstallせず使えます。
compiler と配布元 COPYING も個別に固定し、compilerの外部設定は読みません。これは発行者署名の代替ではありません。
NSIS license を payload に同梱し、対応 source bundle には Installer/script を含めます。
署名・発行者認証と公開に関する最終判断は保留です。
push/PR workflow は contents:read で検査と artifact 作成のみです。自動 deploy/publish はありません。

## 受入条件と未実施の検証

- 新規導入・reuse・Start・Update・Repair から expanded-tree inventory が呼ばれない。
- archive の streaming SHA/SRI/安全な展開と、重要ファイルの欠損・変更拒否が保たれる。
- `doctor --deep` のみ全件比較し、失敗・中断・timeout が非ゼロになる。
- repair で既存世代を変更せず、失敗時は旧選択が維持される。
- Installer の新規・再実行・版更新・中断・修復・削除・変更済み wrapper・実行中ケースを隔離した Windows fixture で確認する。
- legal/source を含む package の相対リンク、同一 commit の Windows/Linux CI、独立レビューを確認する。

2026-10-21 05:35:17 UTC に性能計測の負荷保留が解除されました。
Windows/Linux Goテスト各9パッケージとvet、Python13テスト、NSIS3.13候補コンパイルを確認しました。
独立した静的レビューの4指摘を修正・再確認しています。
Installer実行は実端末では行わず、隔離されたGitHub-hosted Windows runnerだけで行うfixtureを追加しています。
exact-head CI、共通GUI、macOS/ARM、対話キャンセル/実際の版更新を含む全受入条件が揃うまで公開可能とは扱いません。
