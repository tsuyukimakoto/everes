package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/tsuyukimakoto/everes/internal/provider"
	"github.com/tsuyukimakoto/everes/internal/store"
	"github.com/tsuyukimakoto/everes/internal/task"
)

func runTask(args []string) error {
	if len(args) == 0 {
		return errors.New("task requires a subcommand (create, list, show, update, done)")
	}
	switch args[0] {
	case "create":
		return runTaskCreate(args[1:])
	case "list":
		return runTaskList(args[1:])
	case "show":
		return runTaskShow(args[1:])
	case "update":
		return runTaskUpdate(args[1:])
	case "done":
		return runTaskDone(args[1:])
	default:
		return fmt.Errorf("unknown task subcommand %q", args[0])
	}
}

func runTaskCreate(args []string) error {
	fs := newFlagSet("task create", os.Stderr)
	title := fs.String("title", "", "Task title (required)")
	objective := fs.String("objective", "", "objective of the work")
	summary := fs.String("summary", "", "current state")
	nextAction := fs.String("next-action", "", "next concrete action")
	resumeWhen := fs.String("resume-when", "", "condition for resuming the Task")
	checkAt := fs.String("check-at", "", "local date and time for the next waiting check")
	checkAction := fs.String("check-action", "", "action to perform when the check time arrives")
	statusName := fs.String("status", string(task.StatusActive), "status (active, waiting, needs_me, paused, done)")
	sessionID := fs.String("session-id", "", "Agent Session ID to attach")
	providerName := fs.String("provider", "", "Agent provider to attach (claude, codex)")
	label := fs.String("label", "", "Agent Session label")
	noAttach := fs.Bool("no-attach", false, "do not attach the current session")
	asJSON := fs.Bool("json", false, "output JSON")

	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *title == "" {
		return errors.New("--title is required")
	}

	status, err := task.ParseStatus(*statusName)
	if err != nil {
		return err
	}

	t, err := task.New(*title)
	if err != nil {
		return err
	}
	t.Objective = *objective
	t.Summary = *summary
	t.NextAction = *nextAction
	t.ResumeWhen = *resumeWhen
	t.SetStatus(status)
	if *checkAt != "" {
		parsed, err := parseCheckAt(*checkAt, time.Local)
		if err != nil {
			return err
		}
		t.CheckAt = &parsed
	}
	t.CheckAction = *checkAction

	cwd := currentDir()
	t.Workspace = detectWorkspace(cwd)

	s, err := openStore()
	if err != nil {
		return err
	}

	// sessionの紐付けはTaskの保存前に決める。保存後に失敗して
	// 紐付けのないTaskだけが残る状況を避ける。
	var attached *provider.Session
	if !*noAttach {
		attached, err = resolveSession(*providerName, *sessionID, cwd)
		if err != nil && !errors.Is(err, provider.ErrNoCurrentSession) {
			return err
		}
	}

	if attached != nil {
		t.AttachSession(task.AgentSession{
			Provider:    attached.Provider,
			SessionID:   attached.SessionID,
			Cwd:         attached.Cwd,
			Label:       *label,
			Status:      status,
			Summary:     *summary,
			NextAction:  *nextAction,
			ResumeWhen:  *resumeWhen,
			CheckAt:     t.CheckAt,
			CheckAction: *checkAction,
		})
		if status == task.StatusDone {
			t.Complete()
		}
	}

	if err := s.Create(t); err != nil {
		return err
	}

	if attached != nil {
		if err := s.PutSession(store.SessionIndex{
			Provider:  attached.Provider,
			SessionID: attached.SessionID,
			TaskID:    t.TaskID,
			Cwd:       attached.Cwd,
		}); err != nil {
			return err
		}
	}

	if *asJSON {
		return writeJSON(os.Stdout, t)
	}

	fmt.Printf("created %s  %s\n", t.TaskID, t.Title)
	if attached != nil {
		fmt.Printf("attached %s session %s\n", attached.Provider, attached.SessionID)
	} else {
		fmt.Println("Could not identify the current session. Attach it later with everes session attach.")
	}
	return nil
}

func runTaskList(args []string) error {
	fs := newFlagSet("task list", os.Stderr)
	asJSON := fs.Bool("json", false, "output JSON")
	statusFilter := fs.String("status", "", "show only the specified status")
	all := fs.Bool("all", false, "include completed Tasks")

	if err := parseFlags(fs, args); err != nil {
		return err
	}

	s, err := openStore()
	if err != nil {
		return err
	}
	tasks, err := s.List()
	if err != nil {
		return err
	}

	if *statusFilter != "" {
		want, err := task.ParseStatus(*statusFilter)
		if err != nil {
			return err
		}
		tasks = filterTasks(tasks, func(t *task.Task) bool { return t.Status == want })
	} else if !*all {
		// 完了済みは既定で隠す。一覧は「いま抱えている作業」を映す面とする。
		tasks = filterTasks(tasks, func(t *task.Task) bool { return t.Status != task.StatusDone })
	}

	if *asJSON {
		if tasks == nil {
			tasks = []*task.Task{}
		}
		return writeJSON(os.Stdout, tasks)
	}

	renderListWithWidth(os.Stdout, tasks, time.Now(), stdoutIsTerminal(), listTerminalWidth())
	return nil
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func listTerminalWidth() int {
	if columns := os.Getenv("COLUMNS"); columns != "" {
		var width int
		if _, err := fmt.Sscanf(columns, "%d", &width); err == nil && width >= 20 {
			return width
		}
	}
	if stdoutIsTerminal() {
		cmd := exec.Command("stty", "size")
		cmd.Stdin = os.Stdin
		if out, err := cmd.Output(); err == nil {
			var rows, width int
			if _, err := fmt.Sscanf(string(out), "%d %d", &rows, &width); err == nil && width >= 20 {
				return width
			}
		}
	}
	return defaultListWidth
}

func runTaskShow(args []string) error {
	fs := newFlagSet("task show", os.Stderr)
	asJSON := fs.Bool("json", false, "output JSON")

	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ref, err := requireArg(fs, 0, "task-id")
	if err != nil {
		return err
	}

	s, err := openStore()
	if err != nil {
		return err
	}
	t, err := resolveTask(s, ref)
	if err != nil {
		return err
	}

	if *asJSON {
		return writeJSON(os.Stdout, t)
	}
	renderDetail(os.Stdout, t, time.Now())
	return nil
}

func runTaskUpdate(args []string) error {
	fs := newFlagSet("task update", os.Stderr)
	title := fs.String("title", "", "replace the title")
	objective := fs.String("objective", "", "replace the objective")
	summary := fs.String("summary", "", "replace the current-state summary")
	nextAction := fs.String("next-action", "", "replace the next action")
	resumeWhen := fs.String("resume-when", "", "replace the resume condition")
	checkAt := fs.String("check-at", "", "replace the next local check time (empty string clears it)")
	checkAction := fs.String("check-action", "", "replace the scheduled check action (empty string clears it)")
	statusName := fs.String("status", "", "change the status")
	revision := fs.Int("revision", 0, "update only when this value matches the stored revision")
	asJSON := fs.Bool("json", false, "output JSON")

	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ref, err := requireArg(fs, 0, "task-id")
	if err != nil {
		return err
	}

	// 明示的に渡されたフラグだけを反映する。空文字での消去と
	// 「指定しなかった」を区別するため、Visitで判定する。
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	var parsedCheckAt *time.Time
	if set["check-at"] && *checkAt != "" {
		parsed, err := parseCheckAt(*checkAt, time.Local)
		if err != nil {
			return err
		}
		parsedCheckAt = &parsed
	}

	if len(set) == 0 || (len(set) == 1 && set["json"]) {
		return errors.New("specify at least one field to update")
	}

	s, err := openStore()
	if err != nil {
		return err
	}
	id, err := s.ResolveID(ref)
	if err != nil {
		return err
	}

	updated, err := s.Update(id, *revision, func(t *task.Task) error {
		if set["title"] {
			t.Title = *title
		}
		if set["objective"] {
			t.Objective = *objective
		}
		if set["summary"] {
			t.Summary = *summary
		}
		if set["next-action"] {
			t.NextAction = *nextAction
		}
		if set["resume-when"] {
			t.ResumeWhen = *resumeWhen
		}
		if set["check-at"] {
			t.CheckAt = parsedCheckAt
		}
		if set["check-action"] {
			t.CheckAction = *checkAction
		}
		if set["status"] {
			st, err := task.ParseStatus(*statusName)
			if err != nil {
				return err
			}
			if st == task.StatusDone {
				t.Complete()
			} else {
				hasTrackedSession := false
				for _, session := range t.AgentSessions {
					hasTrackedSession = hasTrackedSession || session.Status != ""
				}
				if hasTrackedSession {
					return errors.New("the status of a Task with Agent Sessions is an aggregate; use everes session update")
				}
				t.SetStatus(st)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	if *asJSON {
		return writeJSON(os.Stdout, updated)
	}

	fmt.Printf("updated %s  status=%s  revision=%d\n", updated.TaskID, updated.Status, updated.Revision)
	if updated.Status == task.StatusWaiting && !hasWaitingResumeCondition(updated) {
		fmt.Println("hint: for a waiting Task, record --resume-when or both --check-at and --check-action")
	}
	return nil
}

func hasWaitingResumeCondition(t *task.Task) bool {
	if t.ResumeWhen != "" || t.CheckAt != nil {
		return true
	}
	for _, session := range t.AgentSessions {
		if session.Status == task.StatusWaiting && (session.ResumeWhen != "" || session.CheckAt != nil) {
			return true
		}
	}
	return false
}

func parseCheckAt(value string, location *time.Location) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
	} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid --check-at %q (use YYYY-MM-DD HH:MM in local time or RFC3339)", value)
}

func runTaskDone(args []string) error {
	fs := newFlagSet("task done", os.Stderr)
	asJSON := fs.Bool("json", false, "output JSON")
	yes := fs.Bool("yes", false, "complete all attached sessions without prompting")

	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ref, err := requireArg(fs, 0, "task-id")
	if err != nil {
		return err
	}

	s, err := openStore()
	if err != nil {
		return err
	}
	id, err := s.ResolveID(ref)
	if err != nil {
		return err
	}

	current, err := s.Get(id)
	if err != nil {
		return err
	}
	if !*yes {
		confirmed, err := confirmTaskDone(current, os.Stdin, os.Stderr)
		if err != nil {
			return err
		}
		if !confirmed {
			return errors.New("cancelled; Task was not completed")
		}
	}
	updated, err := s.Update(id, current.Revision, func(t *task.Task) error {
		t.Complete()
		return nil
	})
	if err != nil {
		return err
	}

	if *asJSON {
		return writeJSON(os.Stdout, updated)
	}
	fmt.Printf("done %s  %s\n", updated.TaskID, updated.Title)
	return nil
}

func confirmTaskDone(t *task.Task, input io.Reader, output io.Writer) (bool, error) {
	fmt.Fprintf(output, "Completing Task %s (%s) will mark all attached sessions done:\n", t.TaskID, t.Title)
	for _, session := range t.AgentSessions {
		fmt.Fprintf(output, "  %s/%s  status=%s\n", session.Provider, session.SessionID, displaySessionStatus(session.Status))
	}
	fmt.Fprint(output, "Continue? [y/N] ")
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func filterTasks(tasks []*task.Task, keep func(*task.Task) bool) []*task.Task {
	var out []*task.Task
	for _, t := range tasks {
		if keep(t) {
			out = append(out, t)
		}
	}
	return out
}

func writeJSON(w *os.File, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}
