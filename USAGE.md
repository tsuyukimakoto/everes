# Everesの使い方

Everesの通常操作は、Claude CodeやCodexとの対話から行います。
利用者がTask IDやCLIのoptionを覚える必要はなく、何を記録または確認したいかをAIエージェントへ伝えます。

CLIは、複数のTaskを横断して一覧する場合、JSONを別の処理へ渡す場合、動作を診断する場合に直接使います。
AIエージェントも同じCLIを使うため、対話とCLIのどちらから操作しても保存されるデータは同じです。

## Everesが管理するもの

Everesでは、継続して達成したい作業を**Task**、Claude CodeのsessionやCodexのthreadを**Agent Session**として扱います。
一つのTaskには複数のAgent Sessionを紐付けられるため、conversationを新しくしてもTaskを作り直す必要はありません。

Taskには目的や共有情報を、Agent Sessionにはconversationごとの進捗と状態を記録できます。TaskのstatusはAgent Sessionのstatusから集約されます。

- 作業のタイトルと目的
- 現在のstatus
- 現在までに分かっていること
- 次に行うこと
- 作業を再開する条件
- 関連するIssueや文書のURL
- 判断の経緯などのメモ
- 作業に使ったClaude CodeのsessionとCodexのthread

## 対話でできること

Claude CodeやCodexにEveresのSkillが導入されていれば、次のように依頼できます。

| 状況 | 依頼例 | エージェントが行うこと | 実行後の状態 |
| --- | --- | --- | --- |
| 新しい作業を管理する | 「この作業をEveresで管理して」 | conversationからtitleとobjectiveを組み立ててTaskを作成 | Taskと現在のAgent Sessionが紐付きます |
| 現在地を記録する | 「いまの状態をEveresに記録して」 | summaryとnext actionを整理して更新 | 次のconversationでも現在地を確認できます |
| 外部の返答を待つ | 「このTaskをEveresでウェイティングにして。PRにレビューが付いたら再開する」 | 現在のAgent Sessionのstatusとresume whenを更新 | activeなsessionがなければTaskも `waiting` として表示されます |
| 指定日時に確認する | 「明日の9時までウェイティングにして。その時刻を過ぎたらCIを確認する」 | status、check at、check actionを更新 | 時刻を過ぎると確認対象として一覧の上部に表示されます |
| 人間の判断を待つ | 「私の判断待ちにして」 | statusを `needs_me` に更新 | Taskが一覧の先頭に表示されます |
| 作業を一時的に止める | 「この作業を中断して」 | statusを `paused` に更新 | 未完了のまま一覧に残ります |
| Issueや文書を関連付ける | 「このIssueを関連付けて」 | URLをreferenceとして追加 | Task詳細から参照先を確認できます |
| 判断を記録する | 「この方針をメモしておいて」 | 判断内容をnoteとして追加 | 後のconversationで経緯を確認できます |
| 現在の作業を確認する | 「この作業の現在地を教えて」 | 現在のAgent Sessionに紐づくTaskを取得 | 目的、状態、次の作業が表示されます |
| Taskを横断して確認する | 「いま何を抱えてる」 | 未完了のTaskを取得してstatus別に整理 | 人間の判断待ちや外部待ちをまとめて確認できます |
| 条件を指定して確認する | 「レビュー待ちのTaskだけ見せて」 | `waiting` のTaskだけを取得 | 該当するTaskが一覧表示されます |
| 作業を完了する | 「この作業は完了」 | statusを `done` に更新 | 通常の未完了一覧から外れます |

Taskの作成、status、完了、objectiveは、明示的な依頼またはAIエージェントからの確認に対する回答がある場合だけ変更します。
Everesで管理中のAgent Sessionでは、実装・検証・委譲などの節目で、AIエージェントがsummaryとnext actionを現在地のsnapshotとして置き換えます。毎ターンの履歴を追記するのではなく、一覧に最新の状態を一つだけ見せるための更新です。

## 新しい作業を対話から登録する

作業をEveresで管理したい時点で、AIエージェントへ次のように依頼します。

```text
この作業をEveresで管理して
```

AIエージェントはconversationからtitleとobjectiveを組み立て、現在のAgent Sessionを指定してTaskを作成します。
実行したディレクトリがGitリポジトリであれば、cwd、repository root、remote、branch、worktreeも記録されます。

作成後は、AIエージェントがtitleとobjectiveを提示します。
意図と異なる場合は、その場で修正を依頼します。

```text
目的は公開作業そのものではなく、公開前に必要な修正を洗い出すことに変更して
```

## 作業の現在地を対話から更新する

作業を中断する前やconversationが長くなった時点で、現在地の記録を依頼します。

```text
ここまでの状態と、次にすることをEveresに記録して
```

AIエージェントは次の三項目を分けて記録します。

| 項目 | 記録する内容 | 例 |
| --- | --- | --- |
| `summary` | 現在までに完了したことと現在の状態 | 実装とunit testが完了し、PRレビュー待ち |
| `next_action` | 再開後に最初に行う具体的な操作 | レビュー指摘を反映してintegration testを実行する |
| `resume_when` | Taskへ戻る条件 | PRにレビューが付いたら |

記録内容を限定したい場合は、変更する項目を伝えます。

```text
次にすることだけ「公開用READMEの例を確認する」に変えて
```

指定しなかった項目は保持されます。

## EveresのTaskをwaitingにする

レビュー、CI、回答、仕様確定などを待つ場合は、待っている対象と再開条件を伝えます。

```text
このTaskをEveresでウェイティングにして。PRにレビューが付いたら再開する
```

AIエージェントは現在のAgent Sessionのstatusを `waiting` にし、resume whenへ再開条件を記録します。ほかにactiveなsessionがあるTaskはactiveのままです。
一覧では、`waiting` のTaskが `active` より先に表示されます。

「レビュー待ちにして」だけでは、GitHubのIssueやPRに対する依頼とEveresの状態更新を区別できません。
Everesを更新する場合は、Everes、Task、status、waiting、ウェイティングのいずれかを明示します。
これらがない曖昧な依頼で、AIエージェントがEveresを更新することはありません。

日時を決めて確認する場合は、ローカル日時と確認内容を一緒に伝えます。

```text
このTaskをEveresで明日の9時までウェイティングにして。その時刻を過ぎたらPR #381のレビュー状況を確認する
```

AIエージェントは年月日を現在のローカル日時から解決し、`check_at`と`check_action`を記録します。
`check_at`はUTC offset付きで保存されるため、たとえば日本時間の `2026-08-17 09:00` は `2026-08-17T09:00:00+09:00` になります。

確認時刻を過ぎてもstatusは自動的に変わりません。
`everes task list`では通常のwaitingより前に `WAITING — CHECK NOW` として表示され、確認すべき内容も同時に表示されます。
Everes自体は指定時刻に処理を自動実行したり、通知を送ったりしません。

人間の判断や操作がなければ進められない場合は、判断待ちであることを伝えます。

```text
ライセンスを私が選ぶ必要があるので、私の判断待ちにして
```

この場合はstatusが `needs_me` になります。
`needs_me` のTaskは、利用者が対応対象を見つけやすいよう一覧の先頭に表示されます。

## Issueや判断をTaskへ残す

作業に必要なIssue、PR、Google Workspace文書、Notion、Jira、Linearなどは、URLと用途を伝えます。

```text
このIssueを「認証方式変更」として関連付けて
https://github.com/example/foo/issues/381
```

AIエージェントはURLをreferenceとして追加し、URLから種別を判定します。
query parameterにtoken、secret、passwordなどを示す名前が含まれている場合は警告を伝え、登録してよいかを確認します。

URLではなく判断の経緯を残す場合は、メモする内容を伝えます。

```text
refresh tokenについてはA案を採用したとメモしておいて
```

この内容はTaskのnoteとして保存されます。

## 現在のTaskを対話から確認する

conversationの目的や現在地を確認する場合は、現在の作業について尋ねます。

```text
この作業の現在地を教えて
```

AIエージェントは現在のAgent Sessionに紐づくTaskを取得し、objective、status、summary、next action、resume whenを提示します。
referenceやnoteが登録されていれば、それらも確認できます。

SessionStart hookが有効な場合は、新しいAgent Sessionの開始時にも同じ情報がcontextへ渡されます。
同じTaskにほかのAgent Sessionが紐付いていれば、そのprovider、session ID、labelもcontextに含まれます。
hookはTaskを読み取るだけで、Taskの作成や更新は行いません。

conversationの途中で最新のTask状態を読み直す場合は、次のように依頼します。

```text
Everesからこの作業の最新状態を読み直して
```

紐づいているほかのAgent Sessionも確認する場合は、対象を明示します。

```text
このTaskに紐づくセッションと、その記録が残っているかを確認して
```

この確認で分かるのは、Everesへの紐付けとprovider側のsession記録の有無、保存データの更新時刻です。
Agentが現在実行中か待機中かは保存ファイルの時刻から推測しません。

## 複数のTaskを対話から確認する

作業全体を確認したい場合も、AIエージェントへそのまま尋ねられます。

```text
いま何を抱えてる？
```

AIエージェントは未完了のTaskを取得し、`needs_me`、`waiting`、`active`、`paused` の順に整理して提示します。

条件を付けると、対象を絞れます。

```text
私の判断が必要なTaskだけ見せて
```

```text
レビュー待ちとCI待ちのTaskを確認して
```

```text
完了したものも含めて、認証関連のTaskを探して
```

現在のCLIには自由文検索専用のcommandがないため、最後の例ではAIエージェントがTask一覧を取得してtitle、objective、summaryなどを読み分けます。

## 新しいAgent Sessionへ引き継ぐ

SessionStart hookが有効で、新しいAgent SessionがどのTaskにも紐付いていない場合、最初のユーザーメッセージに対する応答で未完了Taskと新規作成の選択肢が提示されます。
hookだけではAIの応答は始まらないため、セッションを作成したら通常どおり最初のメッセージを送ります。
AIエージェントはそのメッセージの依頼を進める前にTaskを確認します。

```text
このsessionはどのTaskの続きですか？

1. 認証方式を変更する（推奨）
2. Everesを公開する
3. 新規Task「ログイン失敗時の表示を修正」を作成
4. 別のタイトルを入力して新規Taskを作成
5. どれにも紐付けない
```

AIエージェントは最初のメッセージと各Taskのtitle、objective、summary、next action、resume whenを比較し、明確に合う既存Taskがあれば1件を推奨します。推奨以外の未完了Taskもすべて選択できます。
新規Taskのタイトルは最初のメッセージから端的なものが1件提案され、そのまま使うか、別のタイトルを入力できます。未完了Taskがない場合は、新しいTaskを作るか同じ選択肢で確認されます。
番号またはタイトルで選択します。Task IDを調べたり入力したりする必要はありません。
既存Taskを選択すると、AIエージェントがSessionStart contextに含まれるTask ID、provider、session IDを使い、現在のAgent Sessionを追加します。新規Taskを選択すると、選んだタイトルでTaskを作成して現在のAgent Sessionを紐付けます。
選択するまではTaskの作成、更新、紐付けは行われません。

候補になるのは `done` 以外のTaskです。
`done` のTaskへ引き継ぐ場合は、Taskのタイトルを指定して依頼します。

```text
このsessionを認証方式変更のTaskに紐付けて、ラベルは「レビュー指摘の反映」にして
```

AIエージェントは既存Taskを特定し、新しいAgent Sessionを追加します。
以前のAgent Sessionは削除されないため、一つのTaskから複数のconversationを確認できます。
`everes task list`ではTaskごとに枠を分け、Taskの集約statusとsession数を表示し、その下に各sessionを並べます。項目名と本文は同じ行に置き、長い本文だけをTerminal幅に合わせて折り返すため、空行を挟む段組みより少ない行数で全体を確認できます。
未完了sessionはstatus、provider、label、summary、next actionまたはresume when、確認予定を表示します。done sessionは「どのsessionか・完了時のsummary・復帰command」だけの圧縮表示になります。通常のCLI出力は対話的に折り畳めないため、この圧縮を既定の折り畳み相当として扱い、詳細は `everes task show <task-id>` で確認します。
リンク対応ターミナルで `↗ codex:<label>` を選択すると、`codex://threads/<thread-id>` が開きます。Claude sessionには `everes session open` commandを表示します。復帰commandはprovider名と、そのprovider内で一意になるsession ID末尾（通常8文字）を表示します。`--session-id` は完全一致のほか、一意な先頭または末尾でも指定できます。
sessionごとのID、label、保存状態は `everes session list <task-id>` で確認します。

同じディレクトリで複数のAgent Sessionを開いている場合、自動検出では現在のsessionを一意に決められません。
SessionStart contextにproviderとsession IDがある場合、AIエージェントはその値を明示して紐付けます。

## 過去のAgent Sessionを再開する

Taskに紐づく過去のconversationを開きたい場合は、Taskと必要ならproviderを伝えます。

```text
認証方式変更のTaskに紐づく最新のCodex threadを再開して
```

AIエージェントはTaskに紐づくAgent Sessionを確認し、指定条件に合うsessionを開きます。
Codexでは `codex resume <session-id>`、Claude Codeでは `claude --resume <session-id>` が保存済みのcwdで実行されます。
Claude DesktopのCodeタブとClaude Code CLIは別のsession履歴を管理します。Everesが保持するClaude CLI session IDはCLIの `--resume` で再開します。CLI再開後に `/desktop` を実行するとDesktopへ移せますが、Desktop側で続いた履歴は元のCLI session IDと同一とは扱いません。

Agent側のsessionファイルが削除されている場合は、EveresにTaskが残っていても元のconversationを再開できません。
その場合はTaskのsummaryとnext actionを確認し、新しいAgent Sessionを紐付けて作業を続けます。

## 別のAgent Sessionへ依頼する

Codexアプリでは、AIエージェントに別のCodex taskの状態確認や追加依頼を任せられます。

```text
認証方式変更のTaskに紐づく「レビュー」セッションの状態を確認して
```

```text
そのセッションをCodexアプリで表示して
```

```text
そのセッションに「APIの互換性も確認してください」と送って
```

AIエージェントはEveresから対象thread IDを特定し、Codexアプリのtask参照・メッセージ送信機能を使います。
Codexアプリ内ではthreadごとに `active`、`idle`、`notLoaded` などの状態を取得できます。
これはEveres Taskの `active`、`waiting`、`needs_me`、`paused`、`done` とは別の状態です。
送信は通知の保存ではなく、対象taskにユーザーの追加指示を渡して新しいターンを開始する操作です。
そのため、宛先と依頼内容が明示された場合だけ実行します。

Claude Codeでは、Agent Teams内のteammateに対しては `SendMessage` を利用できます。
Agent Teamsに属さない独立したsessionには共通のmailboxはなく、`claude -p --resume <session-id> <prompt>` でsessionを再開して指示を実行します。
こちらも単なる通知ではなく、対象sessionで新しい処理を開始します。

Everesは宛先となるTaskとsession IDを管理しますが、独自のメッセージキューは持ちません。
CodexとClaude Codeの実行状態や権限確認は、それぞれの実行環境が管理します。

## Taskを完了する

作業の目的を達成したら完了を伝えます。

```text
この作業は完了
```

AIエージェントはstatusを `done` にします。
Taskの記録は残りますが、通常の未完了一覧には表示されなくなります。

## statusの使い分け

statusはAgent Sessionへ注意を戻す条件を表します。Taskのstatusは、active、needs_me、waiting、pausedの優先順で未完了sessionから集約します。全sessionがdoneになってもTaskは自動でdoneにはならず、Taskの完了は明示します。

| status | 選ぶ状況 | 一覧での扱い |
| --- | --- | --- |
| `active` | 現在作業を進めている | 未完了Taskとして表示されます |
| `waiting` | CI、レビュー、回答、仕様確定などを待っている | `active` より先に表示されます |
| `needs_me` | AIエージェントでは進められず、人間の判断や操作が必要 | 最初に表示されます |
| `paused` | 外部要因待ちではなく、意図的に止めている | `active` より後に表示されます |
| `done` | Taskの目的を達成した | 通常の一覧では非表示になります |

`NOT_SET` はstatusの種類ではなく、Taskとの紐付けはあるものの、そのAgent Sessionにstatusが記録されていないことを示す表示です。statusが未設定の旧データで表示され、Task statusの集約からは除外されます。

## CLIでTaskを横断して確認する

ターミナルで作業全体を俯瞰する場合は、Task一覧を表示します。

```bash
everes task list
```

一覧は `needs_me`、`waiting`、`active`、`paused` の順にグループ化されます。
完了したTaskは既定で表示されません。

特定のstatusだけを表示する場合は `--status` を使います。

```bash
everes task list --status needs_me
everes task list --status waiting
```

完了したTaskも含める場合は `--all` を使います。

```bash
everes task list --all
```

別のcommandやスクリプトへ渡す場合はJSONで出力します。

```bash
everes task list --json
everes task list --status waiting --json
everes task list --all --json
```

一つのTaskを詳しく確認する場合はTask IDを指定します。

```bash
everes task show <task-id>
everes task show <task-id> --json
```

Task IDは、他のTaskと区別できる長さまで先頭部分を指定できます。
前方一致するTaskが複数ある場合は、CLIが曖昧な指定として拒否します。

## CLIから直接更新する

対話を使わずにTaskを作成する場合は、titleとobjectiveを指定します。

```bash
everes task create \
  --title "認証方式の変更" \
  --objective "既存の認証方式を整理し、OAuthベースへ移行する"
```

現在のAgent Sessionを特定できれば、そのsessionもTaskへ紐付きます。
Taskだけを作成する場合は `--no-attach` を使います。

現在のAgent Sessionの現在地とstatusを更新する場合は、変更する項目だけを指定します。

```bash
everes session update <task-id> \
  --status waiting \
  --summary "実装とunit testが完了し、PRレビュー待ち" \
  --next-action "レビュー指摘を反映してintegration testを実行する" \
  --resume-when "PRにレビューが付いたら" \
  --check-at "2026-08-17 09:00" \
  --check-action "PRのレビュー状況を確認する"
```

timezoneを含まない `--check-at` は、コマンドを実行したMacのローカルtimezoneで解釈されます。
RFC 3339形式でoffsetを明示することもできます。
確認予定を消す場合は、両方を空にします。

```bash
everes session update <task-id> --check-at "" --check-action ""
```

referenceとnoteもCLIから追加できます。

```bash
everes ref add <task-id> \
  https://github.com/example/foo/issues/381 \
  --title "認証方式変更"

everes note add <task-id> -- refresh tokenについてはA案を採用した
```

Taskを完了する場合は次を実行します。

```bash
everes task done <task-id>
```

この操作は紐づく全Agent Sessionもdoneにします。一つのAgent Sessionだけを終える場合は `everes session done <task-id>` を使います。

実行前に全sessionの状態を表示して確認します。`y` または `yes` を入力すると完了し、それ以外や入力終了時は変更しません。確認を省略する場合は `--yes` を指定します。完了後にsessionをactiveなどに変更すると、Taskの状態も再集約され、完了日時は消去されます。

## CLIでAgent Sessionを扱う

現在のAgent Sessionに紐づくTaskを確認します。

```bash
everes session current
```

providerとsession IDが分かっている場合は明示できます。

```bash
everes session current \
  --provider codex \
  --session-id <session-id>
```

新しいAgent Sessionを既存Taskへ追加します。

```bash
everes session attach <task-id> \
  --label "レビュー指摘の反映" \
  --provider codex \
  --session-id <session-id>
```

Taskに紐づくAgent Sessionを一覧します。

```bash
everes session list <task-id>
everes session list <task-id> --json
```

Agent Sessionの状態を明示して更新できます。

```bash
everes session update <task-id> --status active --summary "レビュー指摘を反映中"
everes session done <task-id>
```

この一覧の `last_seen_at` はEveresへ最後に紐付けた時刻です。
provider側の保存データに更新時刻がある場合は `metadata.modified_at` へ出力されます。
Codexの `metadata.thread_source` は通常のtaskで `user` になります。subagentなど通常task以外のthreadは自動検出の対象外です。

誤ったAgent Sessionの紐付けだけを外す場合は次を実行します。CodexやClaude側のconversationは削除されません。

```bash
everes session detach <task-id> \
  --provider codex \
  --session-id <session-id>
```

最後に使われたAgent Sessionを再開します。

```bash
everes session open <task-id>
```

Codex CLIではなくCodexアプリの該当taskを表示する場合は `--app` を付けます。複数のsessionがあるときはlabelまたは `session list` でIDを確認し、`--session-id` で指定します。

```bash
everes session open <task-id> --app
everes session open <task-id> --app --session-id <session-id>
```

## 同時更新の競合を検出する

複数のAgent Sessionが同じTaskを更新するときは、読み取ったrevisionを `--revision` に渡せます。

```bash
everes task show <task-id> --json
everes task update <task-id> \
  --revision <revision> \
  --summary "更新後の状態"
```

保存済みのrevisionが指定値から進んでいる場合は `revision conflict` となり、更新されません。
競合が発生したら最新のTaskを読み直し、どの内容を残すか判断してから更新します。

## 利用前の確認と保存場所

CLIが利用できることを確認します。

```bash
everes version
```

バージョン番号が表示されれば利用できます。
導入方法は[README.md](README.md)を参照してください。

Everesのデータは既定で `~/.everes/` に保存されます。
保存先を変更する場合は `EVERES_HOME` を設定します。

```bash
export EVERES_HOME=/path/to/everes-data
```

`~/.everes/` 配下のJSONは直接編集せず、必ず `everes` コマンドを使います。
CLIを経由すると、入力の検証、排他制御、revisionの更新、schema migrationが適用されます。

## 問題が起きた場合

### `everes` が見つからない

次のコマンドで配置を確認します。

```bash
command -v everes
everes version
```

開発用のリンク配置を使っている場合は、リポジトリで次を実行します。

```bash
mise run dev-setup
```

### 現在のsessionを特定できない

SessionStart contextに表示された値を明示します。

```bash
everes session current \
  --provider <claude-or-codex> \
  --session-id <session-id>
```

Taskの作成やsessionの追加でも、同じ二つのoptionを指定できます。

### hookが実行されない

Codexでは `/hooks` を開き、EveresのSessionStart hookが有効か、信頼済みかを確認します。
hookが失敗と表示される場合は詳細を開きます。
`hook returned invalid session start JSON output` は、Codex用ではない古いEveres CLIがプレーンテキストを返している状態を表します。
CLIとAPM packageを同じversionへ更新してから、新しいAgent Sessionで確認します。
hook設定を変更した直後は、Claude CodeまたはCodexで新しいAgent Sessionを開始して確認します。

### Agent Sessionを再開できない

Taskに記録されているsessionを確認します。

```bash
everes session open <task-id> --list
```

Agent側のsessionファイルが削除されている場合は、新しいAgent SessionをTaskへ紐付けて作業を続けます。

## コマンド一覧

```text
everes task create --title <title> [--objective ...] [--summary ...] [--next-action ...] [--resume-when ...] [--check-at ...] [--check-action ...] [--status ...] [--provider ...] [--session-id ...] [--label ...] [--no-attach] [--json]
everes task list [--json] [--status <status>] [--all]
everes task show <task-id> [--json]
everes task update <task-id> [--title ...] [--objective ...] [--status ...] [--summary ...] [--next-action ...] [--resume-when ...] [--check-at ...] [--check-action ...] [--revision ...] [--json]
everes task done <task-id> [--yes] [--json]

everes session current [--provider ...] [--session-id ...] [--json]
everes session list <task-id> [--json]
everes session attach <task-id> [--label ...] [--provider ...] [--session-id ...] [--json]
everes session update <task-id> [--status ...] [--summary ...] [--next-action ...] [--resume-when ...] [--check-at ...] [--check-action ...] [--revision ...] [--provider ...] [--session-id ...] [--json]
everes session done <task-id> [--provider ...] [--session-id ...] [--json]
everes session detach <task-id> --provider <provider> --session-id <session-id> [--json]
everes session open <task-id> [--provider ...] [--session-id ...] [--list] [--app]

everes ref add <task-id> <url> [--title ...] [--type ...] [--json]
everes ref remove <task-id> <ref-id>

everes note add [--json] <task-id> -- <text>
everes note remove <task-id> <note-id>
```

`everes hook session-start` はSessionStart hookから呼ばれる内部コマンドです。
通常の操作では直接実行しません。
