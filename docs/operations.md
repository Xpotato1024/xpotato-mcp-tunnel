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

## v0.2.8のログ設定エラー

公開済みv0.2.8の通常起動では、資格情報取得・子プロセス準備後に`configure tunnel-client runtime: log level requires 'struct-text' or 'json' log format`と表示され、子プロセスがexit 1になる不具合があります。通常起動の`warn`と未指定ログ形式の組合せを固定Tunnel 0.0.16が拒否するためです。キー不正を示すエラーではなく、設定削除・キー再登録やruntimeファイルの手編集では解決しません。

本人が元の起動プロセスの終了を確認してから、公開CLIの次の指定を使えます。同じ既存home/deviceを選び、普段必要な`--home`／`--device`指定があればそのまま付けます。

```text
xpotato-tunnel start --diagnostics
```

この起動だけ`info`を選び、v0.2.8のログ設定エラーを回避します。保存設定・runtime整合性・資格情報は変更しません。内部INFOと相関ID等の表示が増えますが、HTTP raw logは無効、ツール引数表示も別オプションのままです。固定実バイナリの設定検証で確認した回避であり、実認証・外向きHTTPS・ChatGPT登録・MCP操作の成功は別に確認が必要です。

正式版v0.2.9は通常`warn`／診断`info`の両方に`struct-text`を指定し、この設定エラーを修正しています。公開済みv0.2.8の同名assetは置き換えません。本人が[新しい正式Release](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/tag/v0.2.9)のexeから`update`を実行し、保存更新の完了後に旧プロセスを停止・終了確認して、普段の短い起動コマンドで再開します。同じhome/deviceの設定・保存済みキー・Tunnel IDを維持し、Setupやキー再登録は通常不要です。同版CI再ビルドへのupdateはしないでください。

## 診断表示を明示する

本人が旧接続を終了してから、短いコマンドで次を実行します。

```text
xpotato-tunnel start --diagnostics
```

upstreamのINFO（起動情報、dispatcher、相関ID等）をstderrへ表示します。警告・エラーは通常起動でも表示します。HTTP raw logは常に無効。既知Runtimeキーと認証候補値をマスクし、制御文字をエスケープします。専用ログファイルや設定は作りません。この起動だけの選択です。

ツール引数は別に`start --activity-details`を明示します。--diagnosticsだけでは引数表示を有効にしません。本文・結果や未知の引数の収集機能ではありません。共有前はファイル名や操作の相関情報を本人が確認してください。

stderrは1行4KiB、各queueは書込み中を含め64KiBです。長い行や混雑は可能な範囲で省略通知を出します。遅い/壊れた表示先で処理・回収を止めません。省略があり得るため完全な監査記録とは扱いません。runtime自身のstdout/stderrログはコアのstderrへ送り、runtimeとCommander間の専用MCP入出力は保持します。

## doctorと保存先

`doctor`は保存先とinstall lock、`doctor --device 端末名`はその設定・runtime/lock整合性を読み取ります。runtime検証はreceiptと全ファイルのhash・固定版の照合で、起動引数を実行する設定検証ではありません。資格情報の値、実接続、アカウント登録も検査しません。成功だけで起動設定の受理・オンライン/MCP成功とは判定しません。

恒久homeにはcore、releases、devices、command-install metadataとロック、キーはOS保管庫にあります。配布フォルダーと恒久homeは別です。通常のupdateは旧core/runtimeや設定を削除しません。候補0.2.10のrollbackは保存済みcore/runtimeを組で戻し、現管理coreが全検証できる互換runtimeに限ります。[世代切替と再開の詳細](update-transactions.md)を参照してください。

新規ユーザーも更新ユーザーも[導入の入口](quickstart.md)から進めます。公開インストーラ、自動常駐、アカウント自動登録はありません。
