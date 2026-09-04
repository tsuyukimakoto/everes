# Everes

Everes is a Task Context Manager for AI coding work running in parallel.

It helps people understand and resume 10–20 concurrent jobs across tools such as Claude Code and Codex without relying on memory. Even after a conversation becomes long, Everes keeps the purpose, current state, next action, and resume condition visible.

## Core model

Everes manages **Tasks**. Claude Code sessions and Codex threads are treated as **Agent Sessions** working on a Task.

```text
Task
 ├── Claude session A
 ├── Claude session B
 └── Codex thread C
```

A Task survives when work moves to a new conversation, so Everes uses its own ULID rather than a provider session ID as the primary key.

Progress and status are recorded per Agent Session. The Task status is derived from those sessions: any active session makes the Task active; if no session is active and every unfinished session is waiting, the Task is waiting. Completing a Task completes all attached sessions. Everes never infers status from provider runtime state or file modification times.

See [USAGE.md](USAGE.md) for detailed usage and [docs/requirements.md](docs/requirements.md) for the design specification.

## Installation

[APM](https://microsoft.github.io/apm/getting-started/installation/) is required. The APM package includes a prebuilt CLI for macOS on Apple silicon, so a Go toolchain is not required.

One command installs the CLI, shared Skill, and SessionStart hooks for Claude Code and Codex from the same release tag:

```bash
EVERES_VERSION=v0.3.0
apm install -g "tsuyukimakoto/everes#${EVERES_VERSION}" --target claude,codex
```

The CLI is stored inside each installed Skill and invoked by absolute path, so Agent and hook execution does not depend on `~/bin` or the shell `PATH`.

```bash
"$HOME/.agents/skills/everes-task/bin/everes" version
apm view -g tsuyukimakoto/everes
```

APM installs the shared Skill into `~/.agents/skills/` and `~/.claude/skills/`, and merges each provider-specific SessionStart hook into that provider's user configuration.

### Updating

Set `EVERES_VERSION` to the desired release tag and run the same install command:

```bash
EVERES_VERSION=v0.3.0
apm install -g "tsuyukimakoto/everes#${EVERES_VERSION}" --target claude,codex
```

Verify both the bundled CLI and package version afterward:

```bash
"$HOME/.agents/skills/everes-task/bin/everes" version
apm view -g tsuyukimakoto/everes
```

If you also want to invoke `everes` directly from a terminal, the optional `install.sh` installs it as `~/bin/everes`. Agent and hook operation does not require this additional installation.

To install it as a Go module, use Go 1.26 or later and specify a release tag:

```bash
GOBIN="$HOME/bin" go install github.com/tsuyukimakoto/everes/cmd/everes@v0.3.0
```

### Installing from a repository checkout

Build the CLI and deploy the current APM package from a clone:

```bash
mise run install
mise run deploy
```

`apm install -g` records the absolute repository path in `~/.apm/apm.yml`. Run `mise run deploy` again after moving the repository.

### Development setup

Use symlinks when iterating on the local source:

```bash
mise run dev-setup
```

This links `~/bin/everes` to `build/everes` and both installed `everes-task` Skills to `.apm/skills/everes-task/`. Existing copies are backed up under `~/.everes/install-backups/` on the first run.

After changing Go code, rebuild the linked binary:

```bash
mise run build
```

Skill changes are read through the symlink. Run `mise run dev-setup` again after changing hook packages because APM merges hooks into user configuration files rather than linking them.

## Usage

Normal operation happens through conversation with Claude Code or Codex.

For an unregistered Agent Session, the first response offers unfinished Tasks, a concise proposed title for a new Task, a user-provided title, and an option to remain unattached. A clearly matching existing Task is marked as recommended. The SessionStart hook provides context but does not initiate a response, so send the first message normally after starting a session.

Example requests:

```text
Manage this work in Everes
Record the current state and next action in Everes
Set this Task to waiting in Everes; resume when the review arrives
Set this Task to waiting until 9:00 tomorrow, then check CI
Attach this issue
What am I currently working on?
This work is complete
Show the sessions attached to this Task
Open this Task's release-preparation session in the Codex app
Send an additional request to the Codex review task
```

The AI agent uses the `everes-task` Skill to translate those requests into CLI operations. Its source is [.apm/skills/everes-task/SKILL.md](.apm/skills/everes-task/SKILL.md).

Use the CLI directly to inspect Tasks across sessions:

```bash
everes task list
everes task list --status waiting
everes task list --all
everes task list --json
```

In terminals that support links, select `↗ codex:<label>` to open the corresponding task in the Codex app. For Claude sessions, the list displays a Claude Code CLI resume command. Claude Desktop and Claude Code CLI maintain separate histories, so Everes resumes Claude sessions through the CLI.

## Storage

```text
~/.everes/
├── tasks/<task-id>.json
├── sessions/<provider>/<session-id>.json
├── locks/
└── logs/
```

Task data and session indexes are not stored in Claude Code or Codex internal directories. Only APM-managed Skills and hook configuration are placed under `~/.claude/` and `~/.codex/`.

Set `EVERES_HOME` to change the storage root. Writes are atomic and guarded with `flock`. Tasks carry a revision number; pass `--revision` to enable optimistic locking.

## Development

The implementation uses only the Go standard library, including its ULID implementation.

```bash
mise run check
mise run build
```

## Release

Run the release task on `main`:

```bash
mise run release
```

It runs checks and a build, shows the current version and pending changes, then asks for a new version. Press Enter to accept the next patch version. After input, it updates version references, validates again, signs the darwin/arm64 CLI with Developer ID and the Hardened Runtime, submits it to Apple's notary service, and verifies the notarized code requirement. Only then does it update the bundled APM binary, commit, create an annotated tag, atomically push `main` and the tag, wait for GitHub Actions, and verify the GitHub Release. It stops at the first failed command and restores version edits and the bundled binary when failure occurs before the release commit.

The release Mac must provide the Developer ID Application identity and a valid `notarytool` Keychain profile. The repository defaults are the Everes G2 certificate fingerprint and the `everes-notary` profile. Set `EVERES_CODESIGN_IDENTITY`, `EVERES_CODESIGN_IDENTIFIER`, or `EVERES_NOTARYTOOL_PROFILE` only when those values change. A standalone executable cannot carry a stapled ticket, so Gatekeeper obtains its notarization ticket from Apple online.

## Limitations

- Codex session auto-detection is a fallback that selects the newest ordinary user thread whose cwd matches; it excludes subagents and approval threads
- `everes session open` uses `codex resume` by default and opens the Codex desktop task when passed `--app`
- Claude sessions are resumed through `claude --resume`; Everes cannot navigate directly to a Claude Desktop Code-tab session
- Session records show saved existence and metadata, not whether an agent is currently running
- Sending work to another session uses the active Codex or Claude Code facility; Everes does not provide a mailbox
- A dedicated management UI is not yet implemented; `everes task list` is the current overview
- GitHub and Google Workspace URLs are stored as references; Everes does not synchronize their APIs

## License

[MIT License](LICENSE)

---

# Everes（日本語）

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

apmが必要です。CLIはdarwin/arm64向けのprebuilt binaryとしてAPMパッケージに同梱されるため、Go toolchainは不要です。
apmがない場合は、先に[APM公式の導入手順](https://microsoft.github.io/apm/getting-started/installation/)でインストールしてください。

CLI本体と、Claude Code / Codex用のSkillとhookを同じrelease tagから一括でインストールします。

```bash
EVERES_VERSION=v0.3.0
apm install -g "tsuyukimakoto/everes#${EVERES_VERSION}" --target claude,codex
```

CLIはSkill内に配置され、AgentとSessionStart hookから絶対パスで実行されます。導入を確認します。

```bash
"$HOME/.agents/skills/everes-task/bin/everes" version
apm view -g tsuyukimakoto/everes
```

APMパッケージはCLIを含む共通Skillと、Claude Code / CodexそれぞれのSessionStart hookを配布します。Skillは `~/.agents/skills/` と `~/.claude/skills/`、hookは各エージェントのユーザー設定に配置されます。

### 更新

`EVERES_VERSION`を更新先のrelease tagにして、CLI本体とAPMパッケージを更新します。

```bash
EVERES_VERSION=v0.3.0
apm install -g "tsuyukimakoto/everes#${EVERES_VERSION}" --target claude,codex
```

更新後に `"$HOME/.agents/skills/everes-task/bin/everes" version` と `apm view -g tsuyukimakoto/everes` を実行し、CLIとAPMパッケージのバージョンを確認してください。

ターミナルから `everes` を直接使う場合だけ、従来の `install.sh` で `~/bin/everes` に追加できます。Agentとhookの動作には不要です。

Go moduleとして導入する場合は、Go 1.26以降を用意してrelease tagを指定します。

```bash
GOBIN="$HOME/bin" go install github.com/tsuyukimakoto/everes/cmd/everes@v0.3.0
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

未登録の新しいAgent Sessionでは、最初のメッセージに対する応答で未完了Task、新規Taskの提案タイトル、任意タイトルでの新規作成、紐付けなしが提示されます。依頼内容に合う既存Taskがあれば推奨として表示されます。未完了Taskがない場合も、新しいTaskを作るか確認されます。SessionStart hookだけでは応答は始まらないため、セッション作成後は通常どおり最初のメッセージを送ります。

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

## 公開

`main`で次のcommandを実行すると、checkとbuildの成功後に現在のversionと変更対象が表示され、新しいversionの入力を求められます。空Enterでは次のpatch versionを使います。

```bash
mise run release
```

versionを入力すると、version表記の更新、再検証、Developer ID署名、Hardened Runtimeとsecure timestampの確認、Appleへのnotarization、notarized code requirementの検証を順に実行します。すべて成功した同一バイナリをAPMへ同梱してから、commit、annotated tagの作成、mainとtagのatomic pushへ進みます。tag pushで起動したGitHub Actionsは同梱済みバイナリを再ビルドせずGitHub Releaseへ掲載します。途中のcommandが失敗した場合はそこで停止します。commit前の失敗では、書き換えたversion表記と同梱バイナリを元に戻します。

公開を実行するMacには、Everes用の `Developer ID Application` 証明書と秘密鍵、および `everes-notary` という `notarytool` のキーチェーンprofileが必要です。単体実行ファイルにはticketをstapleできないため、GatekeeperはAppleからnotarization ticketをオンラインで取得します。

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
