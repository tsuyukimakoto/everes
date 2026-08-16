package cli

import (
	"strings"
	"testing"

	"github.com/tsuyukimakoto/everes/internal/task"
)

func TestCollectSessionStatesKeepsUnavailableProvider(t *testing.T) {
	tk, err := task.New("セッション連携")
	if err != nil {
		t.Fatalf("task.New() error = %v", err)
	}
	tk.AttachSession(task.AgentSession{
		Provider:  "unknown-provider",
		SessionID: "session-1",
		Label:     "調査",
	})

	states := collectSessionStates(tk)
	if len(states) != 1 {
		t.Fatalf("len(states) = %d; want 1", len(states))
	}
	if states[0].Provider != "unknown-provider" || states[0].SessionID != "session-1" {
		t.Errorf("state = %#v; want original session identity", states[0])
	}
	if states[0].Error == "" {
		t.Error("state.Error is empty; want provider lookup error")
	}
}

func TestWriteTaskContextIncludesRelatedSessions(t *testing.T) {
	tk, err := task.New("セッション連携")
	if err != nil {
		t.Fatalf("task.New() error = %v", err)
	}
	tk.AttachSession(task.AgentSession{
		Provider:  "codex",
		SessionID: "01900000-0000-7000-8000-000000000001",
		Label:     "実装",
	})
	tk.AttachSession(task.AgentSession{
		Provider:  "claude",
		SessionID: "01900000-0000-7000-8000-000000000002",
		Label:     "レビュー",
	})

	var out strings.Builder
	writeTaskContext(&out, tk, "codex", "01900000-0000-7000-8000-000000000001")

	got := out.String()
	if !strings.Contains(got, "Related Agent Sessions:") {
		t.Fatalf("context = %q; want related session section", got)
	}
	if !strings.Contains(got, "[claude] 01900000-0000-7000-8000-000000000002  レビュー") {
		t.Errorf("context = %q; want related Claude session", got)
	}
	if strings.Contains(got, "[codex] 01900000-0000-7000-8000-000000000001  実装") {
		t.Errorf("context = %q; current session must not be repeated as related", got)
	}
}
