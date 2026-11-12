---
name: everes-task
description: >
  Create, update, and inspect Tasks in Everes, a Task Context Manager for parallel AI coding work.
  Use this skill when the user asks to manage work in Everes; record or inspect its objective, progress,
  waiting state, next action, related links, or notes; mark work complete; or review current and other
  ongoing work. Change a Task to waiting only when the user explicitly mentions Everes, Task, status,
  waiting, or an equivalent term. A request such as "wait for review" may refer to an external service
  and is not enough by itself to update Everes. Use this skill to regain context when a conversation has
  become long. Never edit Everes data directly; always use the everes CLI.
---

# everes-task

Everes helps people track work running concurrently across multiple AI coding agents without relying on memory.

These instructions are written in English, but communicate with the user in the user's language. Localize questions and option labels, and write Task titles, objectives, summaries, next actions, resume conditions, and notes in the language appropriate to the user's work unless the user requests otherwise.

The primary object is a **Task**. A Claude Code session or Codex thread is an **Agent Session** that works on a Task.

Store progress, status, next action, resume condition, and scheduled checks on the current Agent Session. A Task status is aggregated from its Agent Sessions. Store the objective and shared information on the Task, and the current position of each conversation on its Agent Session.

The goal is for Everes to preserve what the work is, where it stands, and what happens next even after the original request has scrolled out of the conversation.

## Non-negotiable rules

**Never read or write JSON files under `~/.everes/` directly.**

Always use the `everes` command. The CLI owns validation, locking, session mapping, and schema migration. Direct edits bypass those safeguards and may corrupt data when agents update it concurrently.

**Never infer a status change, and never leave the current snapshot stale.**

Change `status`, complete a Task, or change an objective only when the user explicitly requests or confirms it. When work has been delegated to another session, or this session's assignment appears complete, ask briefly whether this session should become `waiting`, `paused`, or `done` before ending the work. Do not leave an obsolete `active` status without asking.

For a session already managed by Everes, `summary` and `next_action` are replaceable snapshots of the current conversation. Update them with `everes session update` when they become stale at a meaningful progress point, before delegating, at a long-work boundary, or before the final response. This is not an append-only history. Put only lasting decisions or rationale in notes.

Do not rewrite snapshots without actual progress or update them every turn. The skill cannot run on a wall-clock schedule, so "periodically" means at meaningful work boundaries.

**Do not treat “waiting for review” as an instruction to update Everes.**

Such wording may request an operation on a GitHub issue or pull request. Set Everes to `waiting` only when the user explicitly refers to Everes, a Task, a status, waiting, or an equivalent term. If the target is ambiguous, ask what should be changed.

## Prerequisites

Use the `everes` CLI bundled by APM. Select the absolute path for the current provider. In every command example below, replace `everes` with the appropriate absolute path. Do not rely on the shell `PATH` or `~/bin/everes`.

```text
Claude Code: $HOME/.claude/skills/everes-task/bin/everes
Codex:       $HOME/.agents/skills/everes-task/bin/everes
```

If the corresponding file is missing, ask the user to install the APM package.

```bash
everes version
```

When SessionStart context includes the provider and session ID in either form below, pass those values explicitly to the CLI.

```text
[Everes] This session (codex <session-id>) ...
[Everes] This session (claude <session-id>) ...
```

Timestamp-based auto-detection cannot distinguish concurrent sessions in the same working directory. Never fall back to auto-detection when the context contains explicit values.

When SessionStart reports an unregistered session, ask how to attach it to a Task in the first response, before proceeding with the ordinary request. Compare the first user message with every unfinished candidate's title, objective, summary, next action, and resume condition. Mark one existing Task as **Recommended** when it clearly matches, but keep every unfinished Task available.

Also propose one concise new Task title based on the first user message. Include options to create a Task with the proposed title, create one with a title entered by the user, or leave the session unattached. If no unfinished Tasks exist, ask whether to create a new Task and present those same three options. Never ask the user to type a Task ID.

Run `everes session attach` only after the user selects an existing Task, using the Task ID, provider, and session ID from context. Create a new Task only after the user selects a new-Task option. Do not create, update, or attach anything before the selection.

## Map user requests to operations

### “Manage this work in Everes”

Create a Task and attach the current session. Derive an initial title and objective from the conversation. **The agent may infer initial values only.**

```bash
everes task create \
  --title "Change the authentication method" \
  --objective "Replace the existing authentication method with an OAuth-based flow" \
  --provider <provider> \
  --session-id <session-id>
```

Use the provider and session ID from SessionStart context. Use auto-detection only when context does not contain them. The CLI captures workspace information such as cwd, Git remote, branch, and worktree automatically.

After creation, show the title and objective and ask whether they match the user's intent.

### “Update the current state”

Summarize the conversation's current state and update the session.

```bash
everes session update <task-id> \
  --status waiting \
  --summary "OAuth implementation and unit tests are complete; the pull request is awaiting review." \
  --next-action "Address review feedback and run integration tests" \
  --resume-when "PR #381 receives a review"
```

Keep the fields distinct.

| Field | Meaning | Example |
| --- | --- | --- |
| `--summary` | Current state | OAuth implementation and unit tests are complete; review is pending |
| `--next-action` | Concrete next action | Address review feedback and run integration tests |
| `--resume-when` | Condition for returning | PR #381 receives a review |
| `--check-at` | Local date and time for the next check | 2026-08-17 09:00 |
| `--check-action` | What to inspect after that time | Check the review status of PR #381 |

This replaces the current Agent Session snapshot; it does not append history. Keep `summary` short but sufficient to show what this session owns, what it completed, and where it now stands.

## Snapshot and status checks during work

While working in an attached session, verify that `summary` and `next_action` still match the conversation at these points:

- The current position changes, such as after tests pass or an implementation approach is settled
- Immediately before delegating work to another Agent Session
- At a boundary in a long-running task
- Before the final response

If stale, replace the snapshot with `everes session update` without changing the status. Include what is complete, what remains, and the next concrete action.

Do not change status automatically in these cases; ask the user briefly:

- Work was delegated and this session has nothing left in progress: ask whether it should be `waiting`, `paused`, or `done`
- This session's assignment is complete: ask whether to mark the session `done`
- The whole Task objective is complete: ask whether to mark the Task `done`

If the user specifies the status in the same message, apply it without asking again.

### “Set this Task to waiting in Everes”

Set the status and resume condition. For event-based waiting, record `--resume-when`. For a scheduled check, record both `--check-at` and `--check-action`. A waiting Task is not useful if the list cannot show what it awaits or what must be checked.

```bash
everes session update <task-id> --status waiting --resume-when "The review is returned" --provider <provider> --session-id <session-id>
```

Resolve a user-supplied local date and time in the environment's timezone and record the action to perform.

```bash
everes session update <task-id> \
  --status waiting \
  --check-at "2026-08-17 09:00" \
  --check-action "Check the review status of PR #381" \
  --provider <provider> \
  --session-id <session-id>
```

Resolve relative dates from the current local date and time. Ask when a unique timestamp cannot be determined. Never set `--check-at` without `--check-action`. Passing the check time does not change `waiting`; the Task appears under `WAITING — CHECK NOW`. After checking, follow the user's instruction to resume the Task or schedule another check.

Allowed statuses:

| Status | Meaning |
| --- | --- |
| `active` | Work is in progress |
| `waiting` | Waiting for an external event such as CI, review, another person, or a specification decision |
| `needs_me` | Human judgment or action is required |
| `paused` | Work was intentionally suspended |
| `done` | The objective has been achieved |

### “Attach this issue/document”

Add its URL as a reference. The CLI infers the reference type from the URL.

```bash
everes ref add <task-id> https://github.com/example/foo/issues/381 --title "Authentication change"
```

If the CLI warns that a query parameter may contain a token or secret, relay the warning and ask before storing the URL.

### “Make a note of this”

Record lasting rationale or information that should be available later.

```bash
everes note add <task-id> -- Use approach A for refresh tokens
```

Use `--` to mark the start of note text so words beginning with a hyphen remain intact.

### “This work is complete”

```bash
everes task done <task-id> --yes
```

Completing a Task marks all attached Agent Sessions done. Use `--yes` only after the user explicitly requests or confirms completion of the whole Task and its sessions. To complete only one session, run `everes session done <task-id> --provider <provider> --session-id <session-id>`.

### “What am I working on?”

```bash
everes task list
```

Tasks are grouped by status, starting with `needs_me`. Completed Tasks are hidden by default; pass `--all` to include them.

### “Where does this work stand?”

Show the Task attached to the current session.

```bash
everes session current --provider <provider> --session-id <session-id>
```

Run this first when a long conversation has obscured the original objective.

### “Show the sessions attached to this Task”

```bash
everes session list <task-id> --json
```

`last_seen_at` is when Everes last attached or observed the session; it does not mean the agent is currently running. Use provider state from `exists` and `metadata`. Never infer running, waiting, or completion from a saved file's modification time.

## Inspecting and operating another session

When the user asks about another Agent Session, first identify it with `everes session list <task-id> --json`.

If the Codex app provides thread inspection, read the target thread by ID. If the user asks to show it, use the app's task navigation with that thread ID. From the CLI, run `everes session open <task-id> --app --session-id <session-id>`. The `↗ codex:<label>` link in `everes task list` opens the same deep link.

Claude Desktop and Claude Code CLI keep separate session histories. Resume an attached Claude CLI session with `everes session open <task-id> --provider claude --session-id <session-id>` and give the user the same command shown by `task list`. Never claim that Everes can navigate directly to an existing Claude Desktop session.

An Everes Agent Session `status` describes the work assigned to that conversation. A Task `status` is the aggregate of Agent Session statuses. Both are separate from provider runtime states such as Codex app `active`, `idle`, or `notLoaded`.

For an independent Claude Code session, Everes can report only whether the saved session exists and when it was last modified. Inside Claude Agent Teams, use Agent Teams facilities to inspect team tasks and teammates.

### Sending a request to another session

Send only when the user explicitly specifies both the destination and the request. This operation adds a new user instruction to the target session and starts agent processing; it is not a mailbox note.

- In the Codex app, inspect the target thread when needed and use the app's follow-up-message facility
- In Claude Agent Teams, use `SendMessage`
- For an independent Claude Code session, use `claude -p --resume <session-id> <prompt>` when the user explicitly asks it to perform work

If multiple destinations match, show their labels, providers, and session IDs and ask the user to choose. If the environment cannot send messages, say so and offer `everes session open <task-id>` for interactive resumption. Never imply that Everes has its own mailbox.

## Handing off to a new session

When a long conversation moves to a new session, attach the new session to the existing Task. Do not create a replacement Task.

If SessionStart context already lists unfinished candidates, let the user select one by number or title. Search the Task list only when the requested Task is not among the candidates.

```bash
everes session attach <task-id> \
  --label "Address review feedback" \
  --provider <provider> \
  --session-id <session-id>
```

If the wrong session was attached, detach it using an explicit provider and session ID. This does not delete the provider conversation.

```bash
everes session detach <task-id> \
  --provider <provider> \
  --session-id <session-id>
```

Old sessions remain attached. Multiple Agent Sessions may coexist on one Task.

## Machine-readable output

Use `--json` when structured data is required for a decision.

```bash
everes session current --json
everes session list <task-id> --json
everes task show <task-id> --json
everes task list --json
```

## Task IDs

A unique prefix such as the first eight characters is accepted. An ambiguous prefix produces an error with matching candidates.

```bash
everes task show 01M04EKP
```

## Revision conflicts

When the CLI reports `revision conflict`, another session updated the Task first. Read the latest state with `everes task show`, then ask whether the user wants to overwrite it. Never retry silently.

## Command reference

```text
everes task create --title <title> [--objective ...] [--status ...] [--provider ...] [--session-id ...] [--no-attach]
everes task list [--json] [--status <status>] [--all]
everes task show <task-id> [--json]
everes task update <task-id> [--status ...] [--summary ...] [--next-action ...] [--resume-when ...]
everes task done <task-id> [--yes]

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

`everes hook session-start` is an internal SessionStart hook command. Do not invoke it manually.
