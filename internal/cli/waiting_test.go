package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/tsuyukimakoto/everes/internal/task"
)

func TestParseCheckAtUsesLocalLocation(t *testing.T) {
	location := time.FixedZone("JST", 9*60*60)
	got, err := parseCheckAt("2026-08-17 09:30", location)
	if err != nil {
		t.Fatalf("parseCheckAt() error = %v", err)
	}
	if got.Location() != location {
		t.Errorf("location = %v; want %v", got.Location(), location)
	}
	if got.Format(time.RFC3339) != "2026-08-17T09:30:00+09:00" {
		t.Errorf("time = %s; want local RFC3339 with offset", got.Format(time.RFC3339))
	}
}

func TestRenderListSeparatesOverdueWaiting(t *testing.T) {
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	overdue, err := task.New("レビュー確認")
	if err != nil {
		t.Fatalf("task.New() error = %v", err)
	}
	overdue.SetStatus(task.StatusWaiting)
	checkAt := now.Add(-time.Hour)
	overdue.CheckAt = &checkAt
	overdue.CheckAction = "PR #381のレビュー状況を確認する"

	var out bytes.Buffer
	renderList(&out, []*task.Task{overdue}, now)
	got := out.String()
	if !strings.Contains(got, "WAITING — CHECK NOW") {
		t.Errorf("rendered list = %q; want overdue group", got)
	}
	if !strings.Contains(got, "PR #381のレビュー状況を確認する") {
		t.Errorf("rendered list = %q; want check action", got)
	}
}

func TestSessionCountLabelDistinguishesTaskFromSessions(t *testing.T) {
	tk, err := task.New("複数session")
	if err != nil {
		t.Fatalf("task.New() error = %v", err)
	}
	tk.AgentSessions = []task.AgentSession{
		{Provider: "codex", SessionID: "codex-1"},
		{Provider: "codex", SessionID: "codex-2"},
		{Provider: "claude", SessionID: "claude-1"},
	}

	if got := sessionCountLabel(len(tk.AgentSessions)); got != "3 sessions" {
		t.Errorf("sessionCountLabel() = %q; want %q", got, "3 sessions")
	}
}

func TestSessionDisplayNameLinksCodexThreads(t *testing.T) {
	tk, err := task.New("公開")
	if err != nil {
		t.Fatalf("task.New() error = %v", err)
	}
	tk.AgentSessions = []task.AgentSession{
		{Provider: "codex", SessionID: "01900000-0000-7000-8000-000000000001", Label: "公開準備"},
		{Provider: "claude", SessionID: "claude-1", Label: "レビュー"},
	}

	plain := sessionDisplayName(tk.AgentSessions[0], false)
	if plain != "↗ codex:公開準備" {
		t.Errorf("sessionDisplayName(plain) = %q; want labeled Codex session", plain)
	}

	linked := sessionDisplayName(tk.AgentSessions[0], true)
	if !strings.Contains(linked, "codex://threads/01900000-0000-7000-8000-000000000001") {
		t.Errorf("sessionDisplayName(linked) = %q; want Codex deep link", linked)
	}

	command, ok := claudeResumeCommand(tk, tk.AgentSessions[1])
	wantCommand := "everes session open " + task.ShortID(tk.TaskID) + " --provider claude --session-id claude-1"
	if !ok || command.Command != wantCommand || command.Label != "レビュー" {
		t.Errorf("claudeResumeCommand() = %#v, %v; want labeled command %q", command, ok, wantCommand)
	}
}

func TestRenderListShowsSessionStatusesAndSessionCheck(t *testing.T) {
	now := time.Date(2026, 8, 17, 18, 30, 0, 0, time.FixedZone("JST", 9*60*60))
	tk, _ := task.New("公開準備")
	tk.AttachSession(task.AgentSession{Provider: "codex", SessionID: "writing", Label: "執筆"})
	checkAt := now.Add(-30 * time.Minute)
	if err := tk.UpdateSession("codex", "writing", func(s *task.AgentSession) error {
		s.SetStatus(task.StatusWaiting)
		s.CheckAt = &checkAt
		s.CheckAction = "公開日を決める"
		return nil
	}); err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	var out bytes.Buffer
	renderList(&out, []*task.Task{tk}, now)
	got := out.String()
	for _, want := range []string{"WAITING — CHECK NOW", "│  Task  WAITING (1 session)", "├─ Sessions", "│  └─ WAITING  ↗ codex:執筆", "確認時刻超過", "公開日を決める", "╰─"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered list = %q; want %q", got, want)
		}
	}
}

func TestWaitingResumeConditionIncludesSessions(t *testing.T) {
	tk, _ := task.New("公開準備")
	tk.AttachSession(task.AgentSession{Provider: "codex", SessionID: "writing"})
	_ = tk.UpdateSession("codex", "writing", func(s *task.AgentSession) error {
		s.SetStatus(task.StatusWaiting)
		s.ResumeWhen = "先行記事の公開後"
		return nil
	})
	if !hasWaitingResumeCondition(tk) {
		t.Fatal("session resume_when should satisfy the waiting resume condition")
	}
}

func TestOpenSessionInAppLaunchesCodexDeepLink(t *testing.T) {
	previous := launchAppURL
	t.Cleanup(func() { launchAppURL = previous })

	var launched string
	launchAppURL = func(target string) error {
		launched = target
		return nil
	}

	err := openSessionInApp(task.AgentSession{
		Provider:  "codex",
		SessionID: "01900000-0000-7000-8000-000000000001",
		Label:     "公開準備",
	})
	if err != nil {
		t.Fatalf("openSessionInApp() error = %v", err)
	}
	want := "codex://threads/01900000-0000-7000-8000-000000000001"
	if launched != want {
		t.Errorf("launched URL = %q; want %q", launched, want)
	}
}
