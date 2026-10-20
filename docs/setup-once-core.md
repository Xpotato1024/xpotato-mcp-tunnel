# Go コア: Setup と起動

現行入口は `cmd/xpotato-tunnel` のGoコアです。PS/cmd/shは保存済みexeへ引数を渡し、Desktop Commanderだけに固定Nodeを使います。旧Node CLIと専用テスト/npm入口はGit追跡コードから撤去しました。既存core/shim更新互換と旧版保持は維持しています。まず [導入・更新の入口](quickstart.md) を参照してください。

正式配布は[Release v0.2.10](https://github.com/Xpotato1024/xpotato-mcp-tunnel/releases/tag/v0.2.10)です。[表示仕様](runtime-ux.md)、[非秘密.env入力](setup-env.md)、[確認・診断](operations.md)を参照してください。インストーラの追加はありません。

## 本人による導入

ReleaseのWindows ZIPは一度の展開でexeとガイドが出ます。LinuxはReleaseのtar.gzを直接展開し、実行権限を保持します。ActionsレビューartifactだけはLinuxの外側にZIPがあります。旧0.2.7の二重包装を含む入手先・展開・checksum/version確認は[quickstart](quickstart.md)にまとめています。CIは実際に展開・全checksum確認・version実行まで検証します。

Windows／Linux CIは、実行ファイル・SHA256SUMS・build-info・手順をレビュー用artifactとRelease候補にまとめます。通常利用は上記Releaseを取得し、checksumを確認して通常ユーザーで実行します。本人の端末へGo SDKを導入する必要はありません。アカウント上のプラグイン登録は本人が行います。

```powershell
# Windows: 本人の通常権限の対話端末で実行する。
.\xpotato-tunnel.exe setup

# 短いコマンドの保存に同意し、保存先が既存 PATH にある場合。
xpotato-tunnel
# PATH にない場合も短いユーザー用パスで起動できる。
& "$HOME\.local\bin\xpotato-tunnel.cmd"
```

```sh
# Linux: ログインユーザーの Secret Service が起動・解除されていること。
./xpotato-tunnel setup
# ~/.local/bin が既存 PATH にある場合。
xpotato-tunnel
# PATH にない場合。
~/.local/bin/xpotato-tunnel
```

Linux で絶対パスの `XDG_DATA_HOME` を設定している場合、保存先はその配下です。両 OS とも `--home ABSOLUTE_DIR` で保存先を指定できます。Setup は本人の別の同意で `~/.local/bin` に短いコマンドを置けます。`--command-dir ABSOLUTE_DIR` で別の既存ユーザー用 bin も選べます。PATH、PowerShell 実行ポリシー、シェル設定、サービスは変更しません。保存先の判断と短いコマンドの同意は記録し、再 Setup で再質問しません。既存の他のコマンドや利用者が変更したラッパーは上書きしません。

Setup に旧 launcher の停止確認・自動検出はありません。通常の同一保存先内の端末単位・Tunnel ID 単位の OS 起動ロックを維持します。別保存先・別ホスト・外部クライアントはこのロックの対象外です。

## 日本語ウィザードの進め方

1. 最初に必要な情報と恒久保存先を確認します。Windows の既定保存先は `%USERPROFILE%\AppData\Local\xpotato-mcp-tunnel-v2`、Linux は `$XDG_DATA_HOME/xpotato-mcp-tunnel-v2`（絶対パスが設定されている場合）、それ以外は `~/.local/share/xpotato-mcp-tunnel-v2` です。本体は `core/<version>`、固定 runtime は `releases`、起動ラッパーは `bin`、端末別設定は `devices/<端末名>` に保存します。
2. 端末名を入力します。例は `work-pc`。英小文字・数字・ハイフンを使います。
3. Platform の Tunnel 設定から `tunnel_` で始まる Tunnel ID をコピーします。API キーとは別物です。この通常入力欄にキーを貼り付けないでください。
4. 許可フォルダーを `add` で一つずつ追加します。例は Windows `C:\Work`、Linux `/home/user/Work`。既存の絶対パスが必要です。空白を含むパスも入力でき、外側の引用符は不要です。`remove` と一覧の番号で削除、`review` で確認、`done` で確定します。空の一覧や無効なパスは確定できず、再入力できます。
5. 保存前の確認画面で端末名、Tunnel ID、許可範囲、Commander の初期設定、保存先を確認します。`device`、`tunnel`、`dirs`、`home`、`command` で修正し、`yes` で保存処理へ進みます。既存端末は同じ端末名で再開・更新でき、変更前の実際の範囲と変更後の一覧を確認してから保存します。対話の `dirs` を変更しない再開では既存 profile の絞り込みも保持します。短縮コマンド先が既に選択済みの場合は、違う指定を適用せず、実際の保存先を確認画面に表示します。既存の配置辞退も維持します。
6. 固定 runtime の導入後、本人の OS 資格情報ストアの専用一レコードへの保存に別途同意します。同意した場合にだけ runtime API キーを非表示で入力します。キーは引数・設定 JSON・確認画面・ログに出しません。その後、任意の短縮コマンドの保存にも別の同意を求めます。

通常入力中は `q` / `cancel` / `中断`、確認画面では `no`、非表示キー入力中は Ctrl+C で中断できます。最終確認前はファイルも資格情報も書きません。最終確認後に中断・失敗した場合は途中の設定を保持し、同じ `setup` で再開します。完了済みのキー保存は繰り返しません。OS 保管庫への保存同意に `no` と答えた場合も同様です。

旧コア版の runtime が選択されている既存端末では、確認画面にこのコアの固定 runtime への更新計画を表示します。`yes` 後に新 runtime を導入・検証し、本体／shim の導入に成功してから参照を切り替え、旧 runtime と設定を保持します。稼働中の接続は再起動しません。保存前の中断、新 runtime の検証失敗、本体／shim の導入失敗では旧参照を維持します。現在版の runtime 改変による検証失敗を、この更新分岐で暗黙に採用することはありません。同じ保存先に複数の旧版端末設定がある場合、共有コマンドを更新した後も、他の端末では `setup --device その端末名` で確認付き更新を行ってから新コアで起動します。旧コア本体は保持します。

`--device`、`--tunnel-id`、`--allow` の指定で入力値を事前設定できます。`--allow` は繰り返し指定でき、対話の追加・削除・確認も利用できます。API キーを受け取る引数はありません。新規 Commander の初期値は Windows `powershell.exe`／Linux `/bin/sh`、読み取り上限 100 行、書き込み上限 50 行、telemetry 無効です。禁止コマンドは `shutdown, reboot, format, diskpart, netsh, runas, takeown, reg, sc` です。

資格情報の保存先はこのアプリ・所有者・保存場所・端末・Tunnel に対応する一つのレコードです。エージェントによる実際の保存や取得は実施していません。

## OS ごとの資格情報

| OS | バックエンド | 起動条件 |
| --- | --- | --- |
| Windows amd64 | Windows Credential Manager のユーザー Generic credential、logon 間で保持 | Setup を実行した通常ユーザー。管理者トークンを拒否 |
| Linux amd64 | ユーザー Unix session D-Bus 上の Secret Service、default collection | 同じ UID、非 root、既存 collection が解除済み |

Linux ではキーリングの作成、サービスの自動起動、Unlock/Prompt 呼び出しを行いません。未導入、セッションなし、ロック中、レコードなしを区別して停止し、平文ファイルにフォールバックしません。鍵の転送は Secret Service の `plain` session をユーザー Unix IPC 上で使います。headless 環境で無条件に無人起動できる設計ではありません。Windows の本物の Vault と Linux デスクトップの本物のキーリングは、本人の受け入れ確認が必要です。

Start は入力を要求しません。先に本人・非昇格・Setup 完了・許可範囲・runtime 整合性を確認し、二重起動ロックを取ってからキーを取得します。Tunnel 子プロセスには必要な OS 環境だけを渡し、接続先を `https://api.openai.com` に明示固定します。継承された Tunnel/MCP 設定・接続先・認証・TLS・proxy 上書きは使わず、設定検索先と作業場所も新しい隔離 run directory にします。明示的な proxy/mTLS 対応は別途設計が必要です。

キーは Tunnel 子プロセスの環境へ渡し、Desktop Commander の import 前に取り除きます。Go の byte buffer は消去しますが、OS/Go の内部コピーや同じユーザー権限からの完全な秘匿は保証しません。終了時は Windows の専用 process group に CTRL_BREAK、Linux の専用 process group に SIGTERM を送り、最大 5 秒待った後に、この起動だけの Windows Job Object／Linux process group を回収します。Windows は launcher 強制終了時も Job Object が子孫を終了させます。Linux で意図的に別 session/group へ脱出したプロセスは回収対象外です。

Linux の launcher が SIGKILL や SIGHUP で終了した場合、別 process group の専用 supervisor が private pipe の EOF を検出し、起動した runtime group を終了します。supervisor は取得済み flock の記述を保持し、子孫の終了と reap が済むまで二重起動を防ぎます。終了不能な owned group が残る場合もロックを保持して拒否を継続します。意図的な group/session 脱出は既存の対象外です。

Linux の非表示キー入力は Ctrl+C / SIGTERM / SIGHUP を中断として返し、元の端末状態を復元して未確定入力を破棄します。SIGKILL など捕捉不能な終了は端末復元の対象外です。

## 再開・更新・確認

Setup は途中状態を保持します。保存を断ったり、ネットワーク／Vault で失敗した場合、同じ端末名で再実行します。通常の途中再開では完了済みのキー保存を繰り返しません。範囲変更は旧 profile を残して別 profile を作り、他の Commander 設定を保持して確認付きで参照を切り替えます。Tunnel ID 変更は新しい専用キー保存レコードを使い、以前のレコードを保持します。新キー保存後・参照公開前に失敗した変更の再試行では、再度の保存同意と入力が必要な場合があります。公開後の登録案内・既定端末保存失敗は保存済み更新として明示します。詳細は [.env Setup の失敗時](setup-env.md#保存後の変更と失敗時) を参照してください。旧 schema 2 や孤立 profile を自動移行しません。

| コマンド | 動作 |
| --- | --- |
| 引数なし／`start` | default 端末を保存済みキーでフォアグラウンド起動 |
| `doctor` | default 端末がなくても保存先と install lock を確認。`--device` では途中 Setup と runtime 状態も報告。Vault、実接続、アカウント登録を検査しない |
| `credentials` | この端末の一レコードを、本人の新しい保存同意と非表示入力で置換 |
| `update` | 実行したレビュー済みコアと同梱固定 runtime を導入し、同意済み短いコマンドと次回起動用参照を更新 |
| `rollback` | 候補0.2.10では保存済みの前core/runtimeの組へ戻す。現管理coreが全検証できる互換runtimeに限る |
| `plugin` | 登録用情報／指定済み直接 MCP endpoint の bundle を再生成 |

固定依存はNode24.14.0、Tunnel runtime0.0.16、Desktop Commander0.2.52です。`internal/managed/assets/package-lock.json`を埋め込み、runtime内のnpm ci --ignore-scriptsで推移依存を固定します。rootの旧npm入口とは別です。ripgrepとOS別binaryもlockに含み、Chrome自動取得抑止は保持します。

更新は、取得したレビュー済み新パッケージの `./xpotato-tunnel update`（Windows は `.\xpotato-tunnel.exe update`）を本人が明示実行します。稼働中の接続は再起動しません。Startでnpm/Git／パッケージ取得や背景の自動更新は行いません。候補0.2.10は新coreを先に検証し、同じ依存内容のruntimeを全検証して再利用できます。初回の短縮コマンド移行は旧coreへのfallbackを先に保存し、最後にcore/runtime参照を一つのdevice.jsonで切り替えます。失敗・取消・強制終了後は同じupdateで再開します。旧版と設定を保持し、同じcore版の異なるbytesは上書きしません。[候補の設計と制約](update-transactions.md)も参照してください。

初回Setupに前runtimeはありません。rollbackは現コアが認識する保存済みruntimeに限り、任意の旧版へ戻す機能ではありません。

二重起動は同じ保存先の端末単位と Tunnel ID 単位で防止します。maintenance/install/start は OS ファイルロックを使い、中断で OS が解放します。本人の OS 所有権と記録所有者を確認して再利用し、ファイルを削除・置換して別の生存中 holder を邪魔しません。lock の所有者メタデータも atomic rename で保存します。旧形式や他所有者の lock は自動採用しません。

## プラグイン登録の残り

Setup は `devices/<device>/registration/tunnel-registration.json` に同じ Tunnel ID と案内を生成します。既存プラグインがその ID を使っている場合は再利用し、再 Setup で重複作成を要求しません。アカウント状態は **account-registration-not-probed** と記録します。新規登録だけが必要な場合、公式手順は ChatGPT Plugins で Add custom MCP server → Tunnel → ID の選択 → 認証・権限の確認 → Create as a plugin です。コードからの account 登録 API は確認できておらず、登録完了や未登録とは判定しません。

本人が別途検証した直接 HTTPS MCP endpoint を `--mcp-url` に指定した場合だけ、AgentPlugins 1.0 の `plugin.json`／`mcp.json` を一つの plugin root に入れた ZIP も生成します。Tunnel ID から URL を推測しません。キー、userinfo、query を含む URL は拒否します。bundle 生成はアップロードや有効化を意味しません。

この制約により「Setup だけでプラグインまで自動作成」はまだ完全には満たしていません。実装済みなのはローカル Setup、保存済みキーによる一コマンド起動、登録情報／任意の直接接続 bundle 生成です。

## 開発検証

```text
go test -count=1 ./...
go vet ./...
go build -trimpath ./cmd/xpotato-tunnel
node --check internal/managed/assets/stdio.mjs
node --check internal/managed/assets/activity.mjs
node --test test/activity.test.mjs
```

Go の通常テストは fake Vault、合成キー、一時 profile を使用します。実際の Setup 子プロセスの導入中／キー待ち／コア stage コピーを終了させて再開を検証し、親・孫の終了回収では無関係の sentinel が生存することを確認します。生成 PS/cmd/sh を実行し、空白を含む引数と終了コード転送も検証します。悪意の ambient 設定は、HTTP を行わない runtime 代替 exe の実 argv/env で除去を確認します。

日本語ウィザードのテストは Windows／Linux の実ファイルシステム上で、新規 Setup、複数フォルダーの追加・削除・一覧確認、引用符付きパス、重複・不存在・相対パス・ファイルの拒否、端末名／Tunnel ID／保存先の再入力、最終確認からの修正、中断、既存設定の保持を検証します。確認画面の生成が Vault・runtime・ファイル書き込みを行わないことも確認します。旧 `cutoverReady` フィールドは Start の条件に使いません。端末の通常入力は後続の貼り付け行を失わず、非表示キー入力を先読みしません。

旧コアの途中 Setup 再開は更新計画の明示・確定後の検証・旧 pin 保持を検証し、短縮コマンド先の不一致は資格情報保存前に訂正します。キー保存後の短縮コマンド同意で中断した場合も、保存済み専用レコードを保持して再開し、キーを再入力しないことを検証します。

Linux CI は専用の一時 D-Bus daemon と fake Secret Service を使い、本物の desktop bus を参照しません。両 OS の CI は `XPOTATO_RUNTIME_SMOKE=1` で、公式固定依存の導入・隔離 stdio initialize/tools-list・PATH を空にした実 content search も検証します。キー入力、Tunnel control-plane 接続、実 Vault 操作はありません。

公式資料: [Secure MCP Tunnel](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels)、[Windows CredWriteW](https://learn.microsoft.com/en-us/windows/win32/api/wincred/nf-wincred-credwritew)、[Secret Service](https://specifications.freedesktop.org/secret-service/latest/)。
