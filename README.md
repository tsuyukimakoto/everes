# Everes

並行するAIコーディング作業のTask Context Managerです。

Claude Code / Codex など複数のAIエージェントで同時に進めている作業を、人間が記憶に頼らず把握するために使います。想定規模は常時10〜20の並行作業です。

conversationが長くなって先頭の目的が画面から流れても、Everesを見れば「何の作業か / いまどうなっているか / 次に何をするか / 何待ちなのか」が分かる状態を保ちます。

## 設計の中心

管理対象はTaskです。Claude CodeのsessionやCodexのthreadは、Taskを実行するためのAgent Sessionとして扱います。

```text
Task
 ├── Claude session A
 ├── Claude session B
 └── Codex thread C
```

conversationが長くなって新しいsessionへ引き継いでも、Taskは変わりません。したがってsession IDを主キーにはせず、Everes独自のULIDを主キーとします。

状態はAgent Sessionごとに記録します。Taskのstatusはそれらの集約値で、activeなsessionが一つでもあればTaskもactive、activeがなく全ての未完了sessionがwaitingならTaskもwaitingになります。Taskをdoneにすると、紐づく全sessionもdoneになります。providerの実行状態や更新時刻からstatusを推測することはありません。

操作方法は [USAGE.md](USAGE.md)、設計の詳細は [docs/requirements.md](docs/requirements.md) を参照してください。

## 導入

apmとcurlが必要です。CLIはdarwin/arm64向けのprebuilt binaryとして配布されるため、Go toolchainは不要です。
apmがない場合は、先に[APM公式の導入手順](https://microsoft.github.io/apm/getting-started/installation/)でインストールしてください。

CLI本体と、Claude Code / Codex用のSkillとhookを同じrelease tagからインストールします。

```bash
EVERES_VERSION=v0.2.0
curl -fsSL "https://raw.githubusercontent.com/tsuyukimakoto/everes/${EVERES_VERSION}/install.sh" | bash -s -- "$EVERES_VERSION"
apm install -g "tsuyukimakoto/everes#${EVERES_VERSION}" --target claude,codex
apm install -g "tsuyukimakoto/everes/.apm/packages/everes-claude-hook#${EVERES_VERSION}" --target claude
apm install -g "tsuyukimakoto/everes/.apm/packages/everes-codex-hook#${EVERES_VERSION}" --target codex
```

`~/bin` が `PATH` にない場合は、利用中のシェルの設定に追加してください。導入を確認します。

```bash
everes version
apm view -g tsuyukimakoto/everes
```

APMパッケージは共通SkillとClaude Code / CodexそれぞれのSessionStart hookを配布します。Skillは `~/.agents/skills/`、hookは各エージェントのユーザー設定に配置されます。hookは配置先ごとに別パッケージとして導入します。

### 更新

`EVERES_VERSION`を更新先のrelease tagにして、CLI本体とAPMパッケージを更新します。

```bash
EVERES_VERSION=v0.2.1
curl -fsSL "https://raw.githubusercontent.com/tsuyukimakoto/everes/${EVERES_VERSION}/install.sh" | bash -s -- "$EVERES_VERSION"
apm install -g "tsuyukimakoto/everes#${EVERES_VERSION}" --target claude,codex
apm install -g "tsuyukimakoto/everes/.apm/packages/everes-claude-hook#${EVERES_VERSION}" --target claude
apm install -g "tsuyukimakoto/everes/.apm/packages/everes-codex-hook#${EVERES_VERSION}" --target codex
```

更新後に `everes version` と `apm view -g tsuyukimakoto/everes` を実行し、CLIとAPMパッケージのバージョンを確認してください。

Go moduleとして導入する場合は、Go 1.26以降を用意してrelease tagを指定します。

```bash
GOBIN="$HOME/bin" go install github.com/tsuyukimakoto/everes/cmd/everes@v0.2.0
```

### リポジトリから開発用に導入

このリポジトリをcloneしている場合は、CLIをビルドし、現在のソースからAPMパッケージを導入できます。

```bash
mise run install
mise run deploy
```

`apm install -g` はリポジトリの絶対パスを `~/.apm/apm.yml` に記録します。リポジトリを移動した場合は `mise run deploy` を再実行してください。

### 開発中の導入

ローカルで修正を続ける場合は、CLIとSkillをリポジトリ内のsourceへシンボリックリンクできます。

```bash
mise run dev-setup
```

`~/bin/everes` は `build/everes`、Claude CodeとCodexの `everes-task` Skillは `.apm/skills/everes-task/` を参照します。既存のコピーがある場合は、初回のみ `~/.everes/install-backups/` へ保存します。

Goコードを変更した後は、リンク先のバイナリを再生成します。

```bash
mise run build
```

Skillの変更はリンク先から読み込まれます。hookパッケージの設定を変更した場合は、`mise run dev-setup` を再実行してください。hookはユーザー設定へマージされるため、設定ファイル自体はシンボリックリンクにしません。

## 使い方

通常の操作は、Claude CodeやCodexとの対話から行います。

未登録の新しいAgent Sessionでは、最初のメッセージに対する応答でActive Taskが番号とタイトルで提示されます。番号またはタイトルを選ぶと既存Taskへ紐付きます。Task IDを調べて入力する必要はありません。SessionStart hookだけでは応答は始まらないため、セッション作成後は通常どおり最初のメッセージを送ります。

```text
この作業をEveresで管理して
いまの状態と次にすることをEveresに記録して
このTaskをEveresでウェイティングにして。レビューが返ったら再開する
このTaskを明日の9時までウェイティングにして。その時刻を過ぎたらCIを確認する
このIssueを関連付けて
いま何を抱えてる？
この作業は完了
このTaskに紐づくセッションの状態を確認して
このTaskの公開準備セッションをCodexアプリで表示して
レビュー用のCodex taskに追加の確認事項を送って
```

AIエージェントは `everes-task` Skillを使い、依頼に対応するCLI操作を実行します。
Skillの本文は [.apm/skills/everes-task/SKILL.md](.apm/skills/everes-task/SKILL.md) です。

Taskをターミナルで横断して確認する場合はCLIを直接使います。

```bash
everes task list
everes task list --status waiting
everes task list --all
everes task list --json
```

ターミナルがlinkに対応している場合、一覧の `↗ codex:<label>` を選択するとCodexアプリの該当taskへ移動します。
Claudeは該当sessionのCLI再開コマンドを一覧に表示します。Claude DesktopとClaude Code CLIは別のsession履歴を管理するため、Everesからの既存session再開はCLIを対象とします。

対話例とCLIの詳細は [USAGE.md](USAGE.md) を参照してください。

## 保存場所

```text
~/.everes/
├── tasks/<task-id>.json        # source of truth
├── sessions/<provider>/<session-id>.json  # 逆引きインデックス
├── locks/
└── logs/
```

Taskデータとsession indexはClaude CodeやCodexの内部ディレクトリには置きません。`~/.claude/` と `~/.codex/` に入るのは、APMが管理するSkillとhook設定だけです。

`EVERES_HOME` を設定すると保存先を変更できます。

書き込みはatomic write（tmpへ書く → fsync → rename）で行い、flockで排他します。Taskは `revision` を持ち、`--revision` を渡すと楽観ロックが効きます。

## 開発

依存はGoの標準ライブラリのみです。ULID生成も自前実装しています。

```bash
mise run check   # fmt / vet / test
mise run build
```

## 制約

- Codex Adapterのsession自動検出はcwdが一致する通常のuser threadのうち最新のものを返す補助手段です。subagentや権限確認用threadは除外します。同じcwdで複数sessionを開く場合は、SessionStart contextのproviderとsession IDを明示して操作します。
- `everes session open` のCodex Adapterは通常 `codex resume` でCLIを開きます。`--app` を付けるとCodexデスクトップアプリ内の該当taskへ移動します。
- Claude Adapterは `claude --resume` でClaude Code CLIを開きます。Claude DesktopのCodeタブはCLIと別のsession履歴を持つため、既存のDesktop sessionへの直接移動には対応しません。
- `everes session list` が示すのはsession記録の有無と保存情報です。Agentが現在実行中か待機中かはprovider側の機能で確認します。
- 別sessionへの依頼はEveresのmailboxへ保存するのではなく、利用中のCodexまたはClaude Codeの機能で対象sessionに新しいターンを開始します。
- 管理UI（一覧の専用アプリ）は未実装です。現状は `everes task list` が一覧の役割を担います。
- GitHub / Google WorkspaceのAPI同期は行いません。URLはreferenceとして保存するだけです。

## ライセンス

[MIT License](LICENSE)で公開します。
