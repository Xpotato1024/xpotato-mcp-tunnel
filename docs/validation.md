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

CIはWindows/LinuxのGoとNode activity、review packageの作成・展開・checksum/version確認のみです。deploy/publishはありません。scripts/benchmark-*.mjsと0.2.5測定JSONはGo版の開発/履歴として保持し、新版の性能改善や電池負荷には読み替えません。

race detectorはC compilerが必要です。実接続でのスリープ・回線切替・手動描画・長時間負荷は隔離回帰と区別します。既存Linux heartbeatファイル比較の不安定な履歴は各main配布記録に残しています。
