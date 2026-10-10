# Go コア: Setup と起動

0.2.2 の入口は `cmd/xpotato-tunnel` です。Go を選んだ理由は、単体実行ファイルを配布でき、両 OS の資格情報・ファイルロック・子プロセス API を分離できるためです。PS/cmd/sh は保存済みの Go 実行ファイルへ引数を渡すだけです。Desktop Commander の実行だけに固定版 Node を使用します。旧 Node CLI と 30 テストは保持しています。

## 本人による導入

CI の artifact ZIP 内には OS 別の tar.gz が入っています。Linux は `tar -xzf xpotato-tunnel-linux-amd64.tar.gz`、展開先で `sha256sum -c SHA256SUMS` と `./xpotato-tunnel version` を実行します。実行権限は tar に保持され、追加の chmod は不要です。Windows は `tar -xzf .\xpotato-tunnel-windows-amd64.tar.gz` で展開し、`Get-FileHash .\xpotato-tunnel.exe -Algorithm SHA256` を SHA256SUMS と比較します。CI は実際に展開・チェックサム確認・version 実行を検証します。

PR の Windows／Linux CI は、実行ファイル・SHA256SUMS・build-info・手順を private review artifact にまとめます。本人はレビュー済み head に対応する artifact を取得し、チェックサムを確認して通常ユーザーで実行します。本人の端末へ Go SDK を導入する必要はありません。公開リリースやアカウント上のプラグイン登録は実施しません。

```powershell
# Windows: C:\Work は事前に本人が作成した、許可するディレクトリ。
.\xpotato-tunnel.exe setup --device llm-01 --tunnel-id tunnel_YOUR_ID --allow 'C:\Work'

# 短いコマンドの保存に同意し、保存先が既存 PATH にある場合。
xpotato-tunnel
# PATH にない場合も短いユーザー用パスで起動できる。
& "$HOME\.local\bin\xpotato-tunnel.cmd"
```

```sh
# Linux: ログインユーザーの Secret Service が起動・解除されていること。
./xpotato-tunnel setup --device llm-02 --tunnel-id tunnel_YOUR_ID --allow "$HOME/Work"
# ~/.local/bin が既存 PATH にある場合。
xpotato-tunnel
# PATH にない場合。
~/.local/bin/xpotato-tunnel
```

Linux で絶対パスの `XDG_DATA_HOME` を設定している場合、保存先はその配下です。両 OS とも `--home ABSOLUTE_DIR` で保存先を指定できます。Setup は本人の別の同意で `~/.local/bin` に短いコマンドを置けます。`--command-dir ABSOLUTE_DIR` で別の既存ユーザー用 bin も選べます。PATH、PowerShell 実行ポリシー、シェル設定、サービスは変更しません。保存先の判断と短いコマンドの同意は記録し、再 Setup で再質問しません。既存の他のコマンドや利用者が変更したラッパーは上書きしません。

既存 launcher が同じ Tunnel ID で動いているかは、秘密のプロセス引数を読んで調べません。Setup は本人に旧 launcher を停止済みであることの明示確認を求め、未確認の profile は Start をキー取得前に拒否します。これは本人の申告であり、旧プロセスを検査・停止したという意味ではありません。別保存先・別ホスト・任意の外部クライアントは制御できません。

引数を省略して `setup` を実行すると端末名、Tunnel ID、許可ディレクトリを尋ねます。複数ルートは `--allow` を繰り返します。保存同意で `yes` を入力した後に、runtime API キーを非表示入力します。コマンド引数や JSON にキーを入れません。保存先はこのアプリ・所有者・保存場所・端末・Tunnel に対応する一つのレコードです。エージェントによる実際の保存や取得は実施していません。

## OS ごとの資格情報

| OS | バックエンド | 起動条件 |
| --- | --- | --- |
| Windows amd64 | Windows Credential Manager のユーザー Generic credential、logon 間で保持 | Setup を実行した通常ユーザー。管理者トークンを拒否 |
| Linux amd64 | ユーザー Unix session D-Bus 上の Secret Service、default collection | 同じ UID、非 root、既存 collection が解除済み |

Linux ではキーリングの作成、サービスの自動起動、Unlock/Prompt 呼び出しを行いません。未導入、セッションなし、ロック中、レコードなしを区別して停止し、平文ファイルにフォールバックしません。鍵の転送は Secret Service の `plain` session をユーザー Unix IPC 上で使います。headless 環境で無条件に無人起動できる設計ではありません。Windows の本物の Vault と Linux デスクトップの本物のキーリングは、本人の受け入れ確認が必要です。

Start は入力を要求しません。先に本人・非昇格・切替準備・許可範囲・runtime 整合性を確認し、二重起動ロックを取ってからキーを取得します。Tunnel 子プロセスには必要な OS 環境だけを渡し、接続先を `https://api.openai.com` に明示固定します。継承された Tunnel/MCP 設定・接続先・認証・TLS・proxy 上書きは使わず、設定検索先と作業場所も新しい隔離 run directory にします。明示的な proxy/mTLS 対応は別途設計が必要です。

キーは Tunnel 子プロセスの環境へ渡し、Desktop Commander の import 前に取り除きます。Go の byte buffer は消去しますが、OS/Go の内部コピーや同じユーザー権限からの完全な秘匿は保証しません。終了時は Windows の専用 process group に CTRL_BREAK、Linux の専用 process group に SIGTERM を送り、最大 5 秒待った後に、この起動だけの Windows Job Object／Linux process group を回収します。Windows は launcher 強制終了時も Job Object が子孫を終了させます。Linux で意図的に別 session/group へ脱出したプロセスは回収対象外です。

Linux の launcher が SIGKILL や SIGHUP で終了した場合、別 process group の専用 supervisor が private pipe の EOF を検出し、起動した runtime group を終了します。supervisor は取得済み flock の記述を保持し、子孫の終了と reap が済むまで二重起動を防ぎます。終了不能な owned group が残る場合もロックを保持して拒否を継続します。意図的な group/session 脱出は既存の対象外です。

Linux の非表示キー入力は Ctrl+C / SIGTERM / SIGHUP を中断として返し、元の端末状態を復元して未確定入力を破棄します。SIGKILL など捕捉不能な終了は端末復元の対象外です。

## 再開・更新・確認

Setup は途中状態を保持します。保存を断ったり、ネットワーク／Vault で失敗した場合、同じ端末名で再実行します。完了済みのキー保存は繰り返しません。保存と状態書き込みの間で失敗した場合は、次回もう一度同じレコードへの保存同意が必要です。既存 Commander 設定はバイト単位で保持し、許可ルートの縮小は認め、拡大は新しい端末の Setup を要求します。旧 schema 2 や孤立 profile を自動移行しません。

| コマンド | 動作 |
| --- | --- |
| 引数なし／`start` | default 端末を保存済みキーでフォアグラウンド起動 |
| `doctor` | default 端末がなくても保存先と install lock を確認。`--device` では途中 Setup と runtime 状態も報告。Vault、実接続、アカウント登録を検査しない |
| `credentials` | この端末の一レコードを、本人の新しい保存同意と非表示入力で置換 |
| `update` | 実行したレビュー済みコアと同梱固定 runtime を導入し、同意済み短いコマンドと次回起動用参照を更新 |
| `rollback` | 保存済みの前 runtime へ参照を戻す。現コアが認識する版に限る |
| `plugin` | 登録用情報／指定済み直接 MCP endpoint の bundle を再生成 |

今回の固定版は Go コア 0.2.2、Node 24.14.0、Tunnel runtime 0.0.16、Desktop Commander 0.2.52 です。npm はレビュー済み `package-lock.json` の `ci --ignore-scripts` で全推移依存を固定します。ripgrep 1.18.0 と各 OS 用 optional binary package も lock に含み、ホストの rg を使わない機能検索で検証します。Chrome の自動取得は再有効化しません。

更新は、取得したレビュー済み新パッケージの `./xpotato-tunnel update`（Windows は `.\xpotato-tunnel.exe update`）を本人が明示実行します。コア、固定 runtime、同意済みの短いコマンドを切り替え、稼働中の接続は再起動しません。Start で npm/Git／パッケージ取得を行わず、背景の自動更新も行いません。実行ファイルは検証済みの同一ディレクトリ stage から atomic rename し、ラッパーの途中更新は intent を使って同じ update の再実行で再開します。旧版と設定は保持します。同じコア版の異なるバイナリは上書きせず停止します。

現行 0.2.2 に別 runtime 版は同梱していないため、初回 `rollback` には前版がありません。runtime rollback は、そのコアが認識する版に限ります。

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
npm run check
npm test
```

Go の通常テストは fake Vault、合成キー、一時 profile を使用します。実際の Setup 子プロセスの導入中／キー待ち／コア stage コピーを終了させて再開を検証し、親・孫の終了回収では無関係の sentinel が生存することを確認します。生成 PS/cmd/sh を実行し、空白を含む引数と終了コード転送も検証します。悪意の ambient 設定は、HTTP を行わない runtime 代替 exe の実 argv/env で除去を確認します。

Linux CI は専用の一時 D-Bus daemon と fake Secret Service を使い、本物の desktop bus を参照しません。両 OS の CI は `XPOTATO_RUNTIME_SMOKE=1` で、公式固定依存の導入・隔離 stdio initialize/tools-list・PATH を空にした実 content search も検証します。キー入力、Tunnel control-plane 接続、実 Vault 操作はありません。

公式資料: [Secure MCP Tunnel](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels)、[Windows CredWriteW](https://learn.microsoft.com/en-us/windows/win32/api/wincred/nf-wincred-credwritew)、[Secret Service](https://specifications.freedesktop.org/secret-service/latest/)。
