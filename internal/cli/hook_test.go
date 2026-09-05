package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tsuyukimakoto/everes/internal/store"
	"github.com/tsuyukimakoto/everes/internal/task"
)

func TestWriteSessionStartOutputUsesCodexJSONEnvelope(t *testing.T) {
	const context = "[Everes] This session is not registered.\n"

	var out bytes.Buffer
	if err := writeSessionStartOutput(&out, "codex", context); err != nil {
		t.Fatalf("writeSessionStartOutput() error = %v", err)
	}

	var got struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\noutput = %q", err, out.String())
	}
	if got.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Errorf("hookEventName = %q; want SessionStart", got.HookSpecificOutput.HookEventName)
	}
	if got.HookSpecificOutput.AdditionalContext != context {
		t.Errorf("additionalContext = %q; want %q", got.HookSpecificOutput.AdditionalContext, context)
	}
}

func TestWriteSessionStartOutputKeepsClaudePlainText(t *testing.T) {
	const context = "[Everes] This session is not registered.\n"

	var out bytes.Buffer
	if err := writeSessionStartOutput(&out, "claude", context); err != nil {
		t.Fatalf("writeSessionStartOutput() error = %v", err)
	}

	if got := out.String(); got != context {
		t.Errorf("output = %q; want %q", got, context)
	}
}

func TestWriteSessionStartOutputEmitsValidCodexJSONWithoutContext(t *testing.T) {
	var out bytes.Buffer
	if err := writeSessionStartOutput(&out, "codex", ""); err != nil {
		t.Fatalf("writeSessionStartOutput() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\noutput = %q", err, out.String())
	}
}

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
	if !strings.Contains(got, "Attention:   The scheduled check time has passed.") {
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

	want := "[Everes] This session (codex 01900000-0000-7000-8000-000000000001) is attached to Task "
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

func TestWriteTaskContextRemindsAgentToRefreshSnapshotAndConfirmStatus(t *testing.T) {
	tk, _ := task.New("状態維持")
	tk.AttachSession(task.AgentSession{Provider: "codex", SessionID: "current"})

	var out bytes.Buffer
	writeTaskContext(&out, tk, "codex", "current")
	got := out.String()
	for _, want := range []string{
		"replace this session's summary and next action with a current snapshot",
		"do not infer a status change; ask the user whether to change it",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("context = %q; want %q", got, want)
		}
	}
}

func TestWriteUnregisteredContextListsAllUnfinishedTasksAndSelectionPolicy(t *testing.T) {
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
		"Everesを公開する",
		"Status: active",
		"Task ID: " + active.TaskID,
		"Next Action: READMEを最終確認する",
		"レビュー結果を待つ",
		"Status: waiting",
		"Task ID: " + waiting.TaskID,
		"Compare the first user message",
		"Recommended",
		"every unfinished Task",
		"Propose one concise new Task title",
		"create a new Task with the proposed title",
		"create a new Task with a title entered by the user",
		"do not attach this session",
		"everes session attach " + active.TaskID,
		"--provider codex",
		"--session-id 01900000-0000-7000-8000-000000000001",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("context = %q; want it to contain %q", got, want)
		}
	}
	for _, unwanted := range []string{done.Title, done.TaskID} {
		if strings.Contains(got, unwanted) {
			t.Errorf("context = %q; do not want it to contain %q", got, unwanted)
		}
	}
}

func TestWriteUnregisteredContextAsksToCreateWhenThereAreNoUnfinishedTasks(t *testing.T) {
	done, err := task.New("完了したTask")
	if err != nil {
		t.Fatalf("task.New() error = %v", err)
	}
	done.SetStatus(task.StatusDone)

	var out bytes.Buffer
	writeUnregisteredContext(&out, "claude", "session-1", []*task.Task{done})

	got := out.String()
	for _, want := range []string{
		"There are no unfinished Tasks",
		"whether they would like to create a new Task",
		"first user message",
		"Propose one concise new Task title",
		"create a new Task with the proposed title",
		"create a new Task with a title entered by the user",
		"do not attach this session",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("context = %q; want %q", got, want)
		}
	}
	if strings.Contains(got, done.Title) {
		t.Errorf("context = %q; do not want completed task %q", got, done.Title)
	}
}
