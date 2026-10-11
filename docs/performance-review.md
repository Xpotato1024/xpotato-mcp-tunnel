# 0.2.5 候補の測定とレビュー

LLM-01 の Windows amd64、Go 1.27.2、固定 Node 24.14.0／Tunnel 0.0.16／Commander 0.2.52 での合成測定です。旧版は main `9ac56a8d` の0.2.4、候補は今回のソースです。実キー・Vault・本人の設定・実 control-plane は使っていません。既存 runtime の公式コードだけを読み取り利用し、設定・HOME・作業場所は新しい fixture に分離しました。

## 全ファイル検証の割り当て

同じ256個の小さい合成ファイルを走査し、各ケース1秒・3回の中央値です。全 SHA-256、余分なファイル・欠落・改変・リンクの拒否を維持します。

| 走査 | bytes/op | allocs/op | GC/op | 時間/op |
| --- | ---: | ---: | ---: | ---: |
| 0.2.4 と同じファイル別バッファ | 8,751,858 | 3,014 | 2.529 | 13.367ms |
| 候補のバッファ再利用 | 287,759 | 2,241 | 0.09278 | 12.209ms |
| 候補の逐次検証（2つ目の map なし） | 246,810 | 2,222 | 未計測 | 12.227ms |

同じ map を作る比較で割り当て bytes は約96.7%、GC回数/走査は約96.3%減りました。小さい warm-cache fixture での結果で、実際の初回ダウンロードや cold disk の所要時間がこの比率で短くなる意味ではありません。GC 設定は変更していません。raw 出力は [go-inventory.txt](performance-results/go-inventory.txt)。

## 常駐 Node/DC と表示の負荷

各ケースを別プロセス・別 profile で開始し、10秒 idle と100回の合成 get_config を10秒間へ等間隔で送りました。RSS は時点値で peak ではありません。CPU 秒はプロセスの累積 CPU の差で、PC全体の CPU 使用率ではありません。

| 直接 stdio | idle CPU秒 | idle RSS | load CPU秒 | load RSS | 応答中央値 / P95 |
| --- | ---: | ---: | ---: | ---: | ---: |
| 0.2.4 | 0.078125 | 102.21MiB | 1.171875 | 105.20MiB | 2.856 / 3.541ms |
| 候補・通常表示 | 0.015625 | 101.89MiB | 1.000000 | 105.18MiB | 2.898 / 3.575ms |
| 候補・明示詳細 | 0.156250 | 100.30MiB | 1.000000 | 104.89MiB | 2.891 / 3.735ms |

通常表示は100件で6,984 bytes、明示詳細は14,276 bytesを stderr へ出しました。全100件の protocol 応答が返り、通常表示の中央値は旧版より約0.042ms増加しました。1回の短い測定と Windows CPU 時間の粒度では小さい差の有意性を主張できません。Node メモリ・GCの改善率は主張しません。UI console の描画コストはこの pipe の測定に含みません。raw 値は [node-dc.json](performance-results/node-dc.json)。

## 公式 Tunnel と Node/DC の別プロセス

公式 Tunnel は `127.0.0.1` の合成 poll/response server のみに接続しました。旧版と候補とも同じ Tunnel 0.0.16、各10秒 idle、100件の get_config を10秒間へ送りました。実サービスを使う接続性能ではありません。

| プロセス | 旧版 idle CPU秒 / RSS | 候補 idle CPU秒 / RSS | 旧版 load CPU秒 / RSS | 候補 load CPU秒 / RSS |
| --- | ---: | ---: | ---: | ---: |
| Tunnel 0.0.16 | 0 / 32.92MiB | 0 / 32.73MiB | 0.421875 / 34.09MiB | 0.390625 / 34.04MiB |
| Node/DC | 0.093750 / 102.32MiB | 0.078125 / 102.38MiB | 1.015625 / 105.55MiB | 0.906250 / 105.29MiB |

応答中央値は1.055→1.044ms、P95は1.410→1.242msで、各100応答完了です。依存本体のコードは変更しておらず、これらの揺れを依存の性能改善とは扱いません。raw 値は [tunnel-node-dc.json](performance-results/tunnel-node-dc.json)。

Go コアの core.Start／managed.Run の待機は、合成 Vault・localhost 模擬 Tunnel の同じ helper を両版で動かして別途測定しました。10秒待機の CPU 時間は両版とも0秒、終了時の RSS は51.79→52.74MiBでした。新しい版では受動的な localhost health 観測も動かしています。helper の RSS はテスト実行ファイルを含むため、配布 exe の常駐量とは区別します。小さい単回測定からコア常駐メモリの改善は主張しません。raw 値は [core-idle.json](performance-results/core-idle.json)。実際の Go launcher と公式 Tunnel を繋いだネットワーク負荷は、実 API 固定を緩めず今回の測定対象から外しています。

## 回帰と制約

Windows／Linux の全6 Go package の test と vet、Node38件が成功しました。固定 SDK/Commander の隔離 install smoke は initialize、26 tools、実際の合成ファイル検索、ツールの開始／応答表示を確認しました。secret の表示防止、改行・制御文字、body/result の非表示、観測の停止と protocol stdout の保全を合成テストで確認しています。race detector は両 OS に C compiler がないため実行できず、heartbeat と observer の終了同期をコードと回帰テストで確認しました。

レビューでは、CLIの明示詳細だけが observer に渡ること、SDK の公開ハンドラーを一度だけ包むこと、戻り値・例外を保持すること、固定 assets の hash 検証、処理の期限、owned process の停止経路を確認しました。新たな proxy、SDK/Commander/Tunnel fork、GOGC の変更はありません。Node/DC の大きい常駐メモリは依存側に残り、今回それを削減したとは主張しません。

採用と本人環境への適用は別の承認後です。今回のコードは本人の稼働中接続を変更・停止していません。

独立レビューで詳細表示の連結認証オプション・URL userinfo、遅い stderr のキュー上限、非同期表示エラー、アーカイブハッシュの期限を修正しました。修正後の独立 activity テスト8件と Go 進捗・接続・停止・入力テスト3 package は成功し、コードレビューに残るブロッカーはありません。activity キューは64KiB以内とし、backpressure 中のイベントを省略、表示先のエラー後は表示を無効化します。戻り値・例外は保持します。独立レビューは固定 Commander smoke を再実行しておらず、既存結果を確認しています。最終候補の隔離連携は PR の Windows／Linux CI でも確認します。
