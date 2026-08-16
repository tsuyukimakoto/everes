package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/tsuyukimakoto/everes/internal/store"
	"github.com/tsuyukimakoto/everes/internal/task"
)

func TestDetectHookProviderDoesNotGuessFromSharedEventName(t *testing.T) {
	s := store.New(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	got, err := detectHookProvider(s, "01900000-0000-7000-8000-000000000001")
	if err != nil {
		t.Fatalf("detectHookProvider() error = %v", err)
	}
	if got != "" {
		t.Errorf("provider = %q; want empty when no session index matches", got)
	}
}

func TestWriteTaskContextIncludesOverdueWaitingCheck(t *testing.T) {
	tk, err := task.New("レビュー確認")
	if err != nil {
		t.Fatalf("task.New() error = %v", err)
	}
	tk.SetStatus(task.StatusWaiting)
	checkAt := time.Now().Add(-time.Hour)
	tk.CheckAt = &checkAt
	tk.CheckAction = "PR #381のレビュー状況を確認する"

	var out bytes.Buffer
	writeTaskContext(&out, tk, "codex", "01900000-0000-7000-8000-000000000001")

	got := out.String()
	if !strings.Contains(got, "Check:       PR #381のレビュー状況を確認する") {
		t.Errorf("context = %q; want check action", got)
	}
	if !strings.Contains(got, "Attention:   確認時刻を過ぎています。") {
		t.Errorf("context = %q; want overdue attention", got)
	}
}

func TestWriteTaskContextIncludesExactSession(t *testing.T) {
	tk, err := task.New("Codex連携")
	if err != nil {
		t.Fatalf("task.New() error = %v", err)
	}

	var out bytes.Buffer
	writeTaskContext(&out, tk, "codex", "01900000-0000-7000-8000-000000000001")

	want := "[Everes] このsession (codex 01900000-0000-7000-8000-000000000001) はTask "
	if !strings.Contains(out.String(), want) {
		t.Errorf("context = %q; want it to contain %q", out.String(), want)
	}
}

func TestWriteTaskContextIncludesCurrentSessionStatus(t *testing.T) {
	tk, _ := task.New("並行作業")
	tk.AttachSession(task.AgentSession{Provider: "codex", SessionID: "current"})
	_ = tk.UpdateSession("codex", "current", func(s *task.AgentSession) error {
		s.SetStatus(task.StatusWaiting)
		s.Summary = "公開日を確認待ち"
		return nil
	})
	var out bytes.Buffer
	writeTaskContext(&out, tk, "codex", "current")
	got := out.String()
	for _, want := range []string{"Task Status: waiting (session aggregate)", "Session Status: waiting", "Session Summary: 公開日を確認待ち"} {
		if !strings.Contains(got, want) {
			t.Errorf("context = %q; want %q", got, want)
		}
	}
}

func TestWriteUnregisteredContextListsOnlyActiveTasksForSelection(t *testing.T) {
	active, err := task.New("Everesを公開する")
	if err != nil {
		t.Fatalf("task.New() error = %v", err)
	}
	active.NextAction = "READMEを最終確認する"

	waiting, err := task.New("レビュー結果を待つ")
	if err != nil {
		t.Fatalf("task.New() error = %v", err)
	}
	waiting.SetStatus(task.StatusWaiting)

	done, err := task.New("完了したTask")
	if err != nil {
		t.Fatalf("task.New() error = %v", err)
	}
	done.SetStatus(task.StatusDone)

	var out bytes.Buffer
	writeUnregisteredContext(
		&out,
		"codex",
		"01900000-0000-7000-8000-000000000001",
		[]*task.Task{waiting, active, done},
	)

	got := out.String()
	for _, want := range []string{
		"1. Everesを公開する",
		"Task ID: " + active.TaskID,
		"Next Action: READMEを最終確認する",
		"番号またはタイトル",
		"どれにも紐付けない",
		"everes session attach " + active.TaskID,
		"--provider codex",
		"--session-id 01900000-0000-7000-8000-000000000001",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("context = %q; want it to contain %q", got, want)
		}
	}
	for _, unwanted := range []string{waiting.Title, waiting.TaskID, done.Title, done.TaskID} {
		if strings.Contains(got, unwanted) {
			t.Errorf("context = %q; do not want it to contain %q", got, unwanted)
		}
	}
}

func TestWriteUnregisteredContextDoesNotAskWhenThereAreNoActiveTasks(t *testing.T) {
	waiting, err := task.New("レビュー結果を待つ")
	if err != nil {
		t.Fatalf("task.New() error = %v", err)
	}
	waiting.SetStatus(task.StatusWaiting)

	var out bytes.Buffer
	writeUnregisteredContext(&out, "claude", "session-1", []*task.Task{waiting})

	got := out.String()
	if strings.Contains(got, "番号またはタイトル") {
		t.Errorf("context = %q; must not ask for a selection without active tasks", got)
	}
	if !strings.Contains(got, "指示がない限りTaskを作成する必要はありません") {
		t.Errorf("context = %q; want existing unregistered guidance", got)
	}
}
