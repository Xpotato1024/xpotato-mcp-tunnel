# Xpotato Tunnel 共通 GUI（v0.2.11、Mac公開保留）

Tauri 2 の薄い UI と既存 Go core の限定 pipe bridge です。
導入・更新・修復・接続・所有接続の停止を同じ画面で扱います。
配布形式・利用手順・権限・検証境界は [GUI lifecycle](../docs/gui-lifecycle.md) を参照してください。

- `ui/`: 同梱 HTML/CSS/JS。外部通信や永続ストレージを使わない。
- `src-tauri/`: 固定名と build 時 SHA で照合する Go sidecar、6つの限定 IPC、native folder picker。
- `licenses/`: crate が本文を欠く場合の固定 source commit に結び付けた本文と証跡。
- `../cmd/xpotato-tunnel/gui_bridge.go`: 同意・キー入力・ID 照合・所有操作の取消。

## 開発者用ビルド

Go 1.27.3、Rust 1.94.1、Python 3.12+ と各 OS の Tauri build prerequisites が必要です。
利用者に Go/Rust/Node/npm の事前準備を求めるものではありません。
clean な commit から native core を作り、`scripts/prepare-gui.py --core PATH --target TARGET_TRIPLE` で同梱 sidecar を用意します。
`XPOTATO_BUILD_COMMIT` にその commit、`XPOTATO_BUILD_CLEAN=1` を設定して次を実行します。

```sh
cargo test --locked --manifest-path gui/src-tauri/Cargo.toml
cargo build --locked --release --manifest-path gui/src-tauri/Cargo.toml
python scripts/gui-notices.py --target TARGET_TRIPLE --output dist/gui-notices.txt
```

`--build-info` は GUI/Vault/backend を起動せず provenance だけを出力します。
`scripts/package-gui.py` が core/GUI の commit・版・SHA 一致を確認して配布候補を作ります。
Windows は既存 NSIS、Mac は arch 別 .app/DMG、Ubuntu 24.04 x64 は .deb です。
CI は PR/手動実行の共通 pipeline で候補 artifact を生成します。公開は別の明示操作で、Release・署名・自動配布を行いません。
Windows の native 結合試験は同じ UI/IPC/Go を使う非配布の `native-fixture` build です。昇格した hosted runner では WebView2 の環境変数 override が無視されるため、専用 build だけ API へ CDP/profile を渡します。通常版の DevTools は無効のままで、fixture の target/profile は配布対象から外し、両 packager が `nativeFixture:true` または marker 欠落を拒否します。
実 Laptop 導入、署名資格情報の準備、OS 権限設定は開発ビルドに含めません。

## 段階公開の対象

配布対象は Windows x64 / Ubuntu 24.04 x64 です。Mac は同じ共通実装の native build/test を維持し、公開候補の梱包/upload を保留します。
単一 target profile から SDK binding 実装を除いた対応 source を生成し、両 native host で offline 再構築します。詳細は [source distribution](../docs/source-distribution.md) を参照してください。
