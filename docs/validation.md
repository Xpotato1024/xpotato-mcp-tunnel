# 隔離回帰と互換性

現行入口はGoコアです。撤去した旧Node CLIのbin/src/manifest/npm入口と専用テストを開発コマンドに使いません。旧実装はGit履歴に残り、本人PCの配布物・バックアップ・OS保管庫・設定を削除する処理はありません。

```text
go test -count=1 ./...
go vet ./...
node --check internal/managed/assets/stdio.mjs
node --check internal/managed/assets/activity.mjs
node --test test/activity.test.mjs
```

Go通常テストは合成キー、fake Vault、別home/profileを使います。Windows/Linuxで新規Setup、途中再開、確認付き変更、update/失敗時の旧参照、古いcoreからの更新、旧ラッパーの置換、キー再入力なし、scope/identity/lock拒否を検証します。既存core/shim更新互換と旧版保持は撤去対象ではありません。

表示は正常待機16秒、接続/中断/復帰、古いpoll成功、TTY/NO_COLOR、制御文字、秘密mask、queue上限、遅い/壊れた出力先、stderrとstdout分離を検証します。Node activityは固定SDKの公開handler境界で結果・例外を保持します。

CIのGoジョブはXPOTATO_RUNTIME_SMOKE=1で固定runtimeを新規fixtureに導入し、hash/version、隔離stdio initialize/tools-list、PATH空の実検索を確認します。実キー/control-planeは使いません。Linux保管庫テストは専用の一時D-Bus/fake serviceを使います。停止テストは自身のfixtureだけを生成・回収します。

0.2.8までのhash/versionと直接stdio smokeは、Goの`Manager.Run`が渡すTunnel起動設定を通していませんでした。0.2.9候補では同じsmoke内で、実際の`Manager.Run`→所有子プロセス→SHA256固定のTunnel 0.0.16の設定パーサーを通常／diagnosticsの両方で検証します。合成キーと正規形式のfixture Tunnel IDだけを使います。

固定runtimeにはvalidate-onlyコマンドがないため、テストshimは本番と同じ引数・環境・作業ディレクトリを渡した末尾に`--harpoon.max-response-bytes=0`だけを追加します。[上流の設定ロード](https://github.com/openai/tunnel-client/blob/v0.0.16/pkg/runtimeconfig/config.go#L658-L749)はloggingの後にこの制約を検査し、[アプリ構築](https://github.com/openai/tunnel-client/blob/v0.0.16/cmd/client-runtime/root_command.go#L22-L32)前に必ず終了します。期待するこのエラーに到達することでログ設定の受理を確認し、control-plane通信、health listener、MCP子プロセスは起動しません。歴史的な`warn + 空形式`の拒否と`info + 空形式`の受理も同じ実バイナリで確認します。完全なオンライン接続試験ではありません。

取得済みのSHA固定native binaryだけでこの回帰を実行する場合は、`XPOTATO_TEST_TUNNEL_RUNTIME`にそのパスを指定して`go test -v -count=1 ./internal/managed -run '^TestPinnedRuntimeStartupConfiguration$'`を使えます。通常のCIでは既存smokeの導入済みfixtureを再利用し、追加の配布や端末設定は行いません。

CIはWindows/LinuxのGoとNode activity、review packageの作成・展開・checksum/version確認のみです。deploy/publishはありません。scripts/benchmark-*.mjsと0.2.5測定JSONはGo版の開発/履歴として保持し、新版の性能改善や電池負荷には読み替えません。

race detectorはC compilerが必要です。実接続でのスリープ・回線切替・手動描画・長時間負荷は隔離回帰と区別します。既存Linux heartbeatファイル比較の不安定な履歴は各main配布記録に残しています。
