# Setup・Start とツールの表示（0.2.5 候補）

採用済み main `9ac56a8d` は 0.2.4 です。今回の 0.2.5 はレビュー候補で、本人の実設定へは適用していません。同じコア版の別バイナリを上書きしないため、表示変更にも新しい版番号を使います。

## 待機の表示

Setup は保存前の yes の後に初回 runtime を取得・展開・全ファイル検証してからキー保存の同意に進みます。0.2.4 ではこの処理が無表示でした。今回、各工程の開始・完了・失敗と経過時間を表示し、5秒ごとに処理中の表示を追加します。推定の進捗率は出しません。入力前に heartbeat を終了してから説明と改行した `> ` 入力欄を表示します。秘密キーの非表示入力は保持します。

公式アーカイブの取得は各3分、npm は5分の既存期限を維持します。展開とファイルハッシュ走査は10分の期限を追加しました。期限はファイル間・読み取りの間で確認し、OS の停止した I/O を強制解除するものではありません。失敗には工程名を付け、旧設定・旧 runtime を保持する既存の保存順序を維持します。

Start は保存設定・許可範囲・実行ユーザー、runtime 全体の整合性、二重起動、資格情報ストア、専用資格情報の取得、子プロセス準備を stderr に表示します。秘密値は表示しません。子プロセスを起動しただけでは接続成功を表示しません。

この起動だけの `health-url.txt` から、`http://127.0.0.1:PORT` の受動的な `/health/control-plane` を読みます。proxy・redirect・資格情報の送付は使わず、1秒の request 期限、URL 1KiB・response 24KiB の上限があります。未確認・初回ポーリング成功待ち・成功を観測済み・エラー再試行・停止を区別します。成功とは control-plane poll の成功で、MCP ツール実行の成功ではありません。通常5秒間隔、成功観測後は15秒間隔にして1接続を再利用します。状態変更と15秒ごとの稼働表示を行います。停止時には観測を終えてから、この起動に所有する子プロセスの回収を表示します。

## ツールの表示

固定 Commander の公開 `Server.setRequestHandler` を包み、`tools/call` の開始と応答を観測します。入力 stream を消費・加工せず、stdout は MCP protocol のままです。通常は固定のツール名、開始、応答完了／失敗、所要時間だけを stderr に表示します。応答完了は handler が返った意味で、`start_process` 等が開始した非同期処理の終了を保証しません。Commander に届く前の拒否・配送失敗はこの表示の対象外です。

```text
xpotato-tunnel start --activity-details
```

本人がこのオプションを明示した場合だけ、少数の許可した引数名と値を追加表示します。command の改行は実際の改行と行頭の空白にし、ANSI・制御文字・方向制御文字はエスケープします。API key・token・auth・password・環境変数・本文・結果や未知の引数は非表示です。秘密らしい単語・高密度の文字列・認証系フラグ・変数・代入を含む文字列は丸ごと伏せます。引数16件、文字列2KiB、表示4KiB、配列8件・深さ2までです。表示の失敗でツールの返値・例外を変えません。

詳細の設定はこの起動のオプションだけで、保存しません。観測した引数・結果の収集や専用ログファイル保存は行いません。activity 表示の待機キューは64KiBを上限とし、stderr が混雑している間の表示を省略します。ツール応答を表示待ちで止めません。Tunnel 自体の従来の無制限 `tunnel-info.log` 保存もやめ、既定の stderr logger を使います。HTTP raw log は無効のままです。

## CPU・メモリ

全 runtime の SHA-256 を省略せず、ファイルごとの32KiBバッファと hash object の作成を走査ごとに一度へ減らしました。検証は receipt の hash と逐次比較し、二つ目の全ファイル map を作りません。新しい起動後の自動ダウンロード、GOGC 等の変更、依存 fork はありません。

`go test -run '^$' -bench '^BenchmarkRuntimeInventory$' -benchmem -benchtime=1s -count=3 ./internal/managed` は、同じ256個の合成ファイルで 0.2.4 の実装と新実装を比較します。`scripts/benchmark-activity.mjs --runtime-root 固定0.2.4のruntimeディレクトリ` は Windows の固定 Node 24.14.0／Commander 0.2.52 を使い、別々の隔離 profile で詳細なし／ありと旧版を比較します。各10秒 idle と100回の合成 get_config を10秒で送り、CPU・RSS の時点値・応答時間を測ります。Vault・実キー・control-plane は使いません。常駐した実接続の負荷や console の描画コストを測った値ではありません。結果と測定の制約は今回のレビュー記録へ保存します。

接続状態の意味と schema は公式 tunnel-client v0.0.16 の [PollHealth](https://github.com/openai/tunnel-client/blob/v0.0.16/pkg/controlplane/health.go)、[受動的な health API](https://github.com/openai/tunnel-client/blob/v0.0.16/pkg/runtimehealth/components.go) に基づきます。
