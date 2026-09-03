# Everes Task Manager 設計書

## 1. 目的

Claude Code / Claude Code App / Codex CLI / Codex App など、複数のAIコーディングエージェントで同時並行して進めている作業を、人間が記憶に頼らず管理できるようにする。

想定利用規模は常時10〜20程度の並行作業。

AIエージェントのconversation/session/threadそのものをタスク管理の主体にはしない。

管理対象の中心は **Task** とし、Claude CodeのsessionやCodexのthreadは、そのTaskを実行するための **Agent Session** として扱う。

主に管理したい情報は以下。

* この作業は何を目的としているのか
* 現在どこまで進んでいるのか
* 現在何待ちなのか
* 次に何をするのか
* どの条件になったら再開するのか
* 関連するGitHub Issue / PR
* 関連するGoogle Docs等の資料
* repository / branch / worktree
* 人間向けメモ
* Claude Code / Codexのどのsession/threadに対応するのか
* 最終更新日時

AI conversationが長くなって先頭の目的が画面から流れても、Everesを見るだけで作業の意味と状態を把握できることを目標とする。

---

# 2. 基本思想

## 2.1 TaskとSessionを分離する

データモデルは次の関係とする。

```text
Task
 ├── objective
 ├── status (Agent Sessionからの集約値)
 ├── references
 ├── notes
 │
 └── Agent Sessions
      ├── Claude Code session (status / summary / next_action / resume_when)
      ├── Codex thread (status / summary / next_action / resume_when)
      └── 将来の別Agent
```

1つのTaskに複数のAgent Sessionが存在してよい。

例えばClaude Codeのconversationが長くなり、新しいsessionへ引き継いだとしてもTaskは変わらない。

```text
TASK-A
 ├── Claude session A
 ├── Claude session B
 └── Codex thread C
```

したがってClaudeやCodexのsession IDをEveresの主キーにはしない。

Everes独自の `task_id` を主キーとする。

---

# 3. スコープ

## V1で実装するもの

* ローカルTask store
* Claude Code sessionとの紐付け
* Codex threadとの紐付け
* Taskの作成
* Taskの状態更新
* Task一覧
* Task詳細表示
* GitHub / Google Docs等のURL登録
* notes
* status
* summary
* next_action
* resume_when
* repository / branch / cwd
* Agent別session ID
* Claude/CodexからTask情報を更新するSkill
* 管理アプリから対象Agent sessionを開くためのAdapter

## V1では実装しないもの

* GitHub APIとの自動同期
* Google Workspace APIとの自動同期
* PRレビュー完了などの自動監視
* クラウド同期
* 複数ユーザー
* チーム共有
* AIによる常時自動状態更新
* conversation全体の全文インデックス
* Claude/Codex内部データの書き換え

URLはV1では単なるreferenceとして保存する。

---

# 4. 保存場所

Claude CodeやCodexの内部保存ディレクトリにはEveresのファイルを置かない。

AIツール側の内部形式変更、cleanup、archive等の影響を避けるため、Everes専用ディレクトリを使用する。

```text
~/.everes/
├── tasks/
│   ├── <task-id>.json
│   └── ...
│
├── sessions/
│   ├── claude/
│   │   └── <session-id>.json
│   │
│   └── codex/
│       └── <thread-id>.json
│
└── locks/
    └── <task-id>.lock
```

保存先は環境変数 `EVERES_HOME` で変更できる。テストや検証で本番データを汚さないために使う。

設定ファイルとログはV1では持たない。必要になった時点で追加する。

Task情報のsource of truthは

```text
~/.everes/tasks/<task-id>.json
```

とする。

`~/.everes/sessions/` はsession IDからTaskを逆引きするためのインデックスとして使用する。

---

# 5. Task ID

UUIDでもよいが、人間が目で追うことも考えてULIDを推奨する。

例:

```text
01K2N86E2AS3Q9CJG4Y0J73SME
```

内部では完全なIDを使う。

UIでは短縮表示してよい。

```text
01K2N86E2AS3
```

短縮は12文字とする。

先頭10文字はミリ秒timestampであり、続けて作成したTask同士では同じ値になる。8文字ではtimestampの一部しか映らず、同時期に作ったTaskの短縮IDが揃って見分けられない。random部の先頭2文字まで含めて識別性を確保する。

CLIは短縮IDの前方一致でTaskを解決する。複数該当する場合は候補を示してエラーとする。

---

# 6. Taskデータモデル

例:

```json
{
  "schema_version": 2,
  "task_id": "01K2N86E2AS3Q9CJG4Y0J73SME",
  "revision": 14,

  "title": "認証方式の変更",

  "objective": "既存認証方式を整理し、新しいOAuthベースの認証方式へ移行する",

  "status": "waiting",

  "summary": "基本実装まで完了。現在PRレビュー待ち。",

  "next_action": "レビュー内容を確認し、必要な修正を行った後integration testを実行する",

  "resume_when": "GitHub PR #381 のレビューが返ってきたら",

  "check_at": "2026-08-17T09:00:00+09:00",

  "check_action": "GitHub PR #381 のレビュー状況を確認する",

  "workspace": {
    "cwd": "/Users/example/dev/foo",
    "repo_root": "/Users/example/dev/foo",
    "git_remote": "git@github.com:example/foo.git",
    "branch": "feature/oauth",
    "worktree": null
  },

  "references": [
    {
      "id": "ref-1",
      "type": "github_issue",
      "title": "認証方式変更",
      "url": "https://github.com/example/foo/issues/381"
    },
    {
      "id": "ref-2",
      "type": "google_doc",
      "title": "認証方式設計",
      "url": "https://docs.google.com/document/d/..."
    }
  ],

  "notes": [
    {
      "id": "note-1",
      "text": "refresh tokenについてはA案を採用した",
      "created_at": "2026-08-16T12:30:00+09:00"
    }
  ],

  "agent_sessions": [
    {
      "provider": "claude",
      "session_id": "00000000-0000-4000-8000-000000000001",
      "cwd": "/Users/example/dev/foo",
      "label": "認証方式変更",
      "status": "waiting",
      "summary": "実装を終えてレビュー待ち",
      "next_action": "レビュー指摘を反映する",
      "resume_when": "PRにレビューが付いたら",
      "check_at": "2026-08-17T09:00:00+09:00",
      "check_action": "PRのレビュー状況を確認する",
      "created_at": "2026-08-16T10:00:00+09:00",
      "last_seen_at": "2026-08-16T12:30:00+09:00"
    },
    {
      "provider": "codex",
      "session_id": "thr_123",
      "cwd": "/Users/example/dev/foo",
      "label": "OAuth migration review",
      "created_at": "2026-08-16T11:00:00+09:00",
      "last_seen_at": "2026-08-16T12:20:00+09:00"
    }
  ],

  "timestamps": {
    "created_at": "2026-08-16T10:00:00+09:00",
    "updated_at": "2026-08-16T12:30:00+09:00",
    "completed_at": null
  }
}
```

---

# 7. status

Agent Sessionのstatusは自由入力にせずenumとする。Taskのstatusは一覧と検索に使う集約値である。

statusは以下。

```text
active
waiting
needs_me
paused
done
```

意味:

### active

現在作業を進めている。

### waiting

外部要因待ち。

例:

* CI待ち
* GitHubレビュー待ち
* 他人からの回答待ち
* 仕様確定待ち

`resume_when` を原則として記録する。

### needs_me

AI側では進められず、人間の判断や操作が必要。

### paused

意図的に作業を止めている。

再開条件が外部イベントではない場合にも利用する。

### done

Taskの目的達成済み。

## 7.1 Task statusの集約

明示statusを持つAgent Sessionを次の優先順で集約する。

1. activeなsessionが一つでもあればTaskはactive
2. activeがなくneeds_meがあればTaskはneeds_me
3. activeとneeds_meがなくwaitingがあればTaskはwaiting
4. pausedだけが残ればTaskはpaused

v1から移行しstatusが未設定のsessionは `NOT_SET` と表示して集約から除外し、providerの実行状態やlast_seen_atから推測しない。`NOT_SET` はstatus enumには含めない。全sessionがdoneでもTaskは自動でdoneにしない。`task done`だけがTaskをdoneにし、そのとき全Agent Sessionをdoneにする。

---

# 8. summary / next_action / resume_when / check_at / check_action の意味

この3つを明確に分離する。

## summary

「現在どういう状態なのか」。

例:

```text
OAuth実装とunit testまで完了。PRを作成してレビュー待ち。
```

## next_action

「次に実際に行う作業」。

例:

```text
レビュー指摘を反映してintegration testを実行する。
```

## resume_when

「いつこのTaskに戻ればよいか」。

例:

```text
PR #381 にレビューが付いたら。
```

`waiting` の場合は特に `resume_when` を重要情報として扱う。

## check_at

waitingを次に確認する日時。
timezoneを含まない入力はCLIを実行した環境のローカルtimezoneで解釈し、UTC offset付きのRFC 3339で保存する。

例:

```text
2026-08-17T09:00:00+09:00
```

## check_action

`check_at`を過ぎたら確認する対象または操作。

例:

```text
PR #381のレビュー状況を確認する
```

`check_at`と`check_action`は必ず同時に設定し、statusが`waiting`の場合だけ保持する。
確認時刻を過ぎてもstatusは自動変更しない。
一覧では期限を過ぎたwaitingを通常のwaitingより前に表示し、確認内容を同時に表示する。

---

# 9. Agent Sessionモデル

Agent Sessionはprovider固有の識別情報に加え、status、summary、next_action、resume_when、check_at、check_action、completed_atを持つ。

session index:

```json
{
  "schema_version": 1,
  "provider": "claude",
  "session_id": "00000000-...",
  "task_id": "01K2N86E2AS3Q9CJG4Y0J73SME",
  "cwd": "/Users/example/dev/foo"
}
```

Codex:

```json
{
  "schema_version": 1,
  "provider": "codex",
  "session_id": "thr_123",
  "task_id": "01K2N86E2AS3Q9CJG4Y0J73SME",
  "cwd": "/Users/example/dev/foo"
}
```

これにより、

```text
(provider, session_id)
```

からTaskを即座に検索できる。

---

# 10. Provider Adapter

Everes coreにClaude/Codex固有処理を書かない。

以下のinterfaceを設ける。

```go
type Session struct {
	Provider  string
	SessionID string
	Cwd       string
	Label     string
}

type Provider interface {
	// Name はprovider識別子を返す。
	Name() string
	// DetectCurrentSession は実行中のsessionを推定する。
	DetectCurrentSession(cwd string) (*Session, error)
	// OpenSession は既存のsessionを再開する。
	OpenSession(s Session) error
	// SessionExists はsessionの実体が残っているかを返す。
	SessionExists(s Session) (bool, error)
	// SessionMetadata は表示や診断に使う付随情報を返す。
	SessionMetadata(s Session) (map[string]any, error)
}
```

`DetectCurrentSession` がcwdを受け取るのは、Adapterがsessionの保存場所をcwdから引くためである。Everes coreはその対応関係を知らない。

Adapterは自身をregistryへ登録する。coreは名前で引くだけで、具体的なAdapterをimportしない。

sessionを特定できない場合は `ErrNoCurrentSession` を返す。複数のAdapterが候補を返した場合も特定できないものとして扱う。誤ったTaskへ紐付けるくらいなら検出失敗を返すほうが安全である。

---

# 11. Claude Adapter

Claude固有処理は

```text
internal/provider/claude
```

以下に閉じ込める。

Claude CodeにはSessionStart Hookがあり、新規開始だけでなくresume時にも実行されるため、Everesとのsession紐付けに利用できる。

想定フロー:

```text
Claude session start/resume
        ↓
SessionStart Hook
        ↓
"$HOME/bin/everes" hook session-start --provider claude
        ↓
Claude session IDを取得
        ↓
既存Taskとの対応を検索
        ↓
存在すればTask情報をAgentへ提示
存在しなければ未完了Task候補と新規Task作成方針をAgentへ提示
```

`everes hook session-start` はhook専用の出口とする。

人間やSkillが使う `everes session current` とは責務が異なる。出力先がharnessのcontext注入形式であり、providerごとに形式が変わるためである。

このコマンドは **照会専用** とし、Everesの状態を一切書き換えない。

振る舞いは以下の2通りのみ。

* 現在sessionに紐づくTaskが存在する場合、objective / status / summary / next_action / resume_when / references / notes をcontextへ出力する
* 存在しない場合、未登録である旨、未完了Task候補、現在sessionを候補へ紐付けるために必要な値、新規Task作成の選択方針をcontextへ出力する

新しいsessionでTaskが未設定の場合、Agentは通常の依頼を進める前に、最初の応答でTaskへの紐付けをユーザーに確認する。
最初のユーザーメッセージと未完了Taskのtitle、objective、summary、next_action、resume_whenを比較し、明確に一致するTaskがあれば1件を推奨する。推奨以外の未完了Taskもすべて選択肢に含める。
最初のユーザーメッセージから作業を端的に表す新規Taskタイトルを1件提案し、提案タイトルでの新規作成、ユーザーが入力するタイトルでの新規作成、どのTaskにも紐付けない選択肢を含める。未完了Taskがない場合も、新しいTaskを作るか確認して同じ新規作成選択肢を提示する。
Task IDはcontext内の値を使い、ユーザーに入力を求めない。

ユーザーが選択するまでは、Taskを作成・更新・紐付けしない。
既存Taskが選択された場合だけ、Agentが `everes session attach` を実行する。新規Taskが選択された場合だけ、Agentが現在sessionへ紐づくTaskを作成する。

Claude Code SkillからEveres CLIを呼べる構成にする。Claude Codeでは `SKILL.md` ベースのSkillを利用できる。

---

# 12. Codex Adapter

Codex固有処理は

```text
internal/provider/codex
```

以下に閉じ込める。

Codexにはローカルsession/threadを再開する仕組みがあり、Codex App Serverでは保存済みthread IDを `thread/resume` に渡して再開できる。

Codex SDKもlocal Codex threadのstart / continue / resumeを提供している。

したがって管理アプリからCodexへ戻る処理は、可能ならApp ServerまたはSDKを利用する。

初期実装ではCLI resumeを利用してもよい。

CodexのPersonal Skillは:

```text
$HOME/.agents/skills/
```

へ配置できる。

この配置はEveres自身が行わない。apmが単一のSKILL.mdからClaude / Codex双方へ配布する（「apmによる配布」章を参照）。

Everes SkillはClaude版と同じ操作概念を提供する。

SessionStart HookはCodexにも配布する。

```text
~/.codex/hooks.json
```

へapmが配置する。

Codex用hookは次のコマンドを実行する。

```text
"$HOME/bin/everes" hook session-start --provider codex
```

ClaudeとCodexは共通するhook入力フィールドを持つため、入力形式からproviderを推測しない。

hookが受け取ったsession IDと固定したproviderをcontextへ出力し、Skillはその値を `--provider` と `--session-id` でCLIへ渡す。

Codex Adapterによる自動検出では、`~/.codex/sessions/` のrolloutファイルから末尾のUUIDを取得し、session metadataのcwdが現在のcwdと一致するものを候補にする。`thread_source` がある場合は `user` だけを採用し、subagentや権限確認用threadを対象外とする。

同じcwdで複数のCodex sessionが動いている場合、更新日時だけでは現在のsessionを一意に選べないため、自動検出はhook contextがない場合の補助手段とする。

既存sessionをCLIで再開する場合は `codex resume <session-id>` を使う。Codexデスクトップアプリで表示する場合は `codex://threads/<thread-id>` を開く。

---

# 13. Skill

Claude / Codex双方に、意味的に同一のSkillを提供する。

名称:

```text
everes-task
```

Claude:

```text
~/.claude/skills/everes-task/
```

Codex:

```text
~/.agents/skills/everes-task/
```

SKILL.mdの実体はリポジトリ内の1ファイルのみとする。

```text
.apm/skills/everes-task/SKILL.md
```

symlinkや生成スクリプトは用意しない。apmが単一ソースから両方の配置先へ配布する。

Skillの役割はEveresのデータ構造を直接操作することではなく、Everes CLIを利用すること。

例:

```text
everes session current --json
everes task update ...
everes note add ...
everes ref add ...
```

JSONファイルをClaude/Codex自身に直接編集させない。

---

# 14. Skillが理解するユーザー操作

以下の自然言語を扱えるようにする。

```text
この作業をEveresで管理して
```

新規Task作成と現在sessionの紐付け。

```text
状態を更新して
```

conversationの現在状況を要約し、

* summary
* status
* next_action
* resume_when

を更新する。

```text
このTaskをEveresでウェイティングにして
```

例:

```text
status = waiting
resume_when = レビューが返ってきたら
```

`レビュー待ちにして` だけではGitHubのIssueやPRに対する操作と区別できないため、Everesの更新要求として扱わない。
waitingへ変更するときは、Everes、Task、status、`waiting`、ウェイティングのいずれかが依頼に明示されていることを必要とする。

```text
このIssueを関連付けて
```

GitHub URLをreferenceとして追加。

```text
このドキュメントを関連付けて
```

Google Docs等のURLをreferenceとして追加。

```text
メモしておいて
```

notes追加。

```text
この作業は完了
```

statusをdoneにする。

---

# 15. AIによる状態更新ルール

status、Taskの完了、objectiveはconversationから推測して自動変更しない。別sessionへの委譲を始めたときや、sessionまたはTaskの担当内容が終わったと判断したときは、AIが変更候補を示してユーザーに確認する。ユーザーが同じ発言でstatusを指定していれば、重ねて確認しない。

Agent Sessionのsummaryとnext_actionは履歴ではなく、現在地を可視化する置換可能なsnapshotとする。Everesで管理中のsessionでは、次の節目で内容が古くなっていれば更新する。

* 実装や検証が一区切りつき、現在地が変わったとき
* 別sessionへ仕事を委譲する前
* 長い作業を区切るとき
* 最終回答の前

毎ターン更新したり、進捗がないのに文面だけを書き換えたりしない。Skill単独ではwall clockによる定期処理を行えないため、時間間隔ではなく意味のある作業区切りを更新契機とする。判断経緯や恒久的な情報はsnapshotへ混ぜず、noteへ追加する。

---

# 16. CLI

実行ファイル名:

```text
everes
```

最低限以下を実装する。

```text
everes task create
everes task list
everes task show <task-id>
everes task update <task-id>
everes task done <task-id>

everes session attach <task-id>
everes session update <task-id>
everes session done <task-id>
everes session detach <task-id>
everes session current
everes session open <task-id>

everes ref add <task-id> <url>
everes ref remove <task-id> <ref-id>

everes note add <task-id> <text>
everes note remove <task-id> <note-id>

everes hook session-start
```

コマンド体系は `everes <名詞> <動詞>` に統一する。

`everes hook session-start` はhook専用の出口であり、人間もSkillも直接呼ばない（第11章参照）。

利用頻度が高い `everes session current` にのみaliasを設ける。

```text
everes current
```

AI Skill向けにはmachine-readable optionも用意する。

```text
everes task show --json
everes task list --json
everes session current --json
```

主なoption:

| option | 対象 | 意味 |
| --- | --- | --- |
| `--no-attach` | `task create` | 現在sessionを紐付けない |
| `--status` / `--all` | `task list` | statusで絞る / doneも表示する |
| `--revision` | `task update`, `session update` | 指定revisionと一致する場合のみ更新する（第23章） |
| `--check-at` / `--check-action` | `task create`, `task update`, `session update` | waitingを確認するローカル日時 / その時刻を過ぎたら確認すること |
| `--list` | `session open` | 紐づくsessionを一覧して開かない |
| `--app` | `session open` | providerのdesktop appで対象sessionを表示する |
| `--provider` / `--session-id` | session系 | 対象sessionを明示指定する |
| `--type` / `--title` | `ref add` | 参照種別とタイトル。typeは省略時にURLから推測する |

`task list` はdoneを既定で隠す。一覧は「いま抱えている作業」を映す面とするためである。

一つのTaskに複数のAgent Sessionが紐づいても、`task list` ではTaskを一つのまま表示する。TaskにはAgent Sessionから集約したstatusとsession数を表示し、その下に個別のsessionを階層表示する。各sessionの行にはstatus、provider、label、summary、next actionまたはresume when、確認予定をまとめる。Codex sessionは対応ターミナル上で `codex://threads/<thread-id>` へのリンクとする。Claude sessionはClaude Code CLIで再開する完全な `everes session open` コマンドを表示する。session IDや保存状態などの詳細は `task show` または `session list` で確認する。

flagは位置引数の後ろに書いても解釈する。Skillが生成するコマンドで語順に気を遣わせないためである。

`note add` の本文は残り全体を連結する。ハイフンで始まる語を含む場合に備え、`--` で本文の開始を示せるようにする。

```text
everes note add <task-id> -- refresh tokenについてはA案を採用した
```

---

# 17. `everes session current`

AI連携で最も重要なコマンド。alias `everes current` でも呼べる。

現在実行中のClaude/Codex sessionを識別し、それに対応するTaskを返す。

例:

```json
{
  "provider": "claude",
  "session_id": "abc123",
  "task": {
    "task_id": "01K2N86E2AS3Q9CJG4Y0J73SME",
    "title": "認証方式変更",
    "status": "waiting",
    "objective": "既存認証方式を整理しOAuthベースへ移行する",
    "summary": "PRレビュー待ち",
    "next_action": "レビュー修正",
    "resume_when": "PR #381のレビュー後",
    "revision": 14
  }
}
```

Task全体ではなく、AIが状況を把握するのに必要な項目だけを返す。

`revision` を含めるのは、更新時に楽観ロックをかけられるようにするためである。

Task未登録なら:

```json
{
  "provider": "claude",
  "session_id": "abc123",
  "task": null
}
```

sessionそのものを特定できない場合は、providerとsession_idも空にする。

```json
{
  "provider": "",
  "session_id": "",
  "task": null
}
```

---

# 18. 管理UI

重要なのは画面装飾ではなく一覧性である。

この一覧性の要件は、まず `everes task list` が満たす。専用のUI（macOS向けのnative appまたはTUI）は、CLIを実際に使ったうえで必要性を判断する。

statusごとにグルーピングし、`needs_me` から順に並べる。人間の注意を要する順である。

メインビュー例:

```text
NEEDS ME

╭─ 01K2N86E2AS3  認証API変更
│  Task  NEEDS ME (2 sessions) · 12 min ago
│  現在地: レビュー指摘への対応が必要
│  次: 指摘内容を確認して修正する
│  参照: GitHub #381
├─ Sessions  1 open · 1 done
│  ┌─ NEEDS_ME  codex:修正
│  │  現在地: 指摘内容を確認中
│  │  次: 修正してテストする
│  └─
│  ✓ claude:調査 — 影響範囲の確認完了
│    復帰: everes session open 01K2N86E2AS3 --provider claude --session-id 00000002
╰─


WAITING

╭─ 01K2N86E2BT4  通知基盤変更
│  Task  WAITING (1 session) · 2 days ago
│  現在地: API仕様確定待ち
│  再開: Issue #422が確定したら
├─ Sessions  1 open
│  ┌─ WAITING  codex:実装
│  │  現在地: API案を作成済み
│  │  再開: Issue #422が確定したら再開
│  └─
╰─


ACTIVE

╭─ 01K2N86E2CU5  動画export高速化
│  Task  ACTIVE (2 sessions) · 34 min ago
│  現在地: Metal encoder周辺を調査中
│  次: Instrumentsでボトルネックを確認する
├─ Sessions  2 open
│  ┌─ ACTIVE  codex:計測
│  │  現在地: encoderを計測中
│  │  次: ボトルネックを特定する
│  └─
│  ┌─ PAUSED  claude:資料調査
│  │  現在地: 関連資料を整理済み
│  └─
╰─
```

一覧で最低限表示する:

```text
短縮task-id
title
status
summary
resume_when または next_action
references
session数
sessionごとのstatus、provider、label、summary、next_actionまたはresume_when、確認予定
updated_at
```

短縮task-idを出すのは、続く操作でTaskを指定する必要があるためである。

Taskの `resume_when` と `next_action` はどちらか一方を出す。`waiting` では `resume_when` を、それ以外では `next_action` を優先する。Agent Sessionにも同じ規則を適用する。項目名と短い本文は同じ行に置き、Terminal幅を超える本文だけを同じ項目のインデントへ折り返す。空行はTask group間にだけ置き、Task内では使わない。

未完了sessionはstatus、provider、label、summary、next_actionまたはresume_when、確認予定を展開する。done sessionは一覧上で自動的に圧縮し、providerとlabel、完了時のsummary、provider内で一意になるsession ID末尾を使う復帰commandを2行以内で示す。`session open --session-id` は完全一致のほか、一意な先頭または末尾を受け付ける。通常のcommand出力は対話状態を保持できないため折り畳み操作は設けず、この圧縮表示を既定の折り畳み相当とする。全項目は `task show` と `session list` で確認できる。

---

# 19. Task詳細ビュー

右ペインまたは詳細画面に以下を表示。

```text
Title

Objective

Status

Summary

Next Action

Resume When

References
  GitHub Issue
  GitHub PR
  Google Doc
  Other

Workspace
  Repository
  Branch
  Worktree
  cwd

Agent Sessions
  Claude Code
  Codex

Notes

Created
Updated
```

Task IDとrevisionも表示する。revisionは楽観ロックに使うためである。

Agent Sessionごとに再開手段を提示する。専用UIではボタンとする。

```text
[Open in Claude]

[Open in Codex]
```

CLIでは再開コマンドを案内する。

```text
再開: everes session open <短縮task-id>
```

---

# 20. Open Session

Taskそのものではなく、Taskに紐づくAgent Sessionを選択して開く。

複数sessionが存在する場合:

```text
認証方式変更

Claude Code
  8/16 10:32
  認証実装

Codex
  8/16 11:45
  review

Claude Code
  8/16 12:20
  review修正
```

から選択できるようにする。

最新sessionをワンクリックで開く操作も提供する。

CLIでは以下に対応する。

```text
everes session open <task-id> --list           # 一覧するだけで開かない
everes session open <task-id>                  # 最新sessionを開く
everes session open <task-id> --provider codex # provider内の最新を開く
everes session open <task-id> --session-id ... # session指定で開く
everes session open <task-id> --app           # desktop appで表示する
```

紐付け間違いや不要にsessionをTaskから外すときは、次を使う。provider側の対話自体は削除しない。

```text
everes session detach <task-id> --provider codex --session-id ...
```

sessionの実体が見つからない場合は警告を出したうえで再開を試みる。Adapterの検出が外れているだけで再開自体は成功する場合があるためである。

---

# 21. Provider固有起動処理

## Claude

Claude Adapterは `claude --resume <session-id>` でClaude Code CLIの既存sessionを再開する。Claude DesktopのCodeタブはCLIと別のsession履歴を持つため、Desktopの既存sessionへの直接移動は対象外とする。

非公開・非保証の起動方法を使用する場合でも構わない。

ただしEveres coreはその具体的方法を知らないこと。

破損時にはClaude Adapterのみ変更できること。

## Codex

優先順位:

```text
Codex App Server
    ↓
Codex SDK
    ↓
Codex CLI
```

のいずれかでsession/threadを再開する。

Codex App Serverはthread ID指定のresumeを公式に提供しているため、長期的にはこれを第一候補とする。

---

# 22. 状態ファイル書き込み

ファイル破損防止のためatomic writeを使用する。

手順:

```text
同一ディレクトリへ一時ファイルを作る
↓
書き込む
↓
fsync
↓
rename
↓
task.json
↓
ディレクトリをfsync
```

一時ファイル名は `<task-id>.json.tmp-<random>` とする。固定名にすると同時書き込みで互いを壊すためである。

renameそのものを永続化するため、最後にディレクトリもfsyncする。

同時にClaudeとCodexから更新される可能性があるため、排他制御を入れる。

```text
~/.everes/locks/<task-id>.lock
```

ロックは `flock(2)` で取得する。プロセスが異常終了しても自動解放されるため、PIDを書き込む方式と違ってstale lockの掃除が要らない。

read-modify-writeの全体をロックの内側で行う。

---

# 23. 更新競合

Task JSONにrevisionを持たせる。

```json
{
  "revision": 14
}
```

update時:

```text
read revision 14
↓
update
↓
revision 15
```

expected revisionが一致しなければ書き込みを拒否する。

将来的なGUIとAgentの同時更新に備える。

expected revisionの指定は任意とする。指定がない場合はロック内のread-modify-writeのみで直列化する。

同一ホスト上の同時更新はロックで防げる。revisionが効くのは、表示してから更新するまでの間にロックの外で状態が進む場合である。

---

# 24. schema migration

必ず

```json
"schema_version": 1
```

を持たせる。

Readerは古いversionを読めること。

Writerは常に最新versionを書く。

読み込み経路には必ず変換を挟む。

schema変更処理は、versionの昇順に並べたstepの列として持つ。

```go
var steps = []Step{
	{From: 1, To: 2, Apply: v1ToV2},
	{From: 2, To: 3, Apply: v2ToV3},
}
```

現在のschema versionは1であり、stepは存在しない。schemaを変更する際にここへ追加する。

保存されたschema versionが実装より新しい場合は、読まずにエラーとする。古いバイナリが新しいファイルを壊さないためである。エラーにはEveresの更新を促す文言を含める。

---

# 25. references

将来拡張できるgeneric構造とする。

```json
{
  "id": "ref-123",
  "type": "github_issue",
  "title": "...",
  "url": "...",
  "metadata": {}
}
```

type候補:

```text
github_issue
github_pr
google_doc
google_sheet
google_slide
notion
jira
linear
web
file
other
```

V1ではURLとtitleだけ使用する。

`metadata` は将来のAPI integration用であり、V1では書き出さない。

`type` は省略時にURLから推測する。判別できないものは `other` とする。

`title` が空の場合はURLで代替する。一覧で何も表示されないより読めるためである。

---

# 26. repository情報

workspace情報はTask作成時に自動取得する。

取得可能なら:

```text
cwd
git root
remote origin
branch
worktree
```

を保存する。

Agent session attach時にはTaskのworkspaceを上書きしない。別ディレクトリからattachした際に、その作業の主たる場所が書き換わってしまうためである。

Agent session detach時はTask内の紐付けとEveresの逆引きだけを削除し、provider側のsessionや対話履歴は変更しない。

attach時に記録するのは、そのAgent Session自身のcwdとする。Taskのworkspaceとsessionごとのcwdは別の情報として扱う。

gitが無い場合やリポジトリ外の場合も失敗にはしない。workspaceは付随情報であり、Taskの成立条件ではない。

detached HEADではbranchを空にする。`.git` の位置がrepo直下と異なる場合にworktreeと判定する。

ただしTaskの意味はrepositoryと独立している。

1つのTaskが複数repositoryを跨ぐ可能性を将来的に考慮する。

V1では単一workspaceでよい。

---

# 27. Task作成フロー

Claude/Codexで:

```text
この作業をEveresで管理して
```

↓

Skill:

```text
conversationからtitle / objective候補作成
everes task create --title ... --objective ...
```

↓

Task作成。

現在sessionの取得、workspace情報の取得、session紐付けはCLIが行う。Skillはtitleとobjectiveを組み立てるだけでよい。

sessionを特定できなかった場合もTaskは作成する。その旨を返し、あとから `everes session attach` で紐付けられるようにする。

AIがconversationから推測してよいのは初期値のみ。

---

# 28. Task再開フロー

管理UI:

```text
Task選択
↓
Agent Session選択
↓
Open
```

↓

Provider Adapter:

```text
Claude Adapter
または
Codex Adapter
```

↓

既存session再開。

Agent起動時には以下をAgentへcontextとして渡す。

```text
Objective
Summary
Next Action
Resume When
References
Notes
```

この受け渡しはSessionStart Hookが行う。resume時にもHookが実行されるため、再開経路が管理UIであってもCLIであっても同じ仕組みで機能する。

`everes session open` 自身はcontextを渡さない。対象sessionを選び、通常はAdapterに再開を依頼し、`--app` ではproviderのdesktop app用deep linkを開く。

---

# 29. 新Sessionへの引き継ぎ

Taskに紐づいたsessionが長くなった場合、新sessionへ移行可能にする。

例:

```text
everes session attach <task-id>
```

現在sessionを既存Taskへ追加。

誤った紐付けは次の操作で外す。

```text
everes session detach <task-id> --provider <provider> --session-id <session-id>
```

旧sessionは削除しない。

```text
Task
 ├ old session
 └ new session
```

最新sessionをdefaultとする。`last_seen_at` が最も新しいものを指す。

同一 (provider, session_id) へ再度attachした場合は、重複を作らず `last_seen_at` を更新する。

---

# 30. SkillとEveres Coreの責務分離

Skillは:

```text
自然言語
↓
Everes CLI操作
```

だけを担当。

Everes CLIは:

```text
validation
locking
persistence
session mapping
provider integration
```

を担当。

SkillからJSONを直接書き換えてはいけない。

これによりClaudeとCodexで同一データを安全に利用できる。

---

# 31. ディレクトリ構成案

実装言語はGoとする。

サードパーティ依存は持たない。ULID生成を含め標準ライブラリのみで実装する。

```text
everes/
├── apm.yml                          # apmパッケージmanifest
├── mise.toml                        # build / install / test タスク
├── go.mod
├── README.md
│
├── cmd/
│   └── everes/
│       └── main.go
│
├── internal/
│   ├── task/                        # Taskモデル、status、ULID、validation
│   ├── store/                       # 永続化層
│   │                                #   atomic write、flock、revision
│   │                                #   Task CRUD と session index
│   ├── migration/                   # schema_version移行
│   ├── provider/
│   │   ├── provider.go              # AgentProvider interface
│   │   ├── claude/
│   │   └── codex/
│   └── cli/
│
├── .apm/
│   ├── skills/
│   │   └── everes-task/
│   │       └── SKILL.md
│   └── packages/
│       ├── everes-claude-hook/
│       │   ├── apm.yml
│       │   └── .apm/hooks/session-start.json
│       └── everes-codex-hook/
│           ├── apm.yml
│           └── .apm/hooks/session-start.json
│
├── schemas/
│   ├── task-v1.schema.json
│   └── task-v2.schema.json
│
└── docs/
    └── requirements.md
```

`.apm/` 配下のレイアウトはapmの規約に従う。第32章を参照。

管理UIをSwiftで別途作る場合も、core storage仕様は言語非依存にすること。

---

# 32. apmによる配布

Skillとhookの配布はapm（Agent Package Manager）に委ねる。

Everes自身は `~/.claude/skills/` や `~/.agents/skills/` へファイルを配置しない。

## パッケージレイアウト

apmはパッケージルートの形状でinstall方法を決定する。

Everesはskillとhookの双方を配るため、`.apm/` レイアウトを使う。

リポジトリルートに以下を置いてはならない。別レイアウトと判定され、hookが展開されなくなる。

```text
SKILL.md
skills/
.claude-plugin/
plugin.json
```

## apm.yml

```yaml
name: everes
version: 0.3.0
description: 並行するAIコーディング作業のTask Context Manager
targets:
  - claude
  - codex
```

`targets` はパッケージ側からの制限として働く。consumer側で有効なtargetとの積集合が実際の配布先になる。

共通skillはルートパッケージに置く。

hookはproviderを固定する必要があるため、`target: claude` と `target: codex` を指定した独立したローカルパッケージに分ける。

APM 0.28.0では、ルートのローカルパッケージからtarget固有のローカル依存を宣言すると、依存解決後の統合でhookパッケージがスキップされた。したがって、hookパッケージはそれぞれ直接installする。

## 配布先

apmが以下へ配置する。Everesはこのパスを知らない。

| primitive | source | claude | codex |
| --- | --- | --- | --- |
| skill | `.apm/skills/everes-task/SKILL.md` | `~/.claude/skills/everes-task/` | `~/.agents/skills/everes-task/` |
| Claude hook | `.apm/packages/everes-claude-hook/.apm/hooks/session-start.json` | `~/.claude/settings.json` へマージ | 配布しない |
| Codex hook | `.apm/packages/everes-codex-hook/.apm/hooks/session-start.json` | 配布しない | `~/.codex/hooks.json` へマージ |

hookはAPMが受け付けるClaude形式のJSONで記述する。

各hookパッケージの `target` が配布先を制限し、commandの `--provider` を固定する。hookはアプリ起動時の `PATH` に依存しないよう、`"$HOME/bin/everes"` を実行する。

apmはtargetごとにevent名を正規化し、`${PLUGIN_ROOT}` 等のパス変数を書き換える。

apmは `.claude/apm-hooks.json` を台帳として持ち、各エントリに `_apm_source` を付けて管理する。手書き設定と区別できる。

## 導入手順

CLI本体はapmでは配布できない。apmが扱うのはskillとhookのみである。

```text
mise run install                                  # go build → ~/bin/everes
mise run deploy                                   # Skillと2種類のhookを配布
```

`mise run deploy` は以下の3コマンドを順に実行する。

```text
apm install -g <everes-repo-path> --target claude,codex
apm install -g <everes-repo-path>/.apm/packages/everes-claude-hook --target claude
apm install -g <everes-repo-path>/.apm/packages/everes-codex-hook --target codex
```

`apm install -g` は各ローカルパッケージの絶対パスを `~/.apm/apm.yml` に記録する。

リポジトリを移動した場合は再installが必要になる。

---

# 33. セキュリティ

Task metadataに以下は保存しない。

```text
access token
OAuth token
API key
password
cookie
secret
```

reference URLに機密tokenがquery parameterとして含まれる場合は警告する。

Task JSONは原則ユーザーのhome directory内に保存する。

---

# 34. 重要な設計原則

このアプリは「AI session manager」ではない。

**人間が多数のAI並行作業を把握するためのTask Context Manager** である。

ClaudeやCodexのconversationは作業履歴であって、Taskそのものではない。

したがって最も重要なのは、

```text
Task
  objective
  status
  summary
  next_action
  resume_when
  references
```

をconversationの寿命から独立して保持すること。

Claude Code、Codex、将来の別のcoding agentは交換可能な実行環境として扱う。

この境界を崩さないこと。
