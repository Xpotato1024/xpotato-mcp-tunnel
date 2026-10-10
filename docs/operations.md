# 運用メモ

## 保存場所と更新

既定の保存先は `%USERPROFILE%\AppData\Local\xpotato-mcp-tunnel` です。リポジトリの checkout、npm の cache と独立しています。

```text
xpotato-mcp-tunnel/
  releases/<固定バージョンとmanifest識別子>/   # runtime、stdio wrapper、Commander、receipt
  devices/<device>/device.json               # Tunnel ID と選択された manifest
  devices/<device>/profile/.claude-server-commander/config.json
  devices/<device>/profile/Temp/
  devices/<device>/run-*/                    # health URL、runtime log、pid
  devices/<device>/start.lock                # この CLI の多重起動防止
  tunnel-locks/<Tunnel ID の SHA256>.lock     # 同じ home 内の同一 Tunnel 多重起動防止
```

`setup` の再実行は保存済み `config.json` を書き直しません。本人が Commander の設定を変更した場合も保持します。既存端末と異なる `--tunnel-id` や `--allow` を与えると停止します。新しい接続には別の `--device` を使ってください。許可ルートは setup 時に明示的に承認した範囲をローカルの device.json に記録します。config をその範囲の下へ狭めることはできますが、範囲を超える変更は start/setup/update/doctor が拒否します。範囲を拡大する場合は新しい --device で明示的に setup し、同じ Tunnel の旧接続を終了してから切り替えてください。

setup は現在の Windows トークンを読み取り、非管理者であることを確認して所有者 SID を device.json (schema 2) に保存します。以後の 4 コマンドも非管理者・同じ所有者であることを確認し、runtime はそのトークンを継承します。runas、昇格要求、OS 権限や ACL の変更を行いません。旧 profile の ACL を別 profile へコピーする機能もありません。所有者情報のない schema 1 profile や既存の未確認 config は自動で引き継ぎません。必要なら旧 profile を保存したまま、新しい --device を使ってください。

`update` は新しい release のダウンロード・SHA256 検証・準備が成功した後だけ、device の manifest を原子的に差し替えます。失敗時は選択と config を保持します。旧 release を削除しないため、実行中の子プロセスはそのまま動きます。適用するには本人がこの CLI のフォアグラウンド起動を Ctrl+C で終了し、同じ `start` をもう一度実行します。キーも再入力します。自動再起動・Windows タスク・常駐サービスの登録は行いません。

stdio wrapper を変えるリリースでは manifest の `stdioVersion` も上げます。この値は release の識別子に含まれるため、依存バージョンが同じでも旧 wrapper を意図せず再利用しません。

一つの Tunnel ID を既存スクリプトとこの CLI で同時に起動しないでください。`start.lock` と tunnel-locks は同じ home 内の CLI の多重起動を拒否します。別 home、別端末、他の launcher は検出・停止しないため、本人が旧接続の終了を確認してから切り替えてください。

## キーの扱い

現段階の資格情報取得方法は対話ターミナルのマスク入力です。Tunnel 子プロセスへ `CONTROL_PLANE_API_KEY` として渡し、引数には `env:CONTROL_PLANE_API_KEY` だけを渡します。親の環境変数は変更しません。キーをファイルへ保存する機能はありません。setup 一度の入力と以後の入力不要起動は次の設計対象で、現在は未達です。

Node の管理メモリと子プロセスの環境にキーが存在する間は、メモリを完全にゼロ化する保証はありません。Commander を import する前に wrapper は `CONTROL_PLANE_API_KEY` と `OPENAI_API_KEY` を取り除きます。無人再起動や OS 資格情報保管庫は今回の対象外です。将来導入する場合は本人の明示的な同意と別レビューが必要です。

runtime は info/JSON log と HTTP raw log 無効で起動します。runtime 自体の出力やログを無条件に非秘密とは扱わず、共有前に本人が確認してください。CLI はログ・既存認証ファイル・環境変数値を doctor の出力へ表示しません。

## Windows のパスとプロセス

Tunnel は `spawn(executable, argv, { shell: false })` で起動します。`--mcp.command` の中では v0.0.16 が認識するシングルクォートを使います。

```text
'C:/Program Files/nodejs/node.exe' 'C:/Runtime With Spaces/stdio.mjs' --no-onboarding
```

Windows PowerShell から二重引用符入り文字列を native 引数に渡す方式を避け、空白付き executable が `C:/Program` に分割される問題を防ぎます。シングルクォート、改行、NUL を含む Node/runtime パスは、曖昧な解釈を避けるため起動前に停止します。別の `--home` を選んでください。

起動後は子プロセスの標準入出力を引き継ぎ、終了コードを CLI に返します。Ctrl+C はこの CLI が起動した Tunnel 子プロセスの終了要求だけに使います。

## doctor と lock

`doctor` は選択された manifest と receipt、および runtime executable、stdio wrapper、package-lock、Commander entry の SHA256 を確認します。全依存ファイルや OS 権限の監査は行いません。`connection: not-probed` は実接続を試していないことを示します。接続の完了は本人がアプリ側で確認してください。

CLI や端末が異常終了すると lock が残る場合があります。多重起動を避けるため、自動で lock を削除したり PID のプロセスを終了したりしません。本人が、その保存先を使う処理が終了済みであることを確認してから、doctor が示した該当の `start.lock`、tunnel-locks の該当 lock、`maintenance.lock`、または `releases/install.lock` のみを削除してください。PID が再利用されることがあるため、PID だけで無関係なプロセスを終了しないでください。

## 複数端末と接続登録

`--device` はこの PC 内の profile 名で、ChatGPT 側の接続名や Tunnel ID を生成しません。同じ CLI を各端末へ導入し、同時に使う端末ごとに異なる Tunnel ID と明示的な許可ルートを指定します。設定・所有者・キー入力は端末ごとに独立しています。コードや新しい launcher を端末ごとに作り直す必要はありません。

現 CLI はプラグイン作成・インストール・接続登録の API を実装していません。端末を追加しただけでアプリに自動表示されるわけではありません。一つのプラグインに複数接続を登録できるか、端末ごとに接続登録が必要かは、対象アプリの対応する登録方法に従います。setup 一度で登録まで完了する目標は、その公式な登録経路と本人の資格情報保管への同意を確認してから実装します。

## Desktop Commander の制限

`allowedDirectories` はファイルツールの設定です。コマンド実行をそのディレクトリへ閉じ込める OS sandbox ではありません。コマンドは起動ユーザーが持つ OS 権限で実行されます。通常ユーザーの PowerShell で起動し、接続先を本人が選択してください。新たな権限モデルや ACL 変更は導入していません。

新規 config は `defaultShell: powershell.exe`、read/write 行数 100/50、telemetry 無効と少数の blockedCommands を設定します。これは既存の最小起動構成に合わせた初期値です。設定済みの値は変更しません。blockedCommands は完全な実行制御ではなく、対話プロセスへの入力も別途安全性を判断する必要があります。

固定版 Commander の初期化時の Chrome 呼び出し 1 カ所だけを、インストール時の互換性 patch で抑止します。これにより初期化だけでの Chrome 実行ファイル探索・取得・cache 整理は行いません。既知のコード形に一致しない場合は準備を失敗させ、旧 runtime/config を保持します。明示的な PDF ツール利用に伴う上流処理は変更していません。新しい manifest はこの抑止を必須とし、抑止前の runtime を起動へ流用しません。

空の `allowedDirectories` は全体へのアクセスを許す upstream の挙動を避けるため、この CLI では拒否します。資格情報サブディレクトリだけを除外する deny-list の機能は upstream v0.2.52 にありません。許可ルートは本人が適切に絞ってください。

上流の `move_file` は `fs.rename` を使うため、別ドライブへの移動は `EXDEV` で失敗します。この wrapper はファイルツールを fork しません。コピー後に元を消す fallback も追加しません。同一ドライブ内の移動を使うか、本人がコピー結果を検証した上で別の操作として扱ってください。CLI の設定差し替えも同じディレクトリの一時ファイルから rename し、`EXDEV` では保存済み設定を保持します。

## 固定バージョンの出典

- [OpenAI Secure MCP Tunnels](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels)
- [公式 tunnel-client v0.0.16 release](https://github.com/openai/tunnel-client/releases/tag/v0.0.16)
- [Desktop Commander upstream](https://github.com/wonderwhy-er/DesktopCommanderMCP)
- [npm exec の仕様](https://docs.npmjs.com/cli/v11/commands/npm-exec/)
- [Home-Servers のリポジトリ分離方針](https://github.com/Xpotato1024/Home-Servers/blob/main/docs/architecture/repository-layout.md)

ZIP の SHA256 は公式 release metadata と承認済み成果物を照合しています。npm パッケージは exact version でインストールし、その場で生成した package-lock を release に残します。transitive dependencies をリポジトリに vendor 固定しているわけではありません。今後 tunnel runtime のバージョンを上げる際は、CLI flag と command parser の互換性も再検証してください。
