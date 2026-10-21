# 各 OS の導入・更新・削除（設計、未対応範囲を含む）

推奨は **Go の共通 setup backend + 薄い共通 GUI + OS 別の外装** です。
通常ユーザーに CLI だけを入口として求めません。Go の Tunnel/core を Rust へ移しません。
Windows の NSIS は配置・登録・削除の外装であり、Linux/macOS に exe を配る設計ではありません。

## UI と配布を分ける

共通 GUI は「導入状態 → 許可範囲の確認 → 本人の同意 → 非表示キー入力 → 導入進捗 → 完了」の順にし、
以後は起動・停止・更新・修復・診断・削除の入口を揃えます。停止操作の対象はその GUI が起動した所有プロセスだけです。
既存プロセスを検出しても更新や修復のため勝手に停止しません。全件診断は明示操作にします。

この共通 GUI には **Tauri を第一候補** とします。OSごとの同意画面を3種類自作する負担を減らせるためです。
Rust は UI と限定 IPC の外側だけで、Go backend を同梱 sidecar として呼びます。
Windows WebView2、Linux WebKitGTK と native libraries、macOS build toolchain の扱いは追加の保守になります。
Go GUI の Fyne/Gio/Wails 等も選択肢ですが、描画・WebView の OS 依存や配布はなくなりません。
この PR に Tauri runtime/依存導入や未検証 GUI の完成宣言は含めません。

GUI/backend 間は任意 shell 実行を公開せず、版付きの限定メッセージと既存 owner/scope 検査を使う設計にします。
秘密値を argv、環境ファイル、永続ログ、UIの永続ストレージへ流しません。GUI を開いたことを同意済み扱いにしません。
キー入力の IPC とメモリ消去、同意済み scope の結び付け、キャンセル時の保存順は実装前のレビュー項目です。
ブラウザーからアクセス可能な localhost API、新アカウント、常駐認証、OS権限の自動拡大は導入しません。

## 配布形式と権限

| OS | 通常入口の候補 | 配置・更新・削除 | 署名・依存・保守上の残件 |
| --- | --- | --- | --- |
| Windows | 共通GUIを含む NSIS setup.exe | ユーザー領域、HKCU登録。明示更新で新しい版を配置、旧実行世代は保持。アプリ一覧から削除、データは保持 | 現PRは NSIS + ターミナル保守メニューまで。共通GUI、WebView2同梱/取得方針、コード署名は別途検証 |
| macOS | `.dmg` 内の `.app` | 個人用 `~/Applications` への配置を基本案とし、GUIから明示更新。削除はアプリ削除＋GUIのデータ保持案内 | Developer ID署名/notarization、Keychain、Mac上のbuild/実機、Intel/Apple Silicon双方のruntime確認が未対応 |
| Linux | 対象distroを絞った `.deb` を基準候補、非管理者向け AppImage を比較 | `.deb` は既存package manager GUIで導入/更新/削除し管理者承認が必要。backend実行とプロファイルは通常ユーザー領域 | WebKitGTK/ABI/desktop統合、署名済み配布経路、対応distroの確定が必要 |

macOS `.pkg` は全ユーザー配置などで必要になるまで作りません。DMG は配送容器、実行するものは `.app` です。
Universal2 は sidecar と Node/Tunnel の両archが揃い検証できるまで宣言せず、最初は arch 別候補を検討します。
署名鍵の発行・Keychain登録・利用設定・notarization提出は本人の別の判断なしには行いません。

Linux は `.deb` / `.rpm` / AppImage を機械的に全生成しません。まず利用するdistro/desktop/archを対象に選びます。
AppImage はユーザー領域の一ファイルにできますが、実行権限、FUSE/host libraries、desktop連携の確認が必要です。
「全Linuxで依存導入・管理者承認なし」とは案内しません。非管理者が必須なら、対象OSで AppImage の実機検証を先に行います。
`.rpm` は RPM 系distroを対象に含める場合に追加し、配布方式ごとに別の自動更新器を増やしません。

## ソースで確認できた現在の範囲

- `internal/managed/runtime.go`: runtime manifest は Windows/Linux **amd64 のみ**。macOS/ARM64 は固定URL・SHA・実行ファイルpinが未実装。
- `internal/platform/native_other.go`: Windows/Linux以外の Identity/Vault は `ErrUnavailable` を返す。
- `scripts/package-review.py` と CI: Windows/Linux amd64 のみ。macOS実機、署名資格情報、notarization結果は提供されていない。
- この PR の保守メニューはターミナル UI。一般ユーザー向け共通 GUI の完成版ではない。

従って `GOOS=darwin` の build 追加だけで macOS 対応とは扱いません。
現在実装・検証するのは既存 Windows/Linux backend の通常全走査除去、Windows NSIS 候補と配布整理、
共通GUIとOS別 lifecycle の設計までです。macOS/ARM64の実装と実機検証、共通GUIは残件として追跡します。
Go backend の検査境界・修復 transaction は [installer-lifecycle](installer-lifecycle.md) に記録しています。

一次資料: [Tauri Windows](https://v2.tauri.app/distribute/windows-installer/)、
[Tauri prerequisites](https://v2.tauri.app/start/prerequisites/)、
[macOS .app](https://v2.tauri.app/distribute/macos-application-bundle/)、
[Debian](https://v2.tauri.app/distribute/debian/)、[AppImage](https://v2.tauri.app/distribute/appimage/)。
配布形式の優先順と共通GUI採用は、このプロジェクトの保守負担を踏まえた設計判断です。
