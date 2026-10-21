# Xpotato Tunnel 共通 GUI（開発中）

Tauri の薄いデスクトップ UI と、同梱 Go core の専用 pipe bridge です。
Go の Tunnel core は維持し、WebView へ任意 shell・filesystem・network API を公開しません。
設定のレビュー・保存同意・OS 資格情報ストア・runtime transaction は Go が担当します。

現在の差分は build/test/download の保留中に作成したものです。未ビルド・未テストで、配布用完成品ではありません。
Cargo.lock、GUI の各 OS 配布への接続、依存ライセンス・対応 source、native build と GUI fixture は後続作業です。
macOS/ARM の Go backend も、GUI のコンパイルだけで対応済みとは扱いません。

構成:

- `ui/`: 同梱 HTML/CSS/JS。browser storage・外部資料・analytics を使用しない。
- `src-tauri/`: 固定名・固定 SHA の Go sidecar 起動と、限定コマンド・フォルダー選択だけの Rust wrapper。
- `../cmd/xpotato-tunnel/gui_bridge.go`: 最大64KiBの frame、操作/prompt ID、同意・キー入力、取消と owned operation の後片付け。
- `../docs/gui-lifecycle.md`: OS ごとの配布・更新・削除の責務、同意境界と検証条件。

Go sidecar は対象 CPU/OS と一致する `src-tauri/binaries/xpotato-tunnel-TARGET_TRIPLE[.exe]` を build 時に用意し、
Tauri の externalBin として同梱します。Rust build はその SHA を埋め込み、起動前に確認します。
利用者の既存端末への導入、署名鍵作成・使用設定、権限変更、Release 公開はこの開発作業では行いません。
