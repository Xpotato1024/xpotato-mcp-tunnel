# 0.2.10の更新処理

0.2.10は更新時の検証重複を減らし、core/runtimeの切替と取消・再開を改善します。配布物の取得・検証は[quickstart](quickstart.md)、管理側の公開手順は[distribution](distribution.md)を参照してください。実端末への導入とWindows検出内容の確認は別の作業です。同版異bytesを避けるため、0.2.9からコア版を進めています。

## 準備と切替

`update`は最初に実行中の新coreを読取り、恒久保存先の同版coreと照合し、必要なら同じディレクトリのstageへコピー・検証します。読取拒否や同版異bytesで止まった場合、ダウンロード・npm展開へ進みません。AVによる拒否を例外扱いして継続したり、保護設定を変更したりしません。

runtimeのIDはcore版から分離し、platform、公式archive/exeの固定digest、依存lock、stdio/activityアダプター、Chrome抑止の生成方式を含むrecipeで決めます。生成方式を変えるときはrecipe番号も更新します。現在選択されている0.2.6〜0.2.9のruntimeも、互換IDとreceiptのmanifest・全ファイル・固定digestを検証できた場合に限り、その場所で再利用します。他homeや無関係なruntimeは探索しません。互換性のない版は新しいrecipeを準備し、壊れた互換候補は停止します。

更新時の全走査は、新規runtimeではSHA記録と最後の全検証の2回、既存runtime再利用では最後の全検証の1回です。準備処理がinstall lockを保持し、検証後に新stageを保存します。その後、coreを再照合し、`device.json`の`core`（version/SHA256）、`release`、`previousCore`、`previousRelease`を一回のファイル置換で切り替えます。scope・credential参照・Tunnel IDは変えず、Vaultは読みません。稼働中の旧接続を停止・再起動しません。

通常の`start`では毎回、core pinと全runtimeファイルを検証してから資格情報を取得します。mtime、永続的な検証済みフラグ、前回の成功だけで全検証を省きません。receiptとpinは保存時の整合性を照合する情報で、署名やAV判定ではありません。install lockは協調する処理を排他にしますが、任意の同一ユーザーのファイル書換えを防ぐ境界ではありません。

## 短縮コマンドの移行と復旧

初回updateで短縮コマンドを固定launcherへ移します。先に旧coreの版とSHAをfallbackとしてcommand-install metadataへ保存し、承認済みの場所だけラッパーを順次置換します。途中で止まっても、旧ラッパーと新ラッパーは旧世代を選びます。未更新の別deviceもfallbackを使います。本人が編集したラッパーは上書きしません。同じ新exe・home/deviceで再実行すると、記録された旧bytesか正しい新bytesだけを受け入れて移行を再開します。

移行後の起動と`version`はdeviceのcore pinを選び、setup/update/rollback/doctor/credentials/pluginは世代情報を理解する管理coreを使います。command-installの`Version`は管理coreの版で、次回起動版は短縮コマンドの`version`またはdeviceの`core`で確認します。稼働中プロセスの版は別です。固定launcher・旧core・旧runtimeは削除しません。

旧形式の短縮コマンドが残るhomeでは、新exeのSetupで更新を代用せず、先に明示的な`update`を完了します。Setupの事前確認でもこの条件を検査します。移行途中からSetupを実行して旧起動経路を失うことを防ぎます。

`rollback`は保存済みのcoreとruntimeの組を検証して一緒に戻します。現管理coreが全検証できる互換runtimeが対象で、任意の古い依存版へ戻す機能ではありません。旧coreへの切替と起動が競合した場合、完全な旧世代が続行するか、整合性確認で資格情報取得前に停止し、短縮コマンドからの再試行が必要になる場合があります。

切替前の失敗・取消では旧世代を維持します。切替後の中断では新世代全体が選択されています。未使用の新core/runtimeや強制終了時のstageが残る場合がありますが、それを検証済みとして自動採用・自動削除しません。OSロックはプロセス終了で解放されます。ファイルのSync後に同じディレクトリで置換し、プロセス中断への整合性を検証しています。電源断やファイルシステム故障への無条件の耐久性は保証しません。

## 取消・進捗と検証範囲

update/start/rollbackのCtrl+Cはcoreコピー、runtime走査、HTTP取得、npmの所有子プロセスへ伝播します。対話Setup/credentialsの入力中は従来の中断動作を保ちます。npmは上限5分、ファイル処理は工程ごとに上限10分、archive取得は上限3分です。OS内部で停止したOpen/Readはcontextだけで即時中断できず、これらは絶対的な強制終了時刻ではありません。切替直前に取消を確認し、置換成功後の取消を「旧版のまま」と報告しません。

走査の表示には経過時間と完了済みファイル数・bytesを含めます。停止中のI/Oでも件数が進んだようには表示しません。秘密値、URL、個別ファイル名は進捗へ追加しません。

fixtureで変更・欠落・追加、固定ファイルとreceiptの同時改変、移行後の別device、同版異bytes、保存地点ごとの失敗・取消・強制終了と再開、core/runtimeの同時rollback、引数と終了コードの転送、子プロセス回収を検証します。性能fixtureは実runtimeと分離した26,067個の合成ファイルです。走査回数を比較し、Laptopの実所要時間やAV検出原因は推定しません。

## 合成測定

2026-10-20、LLM-01 Windows amd64 / Go 1.27.2で、書込直後の同じ26,067個の合成ファイルを使い、各ケース1回×3測定の中央値です。約27MiBの小ファイル主体で、一部を256KiBにしています。旧版exeの実行ではなく、同じinventory関数を0.2.9相当と候補の走査回数で呼んだ比較です。

| 構成 | 全走査数 | 時間の中央値 |
| --- | ---: | ---: |
| 新規runtime・従来相当 | 3 | 9.04秒 |
| 新規runtime・候補 | 2 | 6.05秒 |
| 既存runtime・従来相当 | 2 | 6.02秒 |
| 既存runtime・候補 | 1 | 3.02秒 |

raw値は[update-inventory-windows.txt](performance-results/update-inventory-windows.txt)。再現は`go test ./internal/managed -run '^$' -bench '^BenchmarkUpdateInventoryPasses$' -benchtime=1x -count=3`です。ダウンロード・npm・coreコピー・ラッパー移行は含みません。cold disk、実runtimeの内容、AVの状態を再現した測定ではなく、実更新全体や起動時間の改善率には換算しません。
