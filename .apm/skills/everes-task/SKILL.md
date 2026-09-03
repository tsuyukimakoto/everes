---
name: everes-task
description: >
  Everes（並行するAIコーディング作業のTask Context Manager）でTaskを作成・更新・参照するスキル。
  「この作業をEveresで管理して」「Everesの状態を更新して」「このTaskをウェイティングにして」
  「明日の9時までウェイティングにして、その時刻になったらCIを確認する」
  「このIssueを関連付けて」「このドキュメントを関連付けて」「メモしておいて」
  「この作業は完了」「いま何を抱えてる」「他に進行中の作業は」など、
  作業の目的・進捗・待ち状態・次のアクション・関連リンクの記録や参照を求められたときに使用する。
  waitingへの変更はEveres、Task、status、waiting、ウェイティングのいずれかが明示された場合に限る。
  「レビュー待ちにして」など外部サービスの操作とも読める依頼だけではEveresを更新しない。
  conversationが長くなって目的を見失った場合の現在地確認にも使う。
  Everesのデータを直接編集してはならず、必ず everes CLI を経由すること。
---

# everes-task

Everesは、複数のAIコーディングエージェントで同時並行に進めている作業を、人間が記憶に頼らず管理するための道具です。

管理の主体は **Task** です。Claude CodeのsessionやCodexのthreadは、そのTaskを実行するための **Agent Session** に過ぎません。

進捗、status、次の作業、再開条件、確認予定は現在のAgent Sessionへ記録します。TaskのstatusはAgent Sessionの集約値です。Taskの目的や共有情報はTaskへ、各conversationの現在地はAgent Sessionへ記録してください。

conversationが長くなって先頭の目的が流れても、Everesを見れば作業の意味と状態が分かる状態を保つことが目的です。

## 絶対に守ること

**`~/.everes/` 配下のJSONを直接読み書きしてはいけません。**

必ず `everes` コマンドを経由してください。validation、排他制御、session mapping、schema migrationはすべてCLIが担っています。直接編集するとこれらが働かず、他のAgentとの同時更新でデータが壊れます。

**statusを推測で変更してはいけません。現在地のsnapshotは古いままにしません。**

`status` の変更、Taskの完了、objectiveの変更は、ユーザーが明示した場合だけ実行します。ただし、別sessionへの委譲を始めたときや、このsessionの担当内容が終わったと判断したときは、作業を終える前に「このsessionを waiting / paused / done のどれにするか」を短く確認してください。状態変更の確認をせず、古い `active` のまま放置してはいけません。

一方、すでにEveresで管理中のsessionについて、`summary` と `next_action` はconversationの現在地を示す置換可能なsnapshotです。実質的な進捗があった節目、別sessionへ委譲する前、長い作業の区切り、最終回答の前に内容が古くなっていれば、`everes session update` で更新してください。これは追記型の履歴ではありません。判断経緯や恒久的に残す情報だけをnoteに記録します。

進捗がないのに文面だけを書き換えたり、毎ターン更新したりしてはいけません。wall clockによる定期実行はSkill単独では行えないため、「定期的」は意味のある作業区切りごとの更新として扱います。

**「レビュー待ち」をEveres更新の合図にしてはいけません。**

「レビュー待ちにして」は、GitHubのIssueやPRに対する操作とも解釈できます。
Everesの `waiting` へ変更するのは、ユーザーがEveres、Task、status、`waiting`、ウェイティングのいずれかを明示した場合だけです。
曖昧な依頼ではEveresを更新せず、操作対象を確認してください。

## 前提の確認

`everes` コマンドが必要です。見つからない場合はユーザーに導入を依頼してください。

```bash
everes version
```

SessionStart contextに次の形式でproviderとsession IDがある場合は、その値をCLIへ明示的に渡してください。

```text
[Everes] このsession (codex <session-id>) ...
[Everes] このsession (claude <session-id>) ...
```

同じ作業ディレクトリで複数sessionを開いている場合、更新日時による自動検出では現在のsessionを区別できません。
contextに値があるのに自動検出へ切り替えてはいけません。

未登録sessionのSessionStart contextを受け取った場合は、新しいsessionの最初の応答で通常の依頼を進める前に、Taskへ紐付けるかをユーザーに確認してください。
最初のユーザーメッセージと未完了Task候補のtitle、objective、summary、next action、resume whenを比較し、明確に合う既存Taskがあれば1件を「推奨」と表示します。推奨以外も含め、未完了Taskはすべて選択肢に残してください。
同じユーザーメッセージから作業を端的に表す新規Taskタイトルを1件提案し、「提案タイトルで新規Taskを作成」「ユーザーが入力するタイトルで新規Taskを作成」「どれにも紐付けない」を選択肢に含めます。未完了Taskがない場合は「新しいTaskを作りますか？」と確認し、この3つを提示してください。Task IDの入力を求めてはいけません。
既存Taskが選択された場合だけ、contextにある対応するTask ID、provider、session IDを使って `everes session attach` を実行します。新規Taskが選択された場合だけ、提案したtitleまたはユーザーが入力したtitleで、現在のsessionへ紐づくTaskを作成します。
選択前にTaskを作成、更新、紐付けしてはいけません。

## ユーザーの依頼と操作の対応

### 「この作業をEveresで管理して」

新しいTaskを作成し、現在のsessionへ紐付けます。

conversationからtitleとobjectiveの候補を組み立ててから実行してください。**AIが推測してよいのは初期値のみ**です。

```bash
everes task create \
  --title "認証方式の変更" \
  --objective "既存認証方式を整理しOAuthベースの認証方式へ移行する" \
  --provider <provider> \
  --session-id <session-id>
```

providerとsession IDはSessionStart contextの値を使います。
contextに値がない場合に限り、現在のsessionが自動検出されます。
workspace（cwd、git remote、branch）は自動取得されます。

作成後、titleとobjectiveが意図と合っているかユーザーに確認してください。

### 「いまの状態を更新して」

conversationの現在状況を要約し、以下を更新します。

```bash
everes session update <task-id> \
  --status waiting \
  --summary "OAuth実装とunit testまで完了。PRを作成してレビュー待ち。" \
  --next-action "レビュー指摘を反映してintegration testを実行する" \
  --resume-when "PR #381 にレビューが付いたら"
```

各項目は意味が異なります。混ぜないでください。

| 項目 | 意味 | 例 |
| --- | --- | --- |
| `--summary` | 現在どういう状態なのか | OAuth実装とunit testまで完了。PRレビュー待ち。 |
| `--next-action` | 次に実際に行う作業 | レビュー指摘を反映してintegration testを実行する |
| `--resume-when` | いつこのTaskに戻ればよいか | PR #381 にレビューが付いたら |
| `--check-at` | waitingを次に確認するローカル日時 | 2026-08-17 09:00 |
| `--check-action` | その時刻を過ぎたら確認すること | PR #381のレビュー状況を確認する |

この更新は過去の記録への追記ではなく、現在のAgent Sessionのsnapshotを置き換えます。`summary` には「このsessionが何を担当し、どこまで終わり、いまどうなっているか」を短く記述し、一覧だけで現在地を判断できるようにしてください。

## 作業中のsnapshotとstatus確認

Everesに紐づくsessionで作業している間は、次の節目で現在のsessionの `summary` と `next_action` がconversationと一致しているか確認します。

- 検証が通る、実装方針が確定するなど、現在地が変わったとき
- 別のAgent Sessionへ仕事を委譲する直前
- 長い作業をいったん区切るとき
- 最終回答を返す前

古くなっていれば、statusは変えずに `everes session update` でsnapshotを置き換えます。更新内容には、完了したこと、残っていること、次に行う具体的な作業を含めます。

次の場合はstatusを自動変更せず、その場でユーザーに短く確認します。

- 別sessionへ委譲し、このsession側に実行中の作業が残っていない: `waiting`、`paused`、`done` のどれにするか
- このsessionの担当内容を終えた: sessionを `done` にするか
- Task全体の目的を達成した: Taskを `done` にするか

ユーザーが同じ発言でstatusを指定している場合は、重ねて確認せず、その指定どおり更新します。

### 「このTaskをEveresでウェイティングにして」

statusと再開条件を設定します。`waiting` では、イベントを待つ場合は `--resume-when`、指定時刻に確認する場合は `--check-at` と `--check-action` を記録してください。何待ちなのか、何を確認するのかが分からないTaskは一覧で役に立ちません。

```bash
everes session update <task-id> --status waiting --resume-when "レビューが返ってきたら" --provider <provider> --session-id <session-id>
```

ローカル日時を指定された場合は、実行環境のtimezoneを使って年月日と時刻を確定し、確認内容と一緒に設定してください。

```bash
everes session update <task-id> \
  --status waiting \
  --check-at "2026-08-17 09:00" \
  --check-action "PR #381のレビュー状況を確認する" \
  --provider <provider> \
  --session-id <session-id>
```

相対表現の日付は現在のローカル日時から解決してください。日時を一意に決められない場合はユーザーに確認します。
`--check-at` だけを設定してはいけません。確認時刻を過ぎてもstatusは `waiting` のままで、`everes task list` の `WAITING — CHECK NOW` に表示されます。
確認を終えてTaskを再開または再度waitingにするときは、ユーザーの指示に従ってstatusまたは次の確認日時を更新してください。

statusは以下のいずれかです。

| status | 意味 |
| --- | --- |
| `active` | 現在作業を進めている |
| `waiting` | 外部要因待ち（CI、レビュー、他人の回答、仕様確定） |
| `needs_me` | AI側では進められず、人間の判断や操作が必要 |
| `paused` | 意図的に作業を止めている |
| `done` | 目的達成済み |

### 「このIssueを関連付けて」「このドキュメントを関連付けて」

URLを参照として追加します。種別はURLから自動判定されます。

```bash
everes ref add <task-id> https://github.com/example/foo/issues/381 --title "認証方式変更"
```

URLのquery parameterにtokenらしき値が含まれる場合、CLIが警告を出します。警告が出たらユーザーに伝え、そのURLを登録してよいか確認してください。

### 「メモしておいて」

判断の経緯など、後から読み返したい内容を記録します。

```bash
everes note add <task-id> -- refresh tokenについてはA案を採用した
```

メモ本文にハイフンで始まる語が含まれる場合に備え、`--` で本文の開始を示してください。

### 「この作業は完了」

```bash
everes task done <task-id>
```

Taskの完了はTaskに紐づく全Agent Sessionをdoneにします。一つのsessionだけを終了する場合は `everes session done <task-id> --provider <provider> --session-id <session-id>` を使います。

### 「いま何を抱えてる」「他に進行中の作業は」

```bash
everes task list
```

statusごとにグルーピングされ、`needs_me` から順に表示されます。完了済みは既定で隠れます。すべて見る場合は `--all` を付けます。

### 「この作業の現在地を教えて」

現在のsessionに紐づくTaskを表示します。

```bash
everes session current --provider <provider> --session-id <session-id>
```

conversationが長くなって目的を見失ったときは、まずこれを実行してください。

### 「このTaskに紐づくsessionを確認して」

```bash
everes session list <task-id> --json
```

`last_seen_at` はEveresへ最後に紐付けた時刻であり、Agentが現在実行中であることを表しません。
provider側の保存状態は `exists` と `metadata` を使って説明してください。
保存ファイルの更新時刻だけで実行中、待機中、完了を推測してはいけません。

## 別のsessionを参照・操作する

ユーザーが別のAgent Sessionの状態確認を求めた場合は、先に `everes session list <task-id> --json` で対象を特定してください。

Codexアプリにthread参照機能がある場合は、対象のCodex thread IDを指定して履歴と状態を読み取ります。
Codexアプリで対象を画面表示するよう求められた場合は、アプリのtask移動機能へthread IDを渡してください。
CLIから表示する場合は `everes session open <task-id> --app --session-id <session-id>` を使います。`everes task list` の `↗ codex:<label>` も同じCodex deep linkを開きます。
Claude DesktopとClaude Code CLIは別のsession履歴を持ちます。Everesに紐づくClaude CLI sessionは `everes session open <task-id> --provider claude --session-id <session-id>` でCLIへ再開します。`task list` に表示される同じコマンドを案内してください。Claude Desktopの既存sessionへ直接移動できると説明してはいけません。
EveresのAgent Session `status` はそのconversationで進める作業の状態です。Task `status` はAgent Session statusの集約値です。どちらもCodexアプリの `active`、`idle`、`notLoaded` などの実行状態とは別物として説明してください。
Claude Codeの独立sessionについてEveresが提供できるのは、session記録の存在と更新時刻までです。
Claude Agent Teams内であれば、teamのtask状態やteammateの情報はAgent Teamsの機能で確認します。

### 別のsessionへ依頼を送る場合

別sessionへの送信は、ユーザーが宛先と依頼内容を明示した場合だけ行ってください。
これは通知の記録ではなく、対象sessionへ新しいユーザー指示を追加してAgentの処理を開始する操作です。

- Codexアプリで別taskへの送信機能が利用できる場合は、対象threadを必要に応じて読み取ってから、その機能でfollow-up promptを送る
- Claude Agent Teams内のteammateには `SendMessage` を使う
- Agent Teamsに属さないClaude sessionへ明示的に実行を依頼する場合は `claude -p --resume <session-id> <prompt>` を使う

対象が複数ある場合は、label、provider、session IDを提示してユーザーに宛先を確認してください。
実行環境に送信機能がない場合は、利用できないことを伝え、`everes session open <task-id>` で対話的に再開する方法を案内してください。
Everesに独自のmailboxがあるかのように振る舞ってはいけません。

## 新しいsessionへの引き継ぎ

conversationが長くなって新しいsessionへ移った場合、既存Taskへ現在のsessionを追加します。Taskは変わりません。

SessionStart contextに未完了Task候補が提示されている場合は、ユーザーが番号またはタイトルを選ぶだけで、対応するTaskへ追加します。
候補にないTaskを指定された場合に限り、Task一覧から対象を探します。

```bash
everes session attach <task-id> \
  --label "レビュー修正" \
  --provider <provider> \
  --session-id <session-id>
```

誤ったsessionを紐付けた場合は、providerとsession IDを明示して外します。provider側のconversationは削除されません。

```bash
everes session detach <task-id> \
  --provider <provider> \
  --session-id <session-id>
```

旧sessionは削除されません。1つのTaskに複数のAgent Sessionが並存してよい設計です。

## machine-readable出力

判断のために構造化データが必要な場合は `--json` を使ってください。

```bash
everes session current --json
everes session list <task-id> --json
everes task show <task-id> --json
everes task list --json
```

## task-idの指定

task-idは先頭8文字などの前方一致で指定できます。曖昧な場合はCLIが候補を示してエラーになります。

```bash
everes task show 01M04EKP
```

## 更新が競合したとき

`revision conflict` が出た場合、他のsessionが先に更新しています。`everes task show` で最新を確認してから、上書きしてよいかユーザーに判断を仰いでください。黙って再実行してはいけません。

## コマンド一覧

```text
everes task create --title <title> [--objective ...] [--status ...] [--provider ...] [--session-id ...] [--no-attach]
everes task list [--json] [--status <status>] [--all]
everes task show <task-id> [--json]
everes task update <task-id> [--status ...] [--summary ...] [--next-action ...] [--resume-when ...]
everes task done <task-id>

everes session current [--provider ...] [--session-id ...] [--json]
everes session list <task-id> [--json]
everes session attach <task-id> [--label ...] [--provider ...] [--session-id ...]
everes session update <task-id> [--status ...] [--summary ...] [--next-action ...] [--resume-when ...] [--check-at ...] [--check-action ...]
everes session done <task-id> [--provider ...] [--session-id ...]
everes session detach <task-id> --provider <provider> --session-id <session-id>
everes session open <task-id> [--list] [--app]

everes ref add <task-id> <url> [--title ...] [--type ...]
everes ref remove <task-id> <ref-id>

everes note add <task-id> -- <text>
everes note remove <task-id> <note-id>
```

`everes hook session-start` はSessionStart hookが使う内部コマンドです。呼ばないでください。
