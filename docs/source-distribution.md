# Windows/Linux の段階公開と対応 source

公開対象の候補は Windows x64 と Ubuntu 24.04 x64、Mac は build/test のみの保留対象です。
この方針は `packaging/source-profiles.json` の１つの profile に宣言します。
Go の setup/update/connect、GUI、Mac adapter を複製・削除せず、同じ commit の共通実装を使います。
CI は PR/手動実行の検証と期限付き候補生成だけで、merge・Release・署名・自動公開をしません。

## binding 実装を含めない仕組み

`cargo vendor` の全出力には、Windows/Linux で使わない Apple SDK 由来の生成 binding も含まれます。
DMG を除くだけでは source から除外できません。また、Cargo は他 target の依存宣言も解決するため、crate のディレクトリを丸ごと消すと offline/locked build が壊れます。

単一の source 生成処理は、固定版・固定取得 SHA の `cargo-vendor-filterer` を使い、宣言した２ target の実装依存を保持します。
対象外 crate は Cargo.toml と解決用 checksum、プロジェクトが生成した `compile_error!` だけの stub にします。
**objc2 等の名前・feature 宣言は lock/manifest に残りますが、SDK 由来 binding 本体は残しません。**
stub は実装ではなく、対応 target の依存として使われると必ずコンパイルが失敗します。
第三者の元ソースと偽って扱わず、`SOURCE-FILTERING.json` に実装保持と解決専用 stub を別々に列挙します。

生成時に以下を検査します。

- 元の Cargo.lock を変更しない。実際に使う package/version の集合が filtering 前後で同じ。
- Windows/Linux の到達可能な依存グラフに Apple binding や stub がない。
- binding の実装が残っていない。stub のファイルは厳密に３つで、追加 SDK/header/source を拒否する。
- 保持する crate は全 source と license/NOTICE、registry pin とファイル checksum を維持する。
- 対応 source の全ファイル inventory/hash と AGPL 本文・帰属・build script を保持する。

上流: [Cargo の target 別 vendor の制約](https://github.com/rust-lang/cargo/issues/7058)、
[CoreOS vendor-filterer](https://github.com/coreos/cargo-vendor-filterer/tree/v0.5.16)。
[objc2 上流の留保](https://github.com/madsmtm/objc2/blob/7b1abfd750a2cacaea71d6a56ecfb83cb7de560b/LICENSE.md)を「再配布禁止」と断定したものではありません。
留保の確認前にその生成 binding を候補 source へ含めないための梱包方針です。

## 維持者の生成手順

clean な対象 commit、Go 1.27.2、Rust 1.94.1、Python 3.12+ を用意します。取得と vendor 生成にはネットワークが必要です。
CI の共通 source job は Linux 上で１回生成し、同じ archive を Windows/Linux 両方の再構築検査へ渡します。

```sh
python scripts/source-vendor.py --install-tool dist/vendor-tool
python scripts/package-source.py --profile windows-linux \
  --vendor-filterer "$PWD/dist/vendor-tool/bin/cargo-vendor-filterer" --output dist/source
```

Windows で維持者が生成する場合は tool 名に `.exe` が付きます。長いファイル名は明示的な Windows file API のパス形式で扱い、OS の long-path policy は変更しません。
以前の `--rust-vendor` による任意の全 vendor tree の取り込みは受け付けません。
Go core のみの従来 source は `--profile core`（既定）で作れますが、GUI 配布の対応 source にはこの profile を使いません。

成果物の名前は core の source 案内 URL と合わせ `xpotato-tunnel-VERSION-source.tar.gz` を維持します。
`SOURCE-PROVENANCE.json` に profile/対象/保留 target/commit/lock SHA、`SOURCE-FILTERING.json` に実装と stub、
`SOURCE-THIRD-PARTY-NOTICES.txt` に実際の依存の全文通知、`SOURCE-SHA256SUMS` に全 inventory が入ります。
一般公開時は Windows/Linux バイナリと同じ Release・取得経路に対応 source と checksum を置き、認証不要の取得も確認します。今回は公開しません。

## 空の依存 cache からの offline 再構築

対象 OS の native Go 1.27.2/Rust 1.94.1 と compiler/linker・OS library・SDK を事前に用意します。
Windows は MSVC/Windows SDK、Ubuntu は GUI README の GTK/WebKitGTK 等が必要です。これらの一般ツール/OS 部品自体は source bundle に含めません。
新しい directory へ source archive を展開し、その root で実行します。

```sh
# Windows x64: --target x86_64-pc-windows-msvc
python scripts/rebuild-source.py --target x86_64-unknown-linux-gnu --output ../source-rebuild
```

再構築 script は source の変更を拒否し、別の空 CARGO_HOME/GOMODCACHE、Cargo `--frozen`、Go `-mod=vendor`/GOPROXY=off/GOSUMDB=off を使います。
Go core と GUI の build/test、commit/version/sidecar digest を確認し、`SOURCE-REBUILD-RESULT.json` を出力します。
再実行時は新しく展開した source と別の出力 directory を使ってください。fork は実際の変更と source の帰属を示して build します。
Mac/別 target をこの source profile で認証済みと扱わず、script は先に拒否します。

この offline は **アプリ本体の source 再構築**の意味です。初回 setup/repair が固定 Node/runtime を別取得することとは区別します。
この検査は配布生成・開発用であり、一般利用者の導入/更新へ全 source hash 検査を追加するものではありません。

Mac の共通コードと native CI は維持しますが、この段階の workflow は Mac の DMG・全 platform vendor source を候補 artifact にしません。
旧 head の unfiltered source/Mac review artifact は旧設計の証拠で、今回の段階公開には使いません。

## Windows にも展開できるファイル名

`static_vcruntime 3.0.0` の上流 source には、内容が同一の `README.md` と `Readme.md` が含まれます。
Linux でそのまま梱包すると Windows の通常の filesystem では同じ名前として扱われ、inventory が一致しません。
生成処理は manifest が指定する表記へ README の同一内容を一度だけ保持し、元の別名・SHA を
`SOURCE-FILTERING.json` の `identicalReadmeAliases` に記録します。実装・license の内容と registry pin は変更しません。
対応する vendor file checksum は更新します。内容が異なる README、その他の file/directory の大小文字衝突は拒否します。
これは配布 source の可搬性処理であり、Windows の filesystem/権限設定は変更しません。