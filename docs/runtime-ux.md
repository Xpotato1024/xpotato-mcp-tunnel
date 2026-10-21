# Setup・Update・Start とツールの表示

> 次版候補では通常の新規導入・更新・起動から全件再走査を除きます。[検査範囲・限界・受入条件](installer-lifecycle.md) を参照してください。以下は従来の説明です。

正式配布は[Release v0.2.10](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/tag/v0.2.10)です。同じコア版の別バイナリは上書きしません。CIの同版再ビルドを正式Releaseの代わりにupdateへ使わず、新しい配布には新しい版番号を使います。本 PR のセキュリティ更新候補は Node 24.22.1／Tunnel 0.0.16／Commander 0.3.0 です。既存端末の切替や正式配布は別手順です。

## 待機の表示

Setup は保存前の yes の後に初回 runtime を取得・展開・全ファイル検証してからキー保存の同意に進みます。0.2.5からの各工程の開始・完了・失敗と経過時間、5秒ごとの処理中案内を保持します。工程ログはstderr、対話説明と入力欄はstdoutです。正常な接続待機とは別の案内です。推定の進捗率は出しません。入力前に heartbeat を終了してから説明と改行した `> ` 入力欄を表示します。秘密キーの非表示入力は保持します。

公式アーカイブの取得は各3分、npm は5分の既存期限を維持します。展開とファイルハッシュ走査は10分の期限を追加しました。期限はファイル間・読み取りの間で確認し、OS の停止した I/O を強制解除するものではありません。失敗には工程名を付け、旧設定・旧 runtime を保持する既存の保存順序を維持します。

Start は保存設定・許可範囲・実行ユーザー、runtime 全体の整合性、二重起動、資格情報ストア、専用資格情報の取得、子プロセス準備を stderr に表示します。秘密値は表示しません。子プロセスを起動しただけでは接続成功を表示しません。

Updateにもruntime取得・展開・検証等の工程表示を渡します。これは本人が実行した更新処理の進捗で、正常な接続待機の定例通知ではありません。更新は保存版の次回起動用参照を切り替える処理で、既存接続を再起動しません。

この起動だけの `health-url.txt` から、`http://127.0.0.1:PORT` の受動的な `/health/control-plane` を読みます。proxy・redirect・資格情報の送付は使わず、1秒の request 期限、URL 1KiB・response 24KiB の上限を保持します。利用者向けの呼び名は「OpenAI トンネル」です。初回接続待機・接続確認・通信中断/接続エラー・再接続確認・未確認・切断を区別し、**変化したときだけ**表示します。正常待機の15秒通知も5分等の定例通知もありません。受動的な状態読み取りは従来どおり通常5秒、接続確認後15秒間隔です。外部のlong polling設定は変えません。

接続確認には新しいsnapshot時刻・最終poll成功時刻・poll期限を使います。snapshotが5秒より古い、1秒を超えて未来、最終成功がpoll期限+5秒より古い、現在のpollが期限+1秒を超えた、必要な時刻や期限が欠ける場合は未確認です。復帰後に古い成功だけで接続済みとは表示しません。長い処理で新しいpollがない場合も接続を推測しません。これは診断上の期限で、通信や要求の期限を変更するものではありません。接続確認はOpenAI側pollの成功で、MCP操作の成功は下記の別イベントです。停止時には観測を終えてから所有する子プロセスの回収を表示します。

## 色・記号と出力先

`[OK]`（接続確認/応答成功）、`[ERR]`（失敗）、`[RUN]`（MCP処理開始）、`[OFF]`（停止/切断）、`[WAIT]`（未確認/初回待機）、`[INFO]`（工程）の文字ラベルを併記します。ANSI色はstderrがTTYかつ対応端末のときだけです。`NO_COLOR`が存在する場合（空値も含む）、非TTY、`TERM=dumb`では無色です。WindowsはVT対応console modeを読み取り判定し、変更しません。従来のWindows PowerShellもASCII記号と日本語ラベルで読めます。絵文字の表示幅・フォント差を避けるため、既定の記号はASCIIにしました。行の上書きやcursor制御はしません。

Start/Updateの工程・接続表示と子runtimeのログは、それぞれ1行4KiB、書き込み中を含め64KiBの非ブロッキングqueueでstderrへ送ります。混雑中は表示を省略し、表示先のエラー後は無効化します。各queueの終了時flush待ちは最大200msです。子runtimeログの途中行も4KiBに制限し、長すぎる行は全体を省略します。可能な範囲で省略通知を出します。制御文字・方向制御文字をエスケープし、安全な色のSGRだけをTTYで保持します。Setup工程は入力と順序を保つため従来の同期表示を維持します。

子runtimeのログpipeを保持した子孫がleader終了後も残る場合、コピー待ちは1秒で打ち切り、その後に既存の所有子プロセス回収へ進みます。この待機上限は通信中の要求期限を変更しません。0.2.9は`struct-text`を明示し、runtime自身のstdout（構造化ログ）とstderrを同じマスク・上限付き表示へ送ります。Commanderとruntime間のMCP stdin/stdoutはruntime内部の専用pipeで、変更しません。コアのstdoutへruntimeログは転送しません。

## ツールの表示

固定 Commander の公開 `Server.setRequestHandler` を包み、`tools/call` の開始と応答を観測します。入力 stream を消費・加工せず、stdout は MCP protocol のままです。通常は固定のツール名、処理開始、応答成功／失敗応答／処理失敗、所要時間だけを stderr に表示します。応答成功はhandlerがisErrorなしで返った意味で、`start_process` 等が開始した非同期処理の終了を保証しません。Commanderに届く前の拒否・配送失敗はこの表示の対象外です。MCP表示の色も起動端末のTTY/NO_COLOR判定に従います。

```text
[OK] OpenAI トンネル 接続を確認（要求を待機）
[RUN] [MCP #1] list_directory 処理開始
[OK] [MCP #1] list_directory 応答成功 (12ms)
[ERR] OpenAI トンネル 通信中断・接続エラーを検出（再接続を試行中）
[OK] OpenAI トンネル 再接続を確認（要求を待機）
```

```text
xpotato-tunnel start --activity-details
```

本人がこのオプションを明示した場合だけ、少数の許可した引数名と値を追加表示します。command の改行は実際の改行と行頭の空白にし、ANSI・制御文字・方向制御文字はエスケープします。API key・token・auth・password・環境変数・本文・結果や未知の引数は非表示です。秘密らしい単語・高密度の文字列・認証系フラグ・変数・代入を含む文字列は丸ごと伏せます。引数16件、文字列2KiB、表示4KiB、配列8件・深さ2までです。表示の失敗でツールの返値・例外を変えません。

詳細の設定はこの起動のオプションだけで、保存しません。観測した引数・結果の収集や専用ログファイル保存は行いません。activity 表示の待機キューは64KiBを上限とし、stderr が混雑している間の表示を省略します。ツール応答を表示待ちで止めません。Tunnel の専用ログファイル保存は行わず、stderr loggerを使います。HTTP raw log は無効のままです。

## 内部診断ログ

通常のupstreamログはWARN以上で、dispatcher/配送INFOや長い相関IDを表示しません。子MCPの処理イベントは公式stdio transportが直接stderrへ送るため、ログレベルとは独立して表示します。本人が`start --diagnostics`を明示した場合だけ、upstreamのINFOも表示します。`--activity-details`は別の選択で、diagnosticsだけでは引数表示を有効にしません。設定には保存せず、HTTP raw logも有効にしません。

取得したRuntimeキーはSHA-256 fingerprintで完全一致をマスクします。起動後に平文キーbufferを消去しても、Write境界をまたぐ同じ行のキーを非表示にできます。Bearer/API key/password等の認証候補値もマスクします。未知の秘密やファイル名等の相関情報を網羅する保証ではなく、共有前の本人確認は必要です。上限による省略があるため、完全な監査ログとしては扱いません。[診断の手順](operations.md)を参照してください。

## CPU・メモリ

全 runtime の SHA-256 を省略せず、ファイルごとの32KiBバッファと hash object の作成を走査ごとに一度へ減らしました。検証は receipt の hash と逐次比較し、二つ目の全ファイル map を作りません。新しい起動後の自動ダウンロード、GOGC 等の変更、依存 fork はありません。

`go test -run '^$' -bench '^BenchmarkRuntimeInventory$' -benchmem -benchtime=1s -count=3 ./internal/managed` は、同じ256個の合成ファイルで 0.2.4 の実装と現実装を比較します。`scripts/benchmark-activity.mjs --runtime-root 固定0.2.4のruntimeディレクトリ` は Windows の固定 Node 24.14.0／Commander 0.2.52 を使い、別々の隔離 profile で詳細なし／ありと旧版を比較します。各10秒 idle と100回の合成 get_config を10秒で送り、CPU・RSS の時点値・応答時間を測ります。Vault・実キー・control-plane は使いません。既存の [測定記録](performance-review.md) とJSONは0.2.5採用前の履歴として保持し、0.2.7の測定結果には読み替えません。今回、実端末の負荷・電池・console描画コストや性能改善率は主張しません。

接続状態の意味と schema は公式 tunnel-client v0.0.16 の [PollHealth](https://github.com/openai/tunnel-client/blob/v0.0.16/pkg/controlplane/health.go)、[受動的な health API](https://github.com/openai/tunnel-client/blob/v0.0.16/pkg/runtimehealth/components.go) に基づきます。
