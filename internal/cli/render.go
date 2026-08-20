package cli

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/tsuyukimakoto/everes/internal/provider"
	"github.com/tsuyukimakoto/everes/internal/task"
)

const defaultListWidth = 100

// renderList はstatusでグルーピングした一覧を書き出す。
//
// 画面装飾ではなく一覧性を優先する。1つのTaskについて
// 「何の作業か / 現在どうなっているか / 次に何をするか / 何待ちなのか」が
// 読み取れることを目標とする。
func renderList(w io.Writer, tasks []*task.Task, now time.Time) {
	renderListWithWidth(w, tasks, now, false, defaultListWidth)
}

// renderListWithLinks はCodex deep linkの有無を指定してTask一覧を書き出す。
func renderListWithLinks(w io.Writer, tasks []*task.Task, now time.Time, links bool) {
	renderListWithWidth(w, tasks, now, links, defaultListWidth)
}

// renderListWithWidth はTerminalの表示幅を指定してTask一覧を書き出す。
func renderListWithWidth(w io.Writer, tasks []*task.Task, now time.Time, links bool, width int) {
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
		renderListItem(w, t, now, links, width)
		fmt.Fprintln(w)
	}
}

func renderListItem(w io.Writer, t *task.Task, now time.Time, links bool, width int) {
	fmt.Fprintf(w, "╭─ %s  %s\n", task.ShortID(t.TaskID), t.Title)
	fmt.Fprintf(w, "│  Task  %s (%s) · %s\n",
		t.Status.Label(), sessionCountLabel(len(t.AgentSessions)), relTime(t.Timestamps.UpdatedAt, now))

	fields := taskListFields(t, now)
	for _, field := range fields {
		fmt.Fprintln(w, "│")
		writeStagedField(w, "│  ", "│    ", field, width)
	}

	if len(t.AgentSessions) > 0 {
		fmt.Fprintln(w, "├─ Sessions")
		for _, attached := range t.AgentSessions {
			fmt.Fprintln(w, "│")
			renderSessionListItem(w, t, attached, now, links, width)
		}
	}
	fmt.Fprintln(w, "╰─")
}

type listField struct {
	Label string
	Value string
}

func taskListFields(t *task.Task, now time.Time) []listField {
	var fields []listField
	if t.Summary != "" {
		fields = append(fields, listField{Label: "現在地", Value: t.Summary})
	}
	if label, hint := listHint(t); hint != "" {
		fields = append(fields, listField{Label: label, Value: hint})
	}
	if t.CheckAt != nil {
		label := "確認予定"
		if t.CheckDue(now) {
			label = "! 確認時刻超過"
		}
		fields = append(fields, listField{
			Label: label,
			Value: formatCheckAt(*t.CheckAt, now.Location()) + ": " + t.CheckAction,
		})
	}
	if refs := refSummary(t); refs != "" {
		fields = append(fields, listField{Label: "参照", Value: refs})
	}
	return fields
}

func sessionCountLabel(count int) string {
	switch count {
	case 0:
		return "no sessions"
	case 1:
		return "1 session"
	default:
		return fmt.Sprintf("%d sessions", count)
	}
}

func renderSessionListItem(w io.Writer, t *task.Task, attached task.AgentSession, now time.Time, links bool, width int) {
	status := strings.ToUpper(displaySessionStatus(attached.Status))
	name := sessionDisplayName(attached, links)
	fmt.Fprintf(w, "│  ┌─ %s  %s\n", status, name)

	fields := sessionListFields(attached, now)
	if resume, ok := claudeResumeCommand(t, attached); ok {
		fields = append(fields, listField{Label: "再開", Value: resume.Command})
	}
	for i, field := range fields {
		if i > 0 {
			fmt.Fprintln(w, "│  │")
		}
		writeStagedField(w, "│  │  ", "│  │    ", field, width)
	}
	fmt.Fprintln(w, "│  └─")
}

func sessionListFields(attached task.AgentSession, now time.Time) []listField {
	var fields []listField
	if attached.Status == "" {
		fields = append(fields, listField{Label: "状態", Value: "status未設定"})
	}
	if attached.Summary != "" {
		fields = append(fields, listField{Label: "現在地", Value: attached.Summary})
	}
	switch attached.Status {
	case task.StatusWaiting, task.StatusPaused:
		if attached.ResumeWhen != "" {
			fields = append(fields, listField{Label: "再開条件", Value: attached.ResumeWhen})
		} else if attached.NextAction != "" {
			fields = append(fields, listField{Label: "次の作業", Value: attached.NextAction})
		}
	default:
		if attached.NextAction != "" {
			fields = append(fields, listField{Label: "次の作業", Value: attached.NextAction})
		}
	}
	if attached.CheckAt != nil {
		label := "確認予定"
		if attached.CheckDue(now) {
			label = "! 確認時刻超過"
		}
		fields = append(fields, listField{
			Label: label,
			Value: formatCheckAt(*attached.CheckAt, now.Location()) + ": " + attached.CheckAction,
		})
	}
	return fields
}

func sessionDisplayName(attached task.AgentSession, links bool) string {
	label := attached.Label
	if label == "" {
		label = shortSessionID(attached.SessionID)
	}
	text := attached.Provider + ":" + sanitizeTerminalText(label)
	appURL, err := provider.SessionAppURL(provider.Session{
		Provider:  attached.Provider,
		SessionID: attached.SessionID,
		Cwd:       attached.Cwd,
		Label:     attached.Label,
	})
	if err != nil {
		return text
	}
	text = "↗ " + text
	if links {
		return terminalLink(appURL, text)
	}
	return text
}

type sessionResumeCommand struct {
	Label   string
	Command string
}

func claudeResumeCommand(t *task.Task, attached task.AgentSession) (sessionResumeCommand, bool) {
	if attached.Provider != "claude" {
		return sessionResumeCommand{}, false
	}
	label := attached.Label
	if label == "" {
		label = shortSessionID(attached.SessionID)
	}
	return sessionResumeCommand{
		Label: label,
		Command: fmt.Sprintf("everes session open %s --provider claude --session-id %s",
			task.ShortID(t.TaskID), shellWord(attached.SessionID)),
	}, true
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

func writeStagedField(w io.Writer, labelPrefix, valuePrefix string, field listField, width int) {
	fmt.Fprintf(w, "%s%s\n", labelPrefix, sanitizeTerminalText(field.Label))
	available := width - terminalDisplayWidth(valuePrefix)
	for _, line := range wrapTerminalText(sanitizeTerminalText(field.Value), available) {
		fmt.Fprintf(w, "%s%s\n", valuePrefix, line)
	}
}

func wrapTerminalText(value string, width int) []string {
	if value == "" {
		return []string{""}
	}
	if width < 1 {
		width = 1
	}
	var lines []string
	var line []rune
	lineWidth := 0
	for _, r := range value {
		runeWidth := terminalRuneWidth(r)
		if lineWidth > 0 && lineWidth+runeWidth > width {
			if prohibitedAtLineStart(r) && len(line) > 1 {
				last := line[len(line)-1]
				line = line[:len(line)-1]
				lines = append(lines, strings.TrimRight(string(line), " "))
				line = []rune{last}
				lineWidth = terminalRuneWidth(last)
			} else {
				lines = append(lines, strings.TrimRight(string(line), " "))
				line = nil
				lineWidth = 0
			}
		}
		if lineWidth == 0 && unicode.IsSpace(r) {
			continue
		}
		line = append(line, r)
		lineWidth += runeWidth
	}
	if len(line) > 0 {
		lines = append(lines, strings.TrimRight(string(line), " "))
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func prohibitedAtLineStart(r rune) bool {
	return strings.ContainsRune("、。，．・：；？！ー〜～…‥）〕］｝〉》」』】〙〗〟’”ゝゞヽヾ々", r)
}

func terminalDisplayWidth(value string) int {
	width := 0
	for _, r := range value {
		width += terminalRuneWidth(r)
	}
	return width
}

func terminalRuneWidth(r rune) int {
	if r == 0 || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r) {
		return 0
	}
	if r < 0x20 || (r >= 0x7f && r < 0xa0) {
		return 0
	}
	if r >= 0x1100 && (r <= 0x115f || r == 0x2329 || r == 0x232a ||
		(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
		(r >= 0xac00 && r <= 0xd7a3) || (r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) || (r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) || (r >= 0xffe0 && r <= 0xffe6) ||
		(r >= 0x1f300 && r <= 0x1faff) || (r >= 0x20000 && r <= 0x3fffd)) {
		return 2
	}
	return 1
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

func listHint(t *task.Task) (string, string) {
	if t.Status == task.StatusWaiting && t.ResumeWhen != "" {
		return "再開条件", t.ResumeWhen
	}
	if t.NextAction != "" {
		return "次の作業", t.NextAction
	}
	if t.ResumeWhen != "" {
		return "再開条件", t.ResumeWhen
	}
	return "", ""
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
