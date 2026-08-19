package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/tsuyukimakoto/everes/internal/provider"
	"github.com/tsuyukimakoto/everes/internal/store"
	"github.com/tsuyukimakoto/everes/internal/task"
)

// currentOutput は everes session current --json の出力形式。
//
// Task全体ではなく、AIが状況を把握するのに必要な項目だけを返す。
type currentOutput struct {
	Provider  string               `json:"provider"`
	SessionID string               `json:"session_id"`
	Task      *taskSummary         `json:"task"`
	Session   *agentSessionSummary `json:"session,omitempty"`
}

type agentSessionSummary struct {
	Status      task.Status `json:"status,omitempty"`
	Summary     string      `json:"summary,omitempty"`
	NextAction  string      `json:"next_action,omitempty"`
	ResumeWhen  string      `json:"resume_when,omitempty"`
	CheckAt     *time.Time  `json:"check_at,omitempty"`
	CheckAction string      `json:"check_action,omitempty"`
}

func newAgentSessionSummary(s *task.AgentSession) *agentSessionSummary {
	if s == nil {
		return nil
	}
	return &agentSessionSummary{s.Status, s.Summary, s.NextAction, s.ResumeWhen, s.CheckAt, s.CheckAction}
}

type taskSummary struct {
	TaskID      string      `json:"task_id"`
	Title       string      `json:"title"`
	Status      task.Status `json:"status"`
	Objective   string      `json:"objective,omitempty"`
	Summary     string      `json:"summary"`
	NextAction  string      `json:"next_action"`
	ResumeWhen  string      `json:"resume_when"`
	CheckAt     *time.Time  `json:"check_at,omitempty"`
	CheckAction string      `json:"check_action,omitempty"`
	Revision    int         `json:"revision"`
}

func newTaskSummary(t *task.Task) *taskSummary {
	return &taskSummary{
		TaskID:      t.TaskID,
		Title:       t.Title,
		Status:      t.Status,
		Objective:   t.Objective,
		Summary:     t.Summary,
		NextAction:  t.NextAction,
		ResumeWhen:  t.ResumeWhen,
		CheckAt:     t.CheckAt,
		CheckAction: t.CheckAction,
		Revision:    t.Revision,
	}
}

func runSession(args []string) error {
	if len(args) == 0 {
		return errors.New("session requires a subcommand (current, list, attach, update, done, detach, open)")
	}
	switch args[0] {
	case "current":
		return runSessionCurrent(args[1:])
	case "list":
		return runSessionList(args[1:])
	case "attach":
		return runSessionAttach(args[1:])
	case "update":
		return runSessionUpdate(args[1:])
	case "done":
		return runSessionDone(args[1:])
	case "detach":
		return runSessionDetach(args[1:])
	case "open":
		return runSessionOpen(args[1:])
	default:
		return fmt.Errorf("unknown session subcommand %q", args[0])
	}
}

func runSessionDetach(args []string) error {
	fs := newFlagSet("session detach", os.Stderr)
	providerName := fs.String("provider", "", "外すsessionのprovider (必須)")
	sessionID := fs.String("session-id", "", "外すsessionのID (必須)")
	asJSON := fs.Bool("json", false, "JSONで出力する")

	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ref, err := requireArg(fs, 0, "task-id")
	if err != nil {
		return err
	}
	if *providerName == "" {
		return errors.New("--provider is required")
	}
	if *sessionID == "" {
		return errors.New("--session-id is required")
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
		return t.DetachSession(*providerName, *sessionID)
	})
	if err != nil {
		return err
	}
	if err := s.RemoveSession(*providerName, *sessionID); err != nil && !errors.Is(err, task.ErrNotFound) {
		return err
	}

	if *asJSON {
		return writeJSON(os.Stdout, newTaskSummary(updated))
	}
	fmt.Printf("detached %s session %s <- %s  %s\n",
		*providerName, *sessionID, updated.TaskID, updated.Title)
	return nil
}

// sessionState はTaskに紐づくAgent Sessionとprovider側の保存状態をまとめたもの。
// LastSeenAtはEveresへ最後に紐付けた時刻であり、実行中かどうかは表さない。
type sessionState struct {
	Provider    string         `json:"provider"`
	SessionID   string         `json:"session_id"`
	Cwd         string         `json:"cwd,omitempty"`
	Label       string         `json:"label,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	LastSeenAt  time.Time      `json:"last_seen_at"`
	Status      task.Status    `json:"status,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	NextAction  string         `json:"next_action,omitempty"`
	ResumeWhen  string         `json:"resume_when,omitempty"`
	CheckAt     *time.Time     `json:"check_at,omitempty"`
	CheckAction string         `json:"check_action,omitempty"`
	Exists      bool           `json:"exists"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Error       string         `json:"error,omitempty"`
}

func runSessionList(args []string) error {
	fs := newFlagSet("session list", os.Stderr)
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
	states := collectSessionStates(t)

	if *asJSON {
		if states == nil {
			states = []sessionState{}
		}
		return writeJSON(os.Stdout, states)
	}

	fmt.Printf("%s\n\n", t.Title)
	if len(states) == 0 {
		fmt.Println("Agent sessionは紐付いていません。")
		return nil
	}
	for _, state := range states {
		fmt.Printf("  [%s] %s  status=%s\n", state.Provider, state.SessionID, displaySessionStatus(state.Status))
		if state.Label != "" {
			fmt.Printf("        %s\n", state.Label)
		}
		availability := "provider側の記録なし"
		if state.Exists {
			availability = "provider側の記録あり"
		}
		fmt.Printf("        %s  Everesでの最終確認: %s\n", availability, relTime(state.LastSeenAt, time.Now()))
		if modifiedAt, ok := state.Metadata["modified_at"].(string); ok {
			fmt.Printf("        provider更新時刻: %s\n", modifiedAt)
		}
		if state.Error != "" {
			fmt.Printf("        確認エラー: %s\n", state.Error)
		}
		if state.Summary != "" {
			fmt.Printf("        %s\n", state.Summary)
		}
		if state.CheckAt != nil {
			fmt.Printf("        確認予定 %s: %s\n", formatCheckAt(*state.CheckAt, time.Local), state.CheckAction)
		}
	}
	return nil
}

func collectSessionStates(t *task.Task) []sessionState {
	states := make([]sessionState, 0, len(t.AgentSessions))
	for _, attached := range t.AgentSessions {
		state := sessionState{
			Provider:    attached.Provider,
			SessionID:   attached.SessionID,
			Cwd:         attached.Cwd,
			Label:       attached.Label,
			CreatedAt:   attached.CreatedAt,
			LastSeenAt:  attached.LastSeenAt,
			Status:      attached.Status,
			Summary:     attached.Summary,
			NextAction:  attached.NextAction,
			ResumeWhen:  attached.ResumeWhen,
			CheckAt:     attached.CheckAt,
			CheckAction: attached.CheckAction,
		}
		p, err := provider.Get(attached.Provider)
		if err != nil {
			state.Error = err.Error()
			states = append(states, state)
			continue
		}
		session := provider.Session{
			Provider:  attached.Provider,
			SessionID: attached.SessionID,
			Cwd:       attached.Cwd,
			Label:     attached.Label,
		}
		state.Exists, err = p.SessionExists(session)
		if err != nil {
			state.Error = err.Error()
			states = append(states, state)
			continue
		}
		state.Metadata, err = p.SessionMetadata(session)
		if err != nil {
			state.Error = err.Error()
		}
		states = append(states, state)
	}
	return states
}

func runSessionCurrent(args []string) error {
	fs := newFlagSet("session current", os.Stderr)
	asJSON := fs.Bool("json", false, "JSONで出力する")
	providerName := fs.String("provider", "", "provider (claude, codex)")
	sessionID := fs.String("session-id", "", "session ID")

	if err := parseFlags(fs, args); err != nil {
		return err
	}

	s, err := openStore()
	if err != nil {
		return err
	}

	sess, err := resolveSession(*providerName, *sessionID, currentDir())
	if err != nil {
		if !errors.Is(err, provider.ErrNoCurrentSession) {
			return err
		}
		if *asJSON {
			return writeJSON(os.Stdout, currentOutput{})
		}
		fmt.Println("現在のsessionを特定できませんでした。--provider と --session-id を指定してください。")
		return nil
	}

	out := currentOutput{Provider: sess.Provider, SessionID: sess.SessionID}

	t, err := lookupTaskBySession(s, sess.Provider, sess.SessionID)
	if err != nil {
		return err
	}
	if t != nil {
		out.Task = newTaskSummary(t)
		if attached := findAttachedSession(t, sess.Provider, sess.SessionID); attached != nil {
			out.Session = newAgentSessionSummary(attached)
		}
	}

	if *asJSON {
		return writeJSON(os.Stdout, out)
	}

	if t == nil {
		fmt.Printf("[%s] %s\n", sess.Provider, sess.SessionID)
		fmt.Println("このsessionに紐づくTaskはありません。")
		fmt.Println("everes task create --title \"...\" で作成できます。")
		return nil
	}

	fmt.Printf("[%s] %s\n\n", sess.Provider, sess.SessionID)
	renderDetail(os.Stdout, t, time.Now())
	return nil
}

func runSessionAttach(args []string) error {
	fs := newFlagSet("session attach", os.Stderr)
	providerName := fs.String("provider", "", "provider (claude, codex)")
	sessionID := fs.String("session-id", "", "session ID")
	label := fs.String("label", "", "Agent sessionのラベル")
	statusName := fs.String("status", string(task.StatusActive), "session status")
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

	cwd := currentDir()
	sess, err := resolveSession(*providerName, *sessionID, cwd)
	if err != nil {
		return err
	}

	status, err := task.ParseStatus(*statusName)
	if err != nil {
		return err
	}
	updated, err := s.Update(id, 0, func(t *task.Task) error {
		t.AttachSession(task.AgentSession{
			Provider:  sess.Provider,
			SessionID: sess.SessionID,
			Cwd:       cwd,
			Label:     *label,
			Status:    status,
		})
		return nil
	})
	if err != nil {
		return err
	}

	if err := s.PutSession(store.SessionIndex{
		Provider:  sess.Provider,
		SessionID: sess.SessionID,
		TaskID:    updated.TaskID,
		Cwd:       cwd,
	}); err != nil {
		return err
	}

	if *asJSON {
		return writeJSON(os.Stdout, newTaskSummary(updated))
	}
	fmt.Printf("attached %s session %s -> %s  %s\n",
		sess.Provider, sess.SessionID, updated.TaskID, updated.Title)
	return nil
}

func runSessionUpdate(args []string) error {
	fs := newFlagSet("session update", os.Stderr)
	providerName := fs.String("provider", "", "provider (claude, codex)")
	sessionID := fs.String("session-id", "", "session ID")
	statusName := fs.String("status", "", "statusを変更する")
	summary := fs.String("summary", "", "このsessionの現在の状態")
	nextAction := fs.String("next-action", "", "このsessionで次に行う作業")
	resumeWhen := fs.String("resume-when", "", "このsessionを再開する条件")
	checkAt := fs.String("check-at", "", "次に確認するローカル日時（空文字で消去）")
	checkAction := fs.String("check-action", "", "確認時刻に確認すること（空文字で消去）")
	revision := fs.Int("revision", 0, "Task revisionが一致する場合のみ更新する")
	asJSON := fs.Bool("json", false, "JSONで出力する")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ref, err := requireArg(fs, 0, "task-id")
	if err != nil {
		return err
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if len(set) == 0 || (len(set) == 1 && set["json"]) {
		return errors.New("更新する項目を1つ以上指定してください")
	}
	var parsedCheckAt *time.Time
	if set["check-at"] && *checkAt != "" {
		parsed, err := parseCheckAt(*checkAt, time.Local)
		if err != nil {
			return err
		}
		parsedCheckAt = &parsed
	}
	sess, err := resolveSession(*providerName, *sessionID, currentDir())
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
	updated, err := s.Update(id, *revision, func(t *task.Task) error {
		return t.UpdateSession(sess.Provider, sess.SessionID, func(attached *task.AgentSession) error {
			if set["status"] {
				status, err := task.ParseStatus(*statusName)
				if err != nil {
					return err
				}
				attached.SetStatus(status)
			}
			if set["summary"] {
				attached.Summary = *summary
			}
			if set["next-action"] {
				attached.NextAction = *nextAction
			}
			if set["resume-when"] {
				attached.ResumeWhen = *resumeWhen
			}
			if set["check-at"] {
				attached.CheckAt = parsedCheckAt
			}
			if set["check-action"] {
				attached.CheckAction = *checkAction
			}
			return nil
		})
	})
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(os.Stdout, updated)
	}
	attached := findAttachedSession(updated, sess.Provider, sess.SessionID)
	fmt.Printf("updated %s/%s  session=%s  task=%s  revision=%d\n",
		sess.Provider, sess.SessionID, displaySessionStatus(attached.Status), updated.Status, updated.Revision)
	return nil
}

func runSessionDone(args []string) error {
	return runSessionUpdate(append(args, "--status", string(task.StatusDone)))
}

func findAttachedSession(t *task.Task, providerName, sessionID string) *task.AgentSession {
	for i := range t.AgentSessions {
		if t.AgentSessions[i].Provider == providerName && t.AgentSessions[i].SessionID == sessionID {
			return &t.AgentSessions[i]
		}
	}
	return nil
}

func displaySessionStatus(status task.Status) string {
	if status == "" {
		return "not_set"
	}
	return string(status)
}

func runSessionOpen(args []string) error {
	fs := newFlagSet("session open", os.Stderr)
	providerName := fs.String("provider", "", "開くsessionのprovider")
	sessionID := fs.String("session-id", "", "開くsessionのID")
	list := fs.Bool("list", false, "紐づくsessionを一覧するだけで開かない")
	app := fs.Bool("app", false, "desktop appでsessionを表示する")

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

	if len(t.AgentSessions) == 0 {
		return fmt.Errorf("task %s にAgent sessionが紐付いていません", task.ShortID(t.TaskID))
	}

	if *list {
		fmt.Printf("%s\n\n", t.Title)
		for _, sess := range t.AgentSessions {
			fmt.Printf("  [%s] %s  %s  %s\n",
				sess.Provider, sess.SessionID, sess.Label, relTime(sess.LastSeenAt, time.Now()))
		}
		return nil
	}

	target, err := selectSession(t, *providerName, *sessionID)
	if err != nil {
		return err
	}
	if *app {
		return openSessionInApp(target)
	}

	p, err := provider.Get(target.Provider)
	if err != nil {
		return err
	}

	exists, err := p.SessionExists(provider.Session{
		Provider:  target.Provider,
		SessionID: target.SessionID,
		Cwd:       target.Cwd,
	})
	if err != nil {
		return err
	}
	if !exists {
		fmt.Fprintf(os.Stderr,
			"warn: %s session %s の記録が見つかりません。再開に失敗する可能性があります。\n",
			target.Provider, target.SessionID)
	}

	return p.OpenSession(provider.Session{
		Provider:  target.Provider,
		SessionID: target.SessionID,
		Cwd:       target.Cwd,
		Label:     target.Label,
	})
}

var launchAppURL = func(target string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("desktop app linkの起動はmacOSにのみ対応しています")
	}
	if err := exec.Command("/usr/bin/open", target).Run(); err != nil {
		return fmt.Errorf("open desktop app link: %w", err)
	}
	return nil
}

func openSessionInApp(target task.AgentSession) error {
	appURL, err := provider.SessionAppURL(provider.Session{
		Provider:  target.Provider,
		SessionID: target.SessionID,
		Cwd:       target.Cwd,
		Label:     target.Label,
	})
	if err != nil {
		return err
	}
	if err := launchAppURL(appURL); err != nil {
		return err
	}
	label := target.Label
	if label == "" {
		label = target.SessionID
	}
	fmt.Printf("Codex Appで表示しました: %s\n", label)
	return nil
}

// selectSession は開く対象のAgent sessionを決める。
// 指定がなければ最後に使われたsessionをdefaultとする。
func selectSession(t *task.Task, providerName, sessionID string) (task.AgentSession, error) {
	if sessionID != "" {
		for _, s := range t.AgentSessions {
			if s.SessionID == sessionID {
				return s, nil
			}
		}
		return task.AgentSession{}, fmt.Errorf("session %s はこのTaskに紐付いていません", sessionID)
	}

	if providerName != "" {
		var latest task.AgentSession
		found := false
		for _, s := range t.AgentSessions {
			if s.Provider != providerName {
				continue
			}
			if !found || s.LastSeenAt.After(latest.LastSeenAt) {
				latest, found = s, true
			}
		}
		if !found {
			return task.AgentSession{}, fmt.Errorf("provider %s のsessionはこのTaskに紐付いていません", providerName)
		}
		return latest, nil
	}

	latest, ok := t.LatestSession()
	if !ok {
		return task.AgentSession{}, errors.New("Agent sessionが紐付いていません")
	}
	return latest, nil
}

// resolveSession は操作対象のsessionを決める。
//
// 明示指定 > 環境変数 > Adapterによる推定 の順で解決する。
func resolveSession(providerName, sessionID, cwd string) (*provider.Session, error) {
	if sessionID != "" && providerName != "" {
		return &provider.Session{Provider: providerName, SessionID: sessionID, Cwd: cwd}, nil
	}
	if sessionID != "" {
		// providerが分からない場合はindexを引いて特定する。
		p, err := findProviderBySessionID(sessionID)
		if err != nil {
			return nil, err
		}
		return &provider.Session{Provider: p, SessionID: sessionID, Cwd: cwd}, nil
	}
	if providerName != "" {
		p, err := provider.Get(providerName)
		if err != nil {
			return nil, err
		}
		return p.DetectCurrentSession(cwd)
	}
	return provider.DetectCurrentSession(cwd)
}

// findProviderBySessionID はsession indexを走査してproviderを特定する。
func findProviderBySessionID(sessionID string) (string, error) {
	s, err := store.Default()
	if err != nil {
		return "", err
	}
	var matched []string
	for _, name := range provider.Names() {
		if _, err := s.LookupSession(name, sessionID); err == nil {
			matched = append(matched, name)
		} else if !errors.Is(err, task.ErrNotFound) {
			return "", err
		}
	}
	switch len(matched) {
	case 0:
		return "", fmt.Errorf("session %s に対応するproviderが不明です。--provider を指定してください", sessionID)
	case 1:
		return matched[0], nil
	default:
		return "", fmt.Errorf("session %s が複数providerに存在します。--provider を指定してください", sessionID)
	}
}

// lookupTaskBySession はsession indexを辿ってTaskを返す。無い場合は nil を返す。
func lookupTaskBySession(s *store.Store, providerName, sessionID string) (*task.Task, error) {
	idx, err := s.LookupSession(providerName, sessionID)
	if err != nil {
		if errors.Is(err, task.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	t, err := s.Get(idx.TaskID)
	if err != nil {
		if errors.Is(err, task.ErrNotFound) {
			// indexだけが残っている状態。Taskが正であり、indexは索引に過ぎない。
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}
