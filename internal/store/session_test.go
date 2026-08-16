package store

import (
	"errors"
	"testing"

	"github.com/tsuyukimakoto/everes/internal/task"
)

func TestPutAndLookupSession(t *testing.T) {
	s := newTestStore(t)
	tk := newTestTask(t, "t")
	if err := s.Create(tk); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	idx := SessionIndex{
		Provider:  "claude",
		SessionID: "00000000-0000-4000-8000-000000000001",
		TaskID:    tk.TaskID,
		Cwd:       "/Users/example/dev/foo",
	}
	if err := s.PutSession(idx); err != nil {
		t.Fatalf("PutSession() error = %v", err)
	}

	got, err := s.LookupSession(idx.Provider, idx.SessionID)
	if err != nil {
		t.Fatalf("LookupSession() error = %v", err)
	}
	if got.TaskID != tk.TaskID {
		t.Errorf("TaskID = %q; want %q", got.TaskID, tk.TaskID)
	}
	if got.Cwd != idx.Cwd {
		t.Errorf("Cwd = %q; want %q", got.Cwd, idx.Cwd)
	}
}

func TestLookupSessionNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.LookupSession("claude", "missing-session")
	if !errors.Is(err, task.ErrNotFound) {
		t.Errorf("LookupSession() error = %v; want task.ErrNotFound", err)
	}
}

// TestSameSessionIDAcrossProviders は同一session IDが別providerで衝突しないことを確認する。
func TestSameSessionIDAcrossProviders(t *testing.T) {
	s := newTestStore(t)

	claudeTask := newTestTask(t, "claude側")
	codexTask := newTestTask(t, "codex側")
	for _, tk := range []*task.Task{claudeTask, codexTask} {
		if err := s.Create(tk); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	const shared = "same-id"
	if err := s.PutSession(SessionIndex{Provider: "claude", SessionID: shared, TaskID: claudeTask.TaskID}); err != nil {
		t.Fatalf("PutSession() error = %v", err)
	}
	if err := s.PutSession(SessionIndex{Provider: "codex", SessionID: shared, TaskID: codexTask.TaskID}); err != nil {
		t.Fatalf("PutSession() error = %v", err)
	}

	gotClaude, err := s.LookupSession("claude", shared)
	if err != nil {
		t.Fatalf("LookupSession(claude) error = %v", err)
	}
	gotCodex, err := s.LookupSession("codex", shared)
	if err != nil {
		t.Fatalf("LookupSession(codex) error = %v", err)
	}

	if gotClaude.TaskID == gotCodex.TaskID {
		t.Error("session indexes collided across providers")
	}
}

// TestSessionKeyRejectsPathTraversal はディレクトリ外への書き込みを防ぐ。
func TestSessionKeyRejectsPathTraversal(t *testing.T) {
	s := newTestStore(t)
	tk := newTestTask(t, "t")
	if err := s.Create(tk); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	tests := []struct {
		name      string
		provider  string
		sessionID string
	}{
		{"parent in session id", "claude", "../escape"},
		{"slash in session id", "claude", "a/b"},
		{"dot prefix", "claude", ".hidden"},
		{"parent in provider", "../claude", "s1"},
		{"empty session id", "claude", ""},
		{"empty provider", "", "s1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.PutSession(SessionIndex{
				Provider:  tt.provider,
				SessionID: tt.sessionID,
				TaskID:    tk.TaskID,
			})
			if err == nil {
				t.Errorf("PutSession(%q, %q) should fail", tt.provider, tt.sessionID)
			}
		})
	}
}

func TestPutSessionRejectsInvalidTaskID(t *testing.T) {
	s := newTestStore(t)
	err := s.PutSession(SessionIndex{Provider: "claude", SessionID: "s1", TaskID: "not-a-ulid"})
	if err == nil {
		t.Error("PutSession() with invalid task id should fail")
	}
}

func TestRemoveSession(t *testing.T) {
	s := newTestStore(t)
	tk := newTestTask(t, "t")
	if err := s.Create(tk); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	idx := SessionIndex{Provider: "claude", SessionID: "s1", TaskID: tk.TaskID}
	if err := s.PutSession(idx); err != nil {
		t.Fatalf("PutSession() error = %v", err)
	}
	if err := s.RemoveSession("claude", "s1"); err != nil {
		t.Fatalf("RemoveSession() error = %v", err)
	}

	if _, err := s.LookupSession("claude", "s1"); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("LookupSession() after remove error = %v; want task.ErrNotFound", err)
	}

	// Task本体は残る。indexは索引に過ぎない。
	if _, err := s.Get(tk.TaskID); err != nil {
		t.Errorf("Get() after removing session index error = %v", err)
	}
}

func TestListSessions(t *testing.T) {
	s := newTestStore(t)
	tk := newTestTask(t, "t")
	if err := s.Create(tk); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	for _, id := range []string{"s2", "s1", "s3"} {
		if err := s.PutSession(SessionIndex{Provider: "claude", SessionID: id, TaskID: tk.TaskID}); err != nil {
			t.Fatalf("PutSession() error = %v", err)
		}
	}

	got, err := s.ListSessions("claude")
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListSessions() length = %d; want 3", len(got))
	}
	for i, want := range []string{"s1", "s2", "s3"} {
		if got[i].SessionID != want {
			t.Errorf("ListSessions()[%d] = %q; want %q", i, got[i].SessionID, want)
		}
	}

	// 未使用providerは空を返す。
	empty, err := s.ListSessions("codex")
	if err != nil {
		t.Fatalf("ListSessions(codex) error = %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("ListSessions(codex) length = %d; want 0", len(empty))
	}
}
