# 確認・診断・再開

通常は本人向けイベントと警告・エラーをstderrへ表示します。dispatcher/配送INFOや長い相関IDは既定で表示しません。正常待機の定例表示もありません。既存設定や資格情報を削除せず、次の経路を使います。

| 状況 | 本人の確認・次の操作 |
|---|---|
| 保存版が不明 | 短いコマンドに`version`を付ける。稼働版とは別 |
| Setup途中で停止 | 同じexe・home/deviceでsetup再実行。作成済み設定と保存済み専用キーは通常再利用 |
| update失敗 | 新exeのupdateを再実行。旧版と途中metadataによる再開を利用。成功まで新しいstartをしない |
| 二重起動 | 元の端末と同じTunnel IDの別端末を確認。本人が終了を待つ。lock削除で回避しない |
| 接続未確認/エラー | 外向きHTTPS、スリープ/回線、Tunnel IDの登録を本人が確認。poll成功とMCP成功を区別 |
| 許可範囲の不一致 | 同じdeviceのsetupで差分確認。無断config編集や範囲拡張をしない |
| 資格情報が利用不可 | Windowsは同じ通常ユーザー、Linuxは同じUIDと解除済み保管庫を確認。credentialsは本人同意・非表示入力で該当一レコードを更新 |

## 診断表示を明示する

本人が旧接続を終了してから、短いコマンドで次を実行します。

```text
xpotato-tunnel start --diagnostics
```

upstreamのINFO（起動情報、dispatcher、相関ID等）をstderrへ表示します。警告・エラーは通常起動でも表示します。HTTP raw logは常に無効。既知Runtimeキーと認証候補値をマスクし、制御文字をエスケープします。専用ログファイルや設定は作りません。この起動だけの選択です。

ツール引数は別に`start --activity-details`を明示します。--diagnosticsだけでは引数表示を有効にしません。本文・結果や未知の引数の収集機能ではありません。共有前はファイル名や操作の相関情報を本人が確認してください。

stderrは1行4KiB、各queueは書込み中を含め64KiBです。長い行や混雑は可能な範囲で省略通知を出します。遅い/壊れた表示先で処理・回収を止めません。省略があり得るため完全な監査記録とは扱いません。stdoutはMCP経路として保持します。

## doctorと保存先

`doctor`は保存先とinstall lock、`doctor --device 端末名`はその設定・runtime/lock整合性を読み取ります。資格情報の値、実接続、アカウント登録は検査しません。成功だけでオンライン/MCP成功とは判定しません。

恒久homeにはcore、releases、devices、command-install metadataとロック、キーはOS保管庫にあります。配布フォルダーと恒久homeは別です。通常のupdateは旧core/runtimeや設定を削除しません。rollbackは現コアが認識する保存済みruntimeに限ります。

新規ユーザーも更新ユーザーも[導入の入口](quickstart.md)から進めます。公開インストーラ、自動常駐、アカウント自動登録はありません。
