# OS 別の導入・更新・削除

Go core を維持し、Tauri 2 の共通 GUI と OS 別の配置形式を採用する。
Go の Fyne/Gio/Wails、Rust 単体 GUI、各 OS の専用 UI を比較すると、いずれも描画・native library・署名と配布の保守は残る。
既存 HTML と限定 pipe の共通画面を一か所で保守できる Tauri を選び、任意 shell/fs/network API は公開しない。
Windows の NSIS は配置だけの外装として活用する。

| 対象 | 配布形式と利用者の入口 | アプリ更新・削除 | 権限と制約 |
| --- | --- | --- | --- |
| Windows x64 | NSIS setup.exe → Start menu の共通 GUI | 新版 Installer / OS のアプリ一覧 | ユーザー領域、HKCU。OS 提供 WebView2 Evergreen が必要 |
| macOS Intel / Apple Silicon | CPU 別 DMG 内の .app → 個人用 ~/Applications | 終了後 Finder で置換 / ゴミ箱へ移動 | 個人領域。未署名候補。Developer ID と notarization は別判断 |
| Ubuntu 24.04 desktop x64 | .deb → OS package manager GUI → アプリ一覧 | 次版 .deb / package manager GUI | 配置に OS 管理者承認。backend は通常ユーザー。WebKitGTK 4.1、Secret Service が必要 |

共通 GUI は初期設定、許可範囲と保存同意、非表示キー入力、進捗、完了、接続・所有接続の停止を提供する。
アプリの更新と runtime の適用を区別する。「この版で更新」は同梱 core の固定 runtime を次の接続へ適用する。
保存済み runtime の全件再読は通常経路に置かない。deep 診断は明示操作だけ。
プロフィール、キー、runtime はアプリ削除後も保持する。

## 選ばなかった形式

macOS .pkg は全ユーザー配置が必要になるまで作らない。DMG は配送容器、実行するものは .app。
Universal2 とせず CPU 別の Go、Node、Tunnel、GUI の native 候補を作り検証する。
Linux は .deb を基準とし、.rpm / AppImage / FUSE と複数の自動更新器を同時に保守しない。
非管理者配布が必須となった場合は、対象 distro と desktop を決めて AppImage を再評価する。

## 実装と検証境界

共通 GUI、Go bridge、Windows/Linux amd64 と macOS amd64/arm64 の runtime pin、Mac Keychain/排他/所有プロセス管理を実装する。
各 OS の native build・機能テスト・固定 runtime smoke・候補生成は `Desktop GUI candidates` CI が担当する。
CI の成功は実利用者の Keychain による認証、各 desktop の操作性、署名や notarization の証明ではない。
実キー保存の試験、署名資格情報、Gatekeeper の正規確認、物理 Mac と Linux desktop の確認は独立した残件。
OS の権限・AV・PATH・サービス・自動起動を変更する工程は追加しない。

具体的な利用手順と IPC は [GUI lifecycle](gui-lifecycle.md)、通常検査と修復 transaction は [installer lifecycle](installer-lifecycle.md)。

一次資料: [Tauri prerequisites](https://v2.tauri.app/start/prerequisites/)、[macOS .app](https://v2.tauri.app/distribute/macos-application-bundle/)、[Debian](https://v2.tauri.app/distribute/debian/)。

## 段階公開の対象

現在の配布候補は Windows x64 / Ubuntu 24.04 x64 です。Mac は同じ共通実装の native build/test を維持し、公開候補の梱包/upload を保留します。
単一 target profile から SDK binding 実装を除いた対応 source を生成し、両 native host で offline 再構築します。詳細は [source distribution](source-distribution.md) を参照してください。
