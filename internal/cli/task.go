package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
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
	title := fs.String("title", "", "Taskのタイトル (必須)")
	objective := fs.String("objective", "", "この作業が何を目的としているか")
	summary := fs.String("summary", "", "現在どういう状態か")
	nextAction := fs.String("next-action", "", "次に実際に行う作業")
	resumeWhen := fs.String("resume-when", "", "いつこのTaskに戻ればよいか")
	checkAt := fs.String("check-at", "", "waitingを次に確認するローカル日時")
	checkAction := fs.String("check-action", "", "確認時刻を過ぎたら確認すること")
	statusName := fs.String("status", string(task.StatusActive), "status (active, waiting, needs_me, paused, done)")
	sessionID := fs.String("session-id", "", "紐付けるAgent sessionのID")
	providerName := fs.String("provider", "", "紐付けるAgentのprovider (claude, codex)")
	label := fs.String("label", "", "Agent sessionのラベル")
	noAttach := fs.Bool("no-attach", false, "現在のsessionを紐付けない")
	asJSON := fs.Bool("json", false, "JSONで出力する")

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
		fmt.Println("現在のsessionは特定できませんでした。everes session attach で後から紐付けできます。")
	}
	return nil
}

func runTaskList(args []string) error {
	fs := newFlagSet("task list", os.Stderr)
	asJSON := fs.Bool("json", false, "JSONで出力する")
	statusFilter := fs.String("status", "", "指定statusのみ表示する")
	all := fs.Bool("all", false, "done を含めてすべて表示する")

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

	renderListWithLinks(os.Stdout, tasks, time.Now(), stdoutIsTerminal())
	return nil
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func runTaskShow(args []string) error {
	fs := newFlagSet("task show", os.Stderr)
	asJSON := fs.Bool("json", false, "JSONで出力する")

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
	title := fs.String("title", "", "タイトルを変更する")
	objective := fs.String("objective", "", "objectiveを変更する")
	summary := fs.String("summary", "", "現在どういう状態かを更新する")
	nextAction := fs.String("next-action", "", "次に行う作業を更新する")
	resumeWhen := fs.String("resume-when", "", "再開条件を更新する")
	checkAt := fs.String("check-at", "", "waitingを次に確認するローカル日時（空文字で消去）")
	checkAction := fs.String("check-action", "", "確認時刻を過ぎたら確認すること（空文字で消去）")
	statusName := fs.String("status", "", "statusを変更する")
	revision := fs.Int("revision", 0, "この値と保存済みrevisionが一致する場合のみ更新する")
	asJSON := fs.Bool("json", false, "JSONで出力する")

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
		return errors.New("更新する項目を1つ以上指定してください")
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
					return errors.New("Agent SessionがあるTaskのstatusは集約値です。everes session updateを使ってください")
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
		fmt.Println("hint: waiting では --resume-when、または --check-at と --check-action を記録しておくと再開判断ができます。")
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
	asJSON := fs.Bool("json", false, "JSONで出力する")

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

	updated, err := s.Update(id, 0, func(t *task.Task) error {
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
