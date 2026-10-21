# 共通 GUI と配布の責務（実装中・未公開）

PR #14 の通常検査・runtime transaction を土台に、Go core を維持した Tauri 2 の薄い GUI を追加します。
この文書の存在やクロスコンパイルだけで、macOS/ARM・署名・各 desktop の利用体験を検証済みとは扱いません。
公開・merge・実 Laptop 導入は今回の作業に含みません。

## 利用者の手順

1. OS 用の Installer からアプリを配置して開く。
2. 「初期設定」で端末名・Tunnel ID・許可フォルダー・保存内容を確認する。同意後だけ API キーを非表示入力する。
3. 「接続を開始」。停止は「このアプリの接続を停止」。他の既存接続は操作しない。
4. 更新したい版のアプリを OS の Installer で入れ替え、そのアプリで「この版で更新」。接続中の世代は次の接続まで保持する。
5. 不具合時は簡易診断、必要なら runtime の再取得で修復。全依存の比較は明示的な全依存診断だけ。

GUI を開くだけでは、runtime 取得、キー取得・保存、設定変更、プロセス停止を行いません。
全依存の検査を GUI 内部に隠して必須化しません。取得時 SHA/SRI、安全な展開、７つの起動入口、所有者と scope を Go が確認します。

## 配布と更新・削除

| 対象 | 通常の入口 | アプリ本体の更新・削除 | runtime・データの管理 |
| --- | --- | --- | --- |
| Windows x64 | PR #14 のユーザー単位 NSIS に GUI と Go sidecar を同梱 | 同じ NSIS の段階配置、HKCU、Start menu、アプリ一覧。既存プロセスを強制終了しない | GUI から Go の明示操作。データ・キー・旧世代は保持 |
| macOS Intel/Apple Silicon | arch ごとの .app を入れた DMG | 個人用 ~/Applications を基本とする Finder 配置・置換・削除。全ユーザー pkg は採用しない | 共通 GUI/Go。Keychain・排他・子プロセス・runtime pin が揃うまでは未対応表示 |
| Linux | 対象を Ubuntu 24.04 系 desktop の .deb から始める | OS の package manager GUI に入れ替え・削除を任せる。この操作だけ OS の管理者承認が必要 | backend は通常ユーザーで動き、既存 Secret Service を使用。root 実行・サービス登録をしない |

Linux はまず .deb に限定し、RPM/FUSE/AppImage と別々の自動更新器を同時に保守しません。
非管理者配布が必要になった場合に対象 distro の AppImage を追加評価します。
アプリを削除する前の「削除の準備」は、所有者・記録 SHA の一致する短縮コマンドだけを取り除きます。
プロファイル・資格情報・runtime の一括削除、OS の権限設定、AV 除外、PATH・サービス・自動起動は追加しません。

## IPC と同意の境界

GUI の Rust 部分は、隣接する固定名 Go sidecar の build 時 SHA256 を確認し、固定の `_gui-bridge` だけを起動します。
PATH 検索・利用者指定プログラム・任意 shell・localhost server・新規アカウント・永続認証トークンを使いません。
Tauri の capability は同梱 main window の６つの限定コマンドだけです。shell/fs/network/updater API を公開しません。
UI 資料は同梱 HTML/CSS/JS のみ、CSP を設定し、非秘密値も textContent で表示します。WebView に storage/analytics を追加しません。

専用の inherited stdin/stdout pipe に最大64KiBの JSON frame を流します。操作 ID・prompt ID を照合し、無関係な reply・取消を拒否します。
秘密入力は prompt の応答だけで、argv/env・進捗・エラー・ログへ含めません。Rust/Go の可変バッファは使用後消去します。
WebView/IPC には一時的なコピーがあり、JavaScript/OS の immutable copy を完全消去できる保証はしません。入力欄は送信・中断・終了時に消します。
キーの保存同意と scope の再確認は既存 Go core が要求し、GUI の表示だけで同意済みとは扱いません。

接続と保守の２枠を分け、各枠で１操作だけを許します。保守の取消は接続に波及しません。
Start の runtime stdin とログを bridge pipe から切り離し、Go の既存 owned-process 管理で停止します。
閉じる際は自操作の context を取消し、後片付け完了を待ちます。PID 一覧・外部プロセス検索・強制的な全停止は行いません。
進行中・入力待ち・完了・中断・期限切れ・失敗を区別し、files と bytes の同単位表示は Go の進捗をそのまま用います。

## 検証と未完了項目

- pipe の同意前キー要求、stale reply、任意コマンド、取消 ID、接続と保守の独立、出力破断のテスト。
- GUI の非秘密表示、入力消去、進行・中断・timeout・失敗の状態遷移とキーボード操作。
- 各 OS の native build、同梱 sidecar digest 拒否、Windows の隔離 Installer fixture。
- macOS の Keychain/runtime/lock/process 実装、Intel/Apple Silicon の実行、Linux の対象 desktop、署名/notarization は独立して検証を記録する。
- Rust 依存の lock、NOTICE/ライセンスと対応 source の追加を確認してから配布候補を作る。

2026-10-21: Prime 性能計測のため build/test/download の解除待ち。現時点の GUI 差分は未ビルド・未テストです。
Tauri の採用は共通 UI の保守を１か所にするためで、Tunnel の Go 実装を置き換えるものではありません。

参考: [Tauri sidecar](https://v2.tauri.app/develop/sidecar/)、[capability](https://v2.tauri.app/security/capabilities/)、
[macOS app](https://v2.tauri.app/distribute/macos-application-bundle/)、[Debian](https://v2.tauri.app/distribute/debian/)。
