# ライセンス・クレジット・対応ソース

プロジェクトの出自・保守表示は **Xpotato.net / Xpotato1024** です。今回整備した
プロジェクト固有のコードは **AGPL-3.0-only**（GNU Affero General Public License
version 3 のみ）です。[LICENSE](../LICENSE) が正式全文、[COPYRIGHT](../COPYRIGHT)
が帰属方針です。後の版への包括許可、商用禁止、広告掲載義務は追加していません。
商用利用も許諾条件を満たせば可能です。各著作権は各権利者に残り、保守者表示を
第三者の権利の付け替えに使いません。過去の Release の条件を遡って変更しません。

## 確認する入口

`xpotato-tunnel version` は従来どおり版番号のみです。`about` はクレジット・条件の
概要、`license` はその表示と AGPL 全文、`source` は版・commit・改変状態・対応ソース
URL を JSON で返します。いずれも Setup・Vault・ネット接続を必要としません。
通常の MCP stdout へ常時バナーは出しません。MCP の initialize の instructions と
tools/list に案内を含め、読み取り専用の `xpotato_tunnel_source` ツールから同じ公開情報を
取得できます。このツールはファイル・API キー・設定・ネットワークにアクセスしません。
クライアントが instructions やツール説明を非表示にする場合、提供者は利用者に見える
画面等へ対応ソースの案内を追加してください。表示経路だけで遵守完了とは判断しません。

## 配布とネットワーク利用

バイナリを未改変のまま再配布する場合も、AGPL 第6条等に従い対応ソースへのアクセスを
提供する必要があります。この配布では同じ Release へ
`xpotato-tunnel-VERSION-source.tar.gz` とその SHA256 を、バイナリと同じ無償の取得経路で
置きます。GitHub 自動生成ソースだけをビルド依存同梱の代わりにしません。配布前に
`source` が示す実際の commit と source bundle が認証不要で到達可能か確認してください。

改変版をネット経由で利用者と対話させる場合は第13条の対応ソース取得の明確な申し出も
必要です。変更者・変更内容・日付を記録し、fork の公開ソースと配布に案内を更新します。
未改変版の利用だけについて第13条が一律に公開を要求する、とする説明はしません。
外部の別プログラムや全利用者サイトをクレジット挿入だけで AGPL 化するものではありません。
適用範囲は実際の改変・結合・配布形態によります。無保証を含む正式な条件は LICENSE を
確認してください。派生版は元の表示を保持しつつ独自の変更を明示してください。

## コンポーネントの境界

| 対象 | 固定版・結合方法 | この core 配布での扱い |
| --- | --- | --- |
| Go toolchain / 標準ライブラリ | Go 1.27.2、標準コードを実行ファイルにリンク | BSD-3-Clause / PATENTS を通知。コンパイラは同梱しない |
| x/sys / x/term / godbus/dbus/v5 | v0.49.0 / v0.47.0 / v5.2.2、Go link（dbus は Linux） | 元の BSD 条件・帰属を維持。対応ソース bundle の vendor に含める |
| 当プロジェクトの stdio / activity / licensing adapter | core に embed、準備した Node プロセスで実行 | AGPL-3.0-only。MCP SDK の公開 handler API を利用 |
| Desktop Commander | `@wonderwhy-er/desktop-commander@0.2.52`、npm integrity を package-lock に固定 | Setup が registry から別取得。MIT。Chrome の起動前ダウンロード呼出を抑止するローカル改変あり。元の MIT 表示を保持 |
| Commander の推移依存 | 同梱 package-lock の正確な各 package/version/resolved/integrity | core ZIP/source bundle に依存実体を含めない。Node と同じ別プロセス内で動作。各 MIT/ISC/BSD/Apache/LGPL 等の条件を維持 |
| Node | 24.14.0、固定 URL・SHA256、別取得・別プロセス | Node の MIT と同梱第三者条件を維持 |
| OpenAI tunnel-client-runtime | 0.0.16、固定 URL・SHA256、別取得・別プロセス、MCP stdio を接続 | 本体 Apache-2.0、NOTICE と同梱依存の条件を維持 |
| Linux の glibc / loader | 利用先 OS の共有ライブラリ | core に同梱しない。完全静的バイナリとは案内しない |

npm の版は公開 npm tarball の integrity で識別しており、検証していない Git commit を
その配布物の出自として断定しません。Desktop Commander の取得物と依存のライセンスは
runtime 内の node_modules に残します。[THIRD-PARTY-NOTICES.txt](../THIRD-PARTY-NOTICES.txt)
は core に実際に含むコードとローカル改変の通知です。655件の lock エントリは optional な
他 OS 向けを含み、その全部が各端末へ導入されるわけではありません。

`buffers@0.1.1` の npm tarball には license 宣言・全文がありません。原著者の
[2012年のコミット](https://github.com/bitpay/node-buffers/commit/1b745ee35d33eb166e15ef1866073a07c6d7de87)
は直接親が 0.1.1 への版上げで、README と package.json に MIT/X11 を宣言します。
npm に含まれる JavaScript 全4ファイルはこのコミットと SHA256 で完全一致しました。
[ファイル別の照合記録](buffers-license-evidence.json)も参照できます。
後世 HEAD の許諾を旧版へ無条件に遡及した判断ではありません。ただし、そのコミットにも
許諾全文・著作権文言はなく、それらを創作しません。依存経路は Commander → exceljs 4.4.0
→ unzipper 0.10.14 → binary 0.3.0 → buffers 0.1.1 です。

自作 repository の公開、依存実体を含まない core 配布、registry から利用者が取得する
runtime、runtime 自体の再配布は区別します。今回のライセンス選択で第三者条件を一括で
AGPL に上書きしません。runtime の事前同梱・再配布は今回の成果物に含めず、行う場合は
各許諾全文・著作権・LGPL 対応ソースや再リンク等の条件を別途確認して用意します。
依存を更新・除去する場合も override 一行だけで済ませず、Commander の機能と推移依存の
互換性・lock・実際の配布内容を検証してください。

## 対応ソースを作る（公開・tag push は行わない）

clean な対象 commit・Go 1.27.2・Python 3.12 以上で、`python scripts/package-source.py --output dist/source`
を実行します。Git 追跡対象と `go mod vendor` による実際の Go build 依存・元の通知を
まとめ、provenance と各ファイル SHA256 を加えます。秘密設定や runtime の取得済み実体は
含めません。一般用コンパイラ・OS library は含めません。生成物は二度作成して一致を検証します。

展開した source bundle では Go 1.27.2 を使用し、provenance に記載の commit を使って
以下のようにビルドします（例はシェル。Windows では出力名に `.exe` を付けます）。

```sh
go build -mod=vendor -buildvcs=false -trimpath \
  -ldflags "-X github.com/Xpotato1024/xpotato-mcp-tunnel/internal/licensing.BuildCommit=COMMIT" \
  -o xpotato-tunnel ./cmd/xpotato-tunnel
./xpotato-tunnel source
```

`COMMIT` は SOURCE-PROVENANCE.json の値へ置き換えます。fork・改変版は実際のソースの
commit を使い、同じ `-X` で
`github.com/Xpotato1024/xpotato-mcp-tunnel/internal/licensing.SourceRepository=https://github.com/OWNER/REPO`
も指定し、その版の source bundle を提供します。dirty / VCS不明の開発ビルドは正確な公開
ソースを保証せず、その旨を表示します。クリーンなビルドでも URL の公開到達性は別途検証が必要です。

## GUI の依存と対応 source

GUI は Rust 1.94.1 と Cargo.lock に固定する。配布候補の `licenses/GUI-THIRD-PARTY-LICENSES.txt` に各 crate の完全な LICENSE/COPYING/NOTICE を含める。
crate が本文を欠く場合だけ `gui/licenses/overrides.json` の crate 名・版・source commit・本文 SHA を照合する。selectors の MPL 本文は Mozilla 公式標準テキストを使用する。

CI は `cargo vendor --locked --manifest-path gui/src-tauri/Cargo.toml dist/gui-vendor` を実行し、
`python scripts/package-source.py --rust-vendor dist/gui-vendor --output dist/source` で Go/Rust 依存を含む対応 source を作る。
OS WebView / SDK / Rust / Go toolchain 自体はこの source bundle に含めない。
展開 source では前節の Go build に続き、対象 native triple の `gui/src-tauri/binaries/xpotato-tunnel-TARGET[.exe]` へ core をコピーする。
provenance の commit を `XPOTATO_BUILD_COMMIT`、`XPOTATO_BUILD_CLEAN=1` に設定し、
`cargo build --offline --locked --release --manifest-path gui/src-tauri/Cargo.toml` を実行する。
同梱 .cargo/config.toml が vendored crates を使う。Mac/Linux の native prerequisites は GUI README を参照。
利用者の秘密値やプロファイル、取得済み runtime を対応 source に含めない。
