package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tsuyukimakoto/everes/internal/provider"
	"github.com/tsuyukimakoto/everes/internal/task"
)

// renderList はstatusでグルーピングした一覧を書き出す。
//
// 画面装飾ではなく一覧性を優先する。1つのTaskについて
// 「何の作業か / 現在どうなっているか / 次に何をするか / 何待ちなのか」が
// 読み取れることを目標とする。
func renderList(w io.Writer, tasks []*task.Task, now time.Time) {
	renderListWithLinks(w, tasks, now, false)
}

// renderListWithLinks はCodex deep linkの有無を指定してTask一覧を書き出す。
func renderListWithLinks(w io.Writer, tasks []*task.Task, now time.Time, links bool) {
	if len(tasks) == 0 {
		fmt.Fprintln(w, "Taskがありません。everes task create で作成してください。")
		return
	}

	var current string
	first := true
	for _, t := range tasks {
		group := listGroup(t, now)
		if group != current {
			if !first {
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "%s\n\n", group)
			current = group
			first = false
		}
		renderListItem(w, t, now, links)
		fmt.Fprintln(w)
	}
}

func renderListItem(w io.Writer, t *task.Task, now time.Time, links bool) {
	fmt.Fprintf(w, "  %s  %s\n", task.ShortID(t.TaskID), t.Title)

	if t.Summary != "" {
		fmt.Fprintf(w, "    %s\n", t.Summary)
	}

	if t.CheckAt != nil {
		when := formatCheckAt(*t.CheckAt, now.Location())
		if t.CheckDue(now) {
			fmt.Fprintf(w, "    ! 確認時刻超過 (%s): %s\n", when, t.CheckAction)
		} else {
			fmt.Fprintf(w, "    確認予定 %s: %s\n", when, t.CheckAction)
		}
	}
	for _, s := range t.AgentSessions {
		if s.CheckAt == nil {
			continue
		}
		when := formatCheckAt(*s.CheckAt, now.Location())
		label := s.Label
		if label == "" {
			label = shortSessionID(s.SessionID)
		}
		if s.CheckDue(now) {
			fmt.Fprintf(w, "    ! %s 確認時刻超過 (%s): %s\n", label, when, s.CheckAction)
		} else {
			fmt.Fprintf(w, "    %s 確認予定 %s: %s\n", label, when, s.CheckAction)
		}
	}

	// waitingではresume_whenが、それ以外ではnext_actionが最も知りたい情報になる。
	if hint := listHint(t); hint != "" {
		fmt.Fprintf(w, "    → %s\n", hint)
	}

	if refs := refSummary(t); refs != "" {
		fmt.Fprintf(w, "    %s\n", refs)
	}

	if sessions := sessionSummary(t, links); sessions != "" {
		fmt.Fprintf(w, "    %s\n", sessions)
	}
	if statuses := sessionStatusSummary(t); statuses != "" {
		fmt.Fprintf(w, "    %s\n", statuses)
	}
	for _, resume := range claudeResumeCommands(t) {
		fmt.Fprintf(w, "      再開 %s: %s\n", sanitizeTerminalText(resume.Label), resume.Command)
	}

	meta := []string{relTime(t.Timestamps.UpdatedAt, now)}
	if providers := providerSummary(t); providers != "" {
		meta = append(meta, providers)
	}
	fmt.Fprintf(w, "    %s\n", strings.Join(meta, "  "))
}

type sessionResumeCommand struct {
	Label   string
	Command string
}

func claudeResumeCommands(t *task.Task) []sessionResumeCommand {
	commands := make([]sessionResumeCommand, 0)
	for _, attached := range t.AgentSessions {
		if attached.Provider != "claude" {
			continue
		}
		label := attached.Label
		if label == "" {
			label = shortSessionID(attached.SessionID)
		}
		commands = append(commands, sessionResumeCommand{
			Label: label,
			Command: fmt.Sprintf("everes session open %s --provider claude --session-id %s",
				task.ShortID(t.TaskID), shellWord(attached.SessionID)),
		})
	}
	return commands
}

func shellWord(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || strings.ContainsRune("-._:/", r))
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func sessionSummary(t *task.Task, links bool) string {
	if len(t.AgentSessions) == 0 {
		return ""
	}
	items := make([]string, 0, len(t.AgentSessions))
	for _, attached := range t.AgentSessions {
		label := attached.Label
		if label == "" {
			label = shortSessionID(attached.SessionID)
		}
		text := attached.Provider + ":" + sanitizeTerminalText(label) + " [" + displaySessionStatus(attached.Status) + "]"
		appURL, err := provider.SessionAppURL(provider.Session{
			Provider:  attached.Provider,
			SessionID: attached.SessionID,
			Cwd:       attached.Cwd,
			Label:     attached.Label,
		})
		if err == nil {
			text = "↗ " + text
			if links {
				text = terminalLink(appURL, text)
			}
		}
		items = append(items, text)
	}
	return "sessions: " + strings.Join(items, "  ")
}

func sessionStatusSummary(t *task.Task) string {
	counts := map[task.Status]int{}
	for _, s := range t.AgentSessions {
		counts[s.Status]++
	}
	order := []task.Status{task.StatusActive, task.StatusNeedsMe, task.StatusWaiting, task.StatusPaused, task.StatusDone, ""}
	var items []string
	for _, status := range order {
		if counts[status] > 0 {
			items = append(items, fmt.Sprintf("%d %s", counts[status], displaySessionStatus(status)))
		}
	}
	if len(items) == 0 {
		return ""
	}
	return "session status: " + strings.Join(items, " · ")
}

func terminalLink(target, label string) string {
	return "\x1b]8;;" + target + "\x1b\\" + label + "\x1b]8;;\x1b\\"
}

func sanitizeTerminalText(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
}

func shortSessionID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func listGroup(t *task.Task, now time.Time) string {
	if t.CheckDue(now) {
		return "WAITING — CHECK NOW"
	}
	return t.Status.Label()
}

func listHint(t *task.Task) string {
	if t.Status == task.StatusWaiting && t.ResumeWhen != "" {
		return t.ResumeWhen
	}
	if t.Status == task.StatusWaiting {
		for _, s := range t.AgentSessions {
			if s.Status == task.StatusWaiting && s.ResumeWhen != "" {
				return s.ResumeWhen
			}
		}
	}
	if t.NextAction != "" {
		return t.NextAction
	}
	return t.ResumeWhen
}

func refSummary(t *task.Task) string {
	if len(t.References) == 0 {
		return ""
	}
	titles := make([]string, 0, len(t.References))
	for _, r := range t.References {
		titles = append(titles, r.Title)
	}
	return strings.Join(titles, " / ")
}

func providerSummary(t *task.Task) string {
	counts := map[string]int{}
	var names []string
	for _, s := range t.AgentSessions {
		if counts[s.Provider] == 0 {
			names = append(names, s.Provider)
		}
		counts[s.Provider]++
	}
	if len(names) == 0 {
		return ""
	}
	labels := make([]string, 0, len(names))
	for _, name := range names {
		if counts[name] == 1 {
			labels = append(labels, name)
		} else {
			labels = append(labels, fmt.Sprintf("%s×%d", name, counts[name]))
		}
	}
	return "[" + strings.Join(labels, ",") + "]"
}

// renderDetail はTaskの詳細を書き出す。
func renderDetail(w io.Writer, t *task.Task, now time.Time) {
	fmt.Fprintf(w, "%s\n", t.Title)
	fmt.Fprintf(w, "%s  (revision %d)\n\n", t.TaskID, t.Revision)

	writeField(w, "Objective", t.Objective)
	writeField(w, "Status", string(t.Status)+" (session aggregate)")
	writeField(w, "Summary", t.Summary)
	writeField(w, "Next Action", t.NextAction)
	writeField(w, "Resume When", t.ResumeWhen)
	if t.CheckAt != nil {
		writeField(w, "Check At", formatCheckAt(*t.CheckAt, now.Location()))
		writeField(w, "Check Action", t.CheckAction)
		if t.CheckDue(now) {
			writeField(w, "Attention", "確認時刻を過ぎています")
		}
	}

	if len(t.References) > 0 {
		fmt.Fprintln(w, "\nReferences")
		for _, r := range t.References {
			fmt.Fprintf(w, "  %s  [%s] %s\n", r.ID, r.Type, r.Title)
			fmt.Fprintf(w, "        %s\n", r.URL)
		}
	}

	if ws := t.Workspace; ws.Cwd != "" || ws.RepoRoot != "" {
		fmt.Fprintln(w, "\nWorkspace")
		writeSubField(w, "Repository", ws.GitRemote)
		writeSubField(w, "Repo Root", ws.RepoRoot)
		writeSubField(w, "Branch", ws.Branch)
		writeSubField(w, "Worktree", ws.Worktree)
		writeSubField(w, "cwd", ws.Cwd)
	}

	if len(t.AgentSessions) > 0 {
		fmt.Fprintln(w, "\nAgent Sessions")
		for _, s := range t.AgentSessions {
			label := s.Label
			if label == "" {
				label = "(no label)"
			}
			fmt.Fprintf(w, "  [%s] %s\n", s.Provider, s.SessionID)
			fmt.Fprintf(w, "        %s  status=%s  %s\n", label, displaySessionStatus(s.Status), relTime(s.LastSeenAt, now))
			if s.Summary != "" {
				fmt.Fprintf(w, "        Summary: %s\n", s.Summary)
			}
			if s.NextAction != "" {
				fmt.Fprintf(w, "        Next Action: %s\n", s.NextAction)
			}
			if s.ResumeWhen != "" {
				fmt.Fprintf(w, "        Resume When: %s\n", s.ResumeWhen)
			}
			if s.CheckAt != nil {
				fmt.Fprintf(w, "        Check At: %s\n", formatCheckAt(*s.CheckAt, now.Location()))
				fmt.Fprintf(w, "        Check: %s\n", s.CheckAction)
			}
		}
		fmt.Fprintf(w, "\n  再開: everes session open %s\n", task.ShortID(t.TaskID))
	}

	if len(t.Notes) > 0 {
		fmt.Fprintln(w, "\nNotes")
		for _, n := range t.Notes {
			fmt.Fprintf(w, "  %s  %s\n", n.ID, n.Text)
			fmt.Fprintf(w, "        %s\n", n.CreatedAt.Format(time.RFC3339))
		}
	}

	fmt.Fprintf(w, "\nCreated  %s\n", t.Timestamps.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(w, "Updated  %s\n", t.Timestamps.UpdatedAt.Format(time.RFC3339))
	if t.Timestamps.CompletedAt != nil {
		fmt.Fprintf(w, "Completed %s\n", t.Timestamps.CompletedAt.Format(time.RFC3339))
	}
}

func formatCheckAt(value time.Time, location *time.Location) string {
	return value.In(location).Format("2006-01-02 15:04 MST")
}

func writeField(w io.Writer, label, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(w, "%-12s %s\n", label, value)
}

func writeSubField(w io.Writer, label, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(w, "  %-11s %s\n", label, value)
}

// relTime は更新からの経過を人間向けに表す。
func relTime(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour")
	default:
		return plural(int(d.Hours()/24), "day")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s ago", unit)
	}
	return fmt.Sprintf("%d %ss ago", n, unit)
}
