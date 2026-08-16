package task

import (
	"strings"
	"testing"
	"time"
)

func TestParseStatus(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Status
		wantErr bool
	}{
		{"active", "active", StatusActive, false},
		{"waiting", "waiting", StatusWaiting, false},
		{"needs_me", "needs_me", StatusNeedsMe, false},
		{"paused", "paused", StatusPaused, false},
		{"done", "done", StatusDone, false},
		{"unknown", "blocked", "", true},
		{"empty", "", "", true},
		{"case sensitive", "Active", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseStatus(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseStatus(%q) error = %v; wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseStatus(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewULID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id, err := NewULID()
		if err != nil {
			t.Fatalf("NewULID() error = %v", err)
		}
		if len(id) != ulidLen {
			t.Fatalf("NewULID() length = %d; want %d", len(id), ulidLen)
		}
		if !ValidULID(id) {
			t.Fatalf("NewULID() = %q is not valid", id)
		}
		if seen[id] {
			t.Fatalf("NewULID() returned duplicate %q", id)
		}
		seen[id] = true
	}
}

func TestNewULIDIsTimeOrdered(t *testing.T) {
	base := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)

	earlier, err := newULIDAt(base)
	if err != nil {
		t.Fatalf("newULIDAt() error = %v", err)
	}
	later, err := newULIDAt(base.Add(time.Second))
	if err != nil {
		t.Fatalf("newULIDAt() error = %v", err)
	}

	// timestampが先頭10文字に入るため、辞書順で時刻順にソートできる。
	if earlier >= later {
		t.Errorf("expected %q < %q", earlier, later)
	}
}

// TestShortIDDistinguishesNearbyTasks は同時期に作ったTaskを見分けられることを確認する。
//
// 先頭8文字だけではミリ秒timestampの一部しか映らず、連続作成で全て同じ表示になった。
func TestShortIDDistinguishesNearbyTasks(t *testing.T) {
	// timestamp部が同じでも、random部の先頭2文字が異なれば表示を区別できる。
	first := "01K2N86E2A00Q9CJG4Y0J73SME"
	second := "01K2N86E2A01Q9CJG4Y0J73SME"
	if got := ShortID(first); got != "01K2N86E2A00" {
		t.Errorf("ShortID(%q) = %q; want %q", first, got, "01K2N86E2A00")
	}
	if got := ShortID(second); got != "01K2N86E2A01" {
		t.Errorf("ShortID(%q) = %q; want %q", second, got, "01K2N86E2A01")
	}
	if ShortID(first) == ShortID(second) {
		t.Error("short ids should differ when the displayed random part differs")
	}
}

func TestShortIDHandlesShortInput(t *testing.T) {
	if got := ShortID("01M04"); got != "01M04" {
		t.Errorf("ShortID(%q) = %q; want the input unchanged", "01M04", got)
	}
}

func TestValidULID(t *testing.T) {
	valid, err := NewULID()
	if err != nil {
		t.Fatalf("NewULID() error = %v", err)
	}

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"generated", valid, true},
		{"too short", "01M04EKP", false},
		{"too long", valid + "X", false},
		{"excluded letter I", strings.Repeat("I", 26), false},
		{"lowercase", strings.ToLower(valid), false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidULID(tt.input); got != tt.want {
				t.Errorf("ValidULID(%q) = %v; want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSetStatusManagesCompletedAt(t *testing.T) {
	task, err := New("認証方式の変更")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if task.Timestamps.CompletedAt != nil {
		t.Fatal("new task should not have completed_at")
	}

	task.SetStatus(StatusDone)
	if task.Timestamps.CompletedAt == nil {
		t.Fatal("done task should have completed_at")
	}

	// doneから戻したら完了時刻は消える。
	task.SetStatus(StatusActive)
	if task.Timestamps.CompletedAt != nil {
		t.Error("reopened task should clear completed_at")
	}
}

func TestValidateWaitingCheck(t *testing.T) {
	checkAt := time.Date(2026, 8, 17, 9, 0, 0, 0, time.FixedZone("JST", 9*60*60))

	tests := []struct {
		name    string
		mutate  func(*Task)
		wantErr bool
	}{
		{
			name: "waiting with check time and action",
			mutate: func(tk *Task) {
				tk.SetStatus(StatusWaiting)
				tk.CheckAt = &checkAt
				tk.CheckAction = "PRのレビュー状況を確認する"
			},
		},
		{
			name: "check time requires action",
			mutate: func(tk *Task) {
				tk.SetStatus(StatusWaiting)
				tk.CheckAt = &checkAt
			},
			wantErr: true,
		},
		{
			name: "check fields require waiting",
			mutate: func(tk *Task) {
				tk.CheckAt = &checkAt
				tk.CheckAction = "PRのレビュー状況を確認する"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk, err := New("確認時刻のテスト")
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			tt.mutate(tk)
			if err := tk.Validate(); (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v; wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSetStatusClearsWaitingCheck(t *testing.T) {
	tk, err := New("確認時刻のテスト")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	checkAt := Now().Add(time.Hour)
	tk.SetStatus(StatusWaiting)
	tk.CheckAt = &checkAt
	tk.CheckAction = "CIの結果を確認する"

	tk.SetStatus(StatusActive)
	if tk.CheckAt != nil || tk.CheckAction != "" {
		t.Errorf("waiting check was not cleared: CheckAt=%v CheckAction=%q", tk.CheckAt, tk.CheckAction)
	}
}

func TestSessionStatusAggregatesTaskStatus(t *testing.T) {
	tk, err := New("並行作業")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	tk.AttachSession(AgentSession{Provider: "codex", SessionID: "active"})
	tk.AttachSession(AgentSession{Provider: "codex", SessionID: "waiting"})
	if err := tk.UpdateSession("codex", "waiting", func(s *AgentSession) error {
		s.SetStatus(StatusWaiting)
		s.ResumeWhen = "レビュー後"
		return nil
	}); err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	if tk.Status != StatusActive {
		t.Fatalf("Status = %q; want active while one session is active", tk.Status)
	}
	if err := tk.UpdateSession("codex", "active", func(s *AgentSession) error {
		s.SetStatus(StatusWaiting)
		return nil
	}); err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	if tk.Status != StatusWaiting {
		t.Fatalf("Status = %q; want waiting when all unfinished sessions are waiting", tk.Status)
	}
}

func TestSessionNeedsMeIsVisibleWithoutActiveSession(t *testing.T) {
	tk, _ := New("判断待ち")
	tk.AttachSession(AgentSession{Provider: "codex", SessionID: "needs-me"})
	tk.AttachSession(AgentSession{Provider: "claude", SessionID: "waiting"})
	_ = tk.UpdateSession("codex", "needs-me", func(s *AgentSession) error {
		s.SetStatus(StatusNeedsMe)
		return nil
	})
	_ = tk.UpdateSession("claude", "waiting", func(s *AgentSession) error {
		s.SetStatus(StatusWaiting)
		return nil
	})
	if tk.Status != StatusNeedsMe {
		t.Fatalf("Status = %q; want needs_me", tk.Status)
	}
}

func TestCompleteCascadesToSessions(t *testing.T) {
	tk, _ := New("完了")
	tk.AttachSession(AgentSession{Provider: "codex", SessionID: "one"})
	tk.AttachSession(AgentSession{Provider: "claude", SessionID: "two"})
	tk.Complete()
	if tk.Status != StatusDone {
		t.Fatalf("Task Status = %q; want done", tk.Status)
	}
	for _, s := range tk.AgentSessions {
		if s.Status != StatusDone || s.CompletedAt == nil {
			t.Errorf("session %s = status %q completed_at %v; want done with completed_at", s.SessionID, s.Status, s.CompletedAt)
		}
	}
}

func TestAttachToCompletedTaskKeepsSessionDone(t *testing.T) {
	tk, _ := New("完了済み")
	tk.SetStatus(StatusDone)
	tk.AttachSession(AgentSession{Provider: "codex", SessionID: "late"})
	if tk.AgentSessions[0].Status != StatusDone {
		t.Fatalf("attached session status = %q; want done", tk.AgentSessions[0].Status)
	}
}

func TestLegacySessionDoesNotOverrideTaskStatus(t *testing.T) {
	tk, _ := New("旧データ")
	tk.SetStatus(StatusWaiting)
	tk.AgentSessions = []AgentSession{{Provider: "codex", SessionID: "legacy"}}
	tk.RecomputeStatus()
	if tk.Status != StatusWaiting {
		t.Fatalf("Status = %q; want legacy task status to remain waiting", tk.Status)
	}
}

func TestValidateSessionWaitingCheck(t *testing.T) {
	tk, _ := New("確認")
	checkAt := Now().Add(time.Hour)
	tk.AgentSessions = []AgentSession{{
		Provider: "codex", SessionID: "one", Status: StatusWaiting,
		CheckAt: &checkAt,
	}}
	if err := tk.Validate(); err == nil {
		t.Fatal("Validate() should reject a session check_at without check_action")
	}
}

func TestSeqIDDoesNotCollideAfterRemoval(t *testing.T) {
	task, err := New("t")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	first, err := task.AddNote("one")
	if err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
	second, err := task.AddNote("two")
	if err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
	if first != "note-1" || second != "note-2" {
		t.Fatalf("unexpected ids: %q, %q", first, second)
	}

	if err := task.RemoveNote(second); err != nil {
		t.Fatalf("RemoveNote() error = %v", err)
	}

	// 削除後も最大連番+1を使うため、既存IDと衝突しない。
	third, err := task.AddNote("three")
	if err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
	if third != "note-2" {
		t.Errorf("third note id = %q; want note-2 (max remaining is note-1)", third)
	}
	if third == first {
		t.Error("note id collided with existing note")
	}
}

func TestAttachSessionIsIdempotent(t *testing.T) {
	task, err := New("t")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	task.AttachSession(AgentSession{Provider: "claude", SessionID: "s1"})
	task.AttachSession(AgentSession{Provider: "claude", SessionID: "s1", Label: "後から付けたラベル"})

	if len(task.AgentSessions) != 1 {
		t.Fatalf("AgentSessions length = %d; want 1", len(task.AgentSessions))
	}
	if task.AgentSessions[0].Label != "後から付けたラベル" {
		t.Errorf("Label = %q; want updated label", task.AgentSessions[0].Label)
	}

	// 別providerの同一session IDは別物として扱う。
	task.AttachSession(AgentSession{Provider: "codex", SessionID: "s1"})
	if len(task.AgentSessions) != 2 {
		t.Errorf("AgentSessions length = %d; want 2", len(task.AgentSessions))
	}
}

func TestDetachSessionRemovesOnlyTarget(t *testing.T) {
	tk, err := New("t")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	tk.AttachSession(AgentSession{Provider: "codex", SessionID: "user-thread"})
	tk.AttachSession(AgentSession{Provider: "codex", SessionID: "guardian-thread"})

	if err := tk.DetachSession("codex", "guardian-thread"); err != nil {
		t.Fatalf("DetachSession() error = %v", err)
	}
	if len(tk.AgentSessions) != 1 || tk.AgentSessions[0].SessionID != "user-thread" {
		t.Fatalf("AgentSessions = %#v; want only user-thread", tk.AgentSessions)
	}
	if err := tk.DetachSession("codex", "missing"); err == nil {
		t.Error("DetachSession() should reject an unregistered session")
	}
}

func TestLatestSession(t *testing.T) {
	task, err := New("t")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	old := Now().Add(-2 * time.Hour)
	task.AgentSessions = []AgentSession{
		{Provider: "claude", SessionID: "old", LastSeenAt: old},
		{Provider: "codex", SessionID: "new", LastSeenAt: Now()},
		{Provider: "claude", SessionID: "older", LastSeenAt: old.Add(-time.Hour)},
	}

	latest, ok := task.LatestSession()
	if !ok {
		t.Fatal("LatestSession() returned false")
	}
	if latest.SessionID != "new" {
		t.Errorf("LatestSession() = %q; want %q", latest.SessionID, "new")
	}
}

func TestValidate(t *testing.T) {
	valid := func() *Task {
		task, err := New("認証方式の変更")
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		return task
	}

	tests := []struct {
		name    string
		mutate  func(*Task)
		wantErr bool
	}{
		{"valid", func(*Task) {}, false},
		{"empty title", func(tk *Task) { tk.Title = "" }, true},
		{"whitespace title", func(tk *Task) { tk.Title = "   " }, true},
		{"invalid status", func(tk *Task) { tk.Status = "blocked" }, true},
		{"invalid task id", func(tk *Task) { tk.TaskID = "short" }, true},
		{"zero revision", func(tk *Task) { tk.Revision = 0 }, true},
		{"missing schema version", func(tk *Task) { tk.SchemaVersion = 0 }, true},
		{"session without id", func(tk *Task) {
			tk.AgentSessions = []AgentSession{{Provider: "claude"}}
		}, true},
		{"reference without url", func(tk *Task) {
			tk.References = []Reference{{ID: "ref-1", Type: RefWeb}}
		}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := valid()
			tt.mutate(task)
			err := task.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v; wantErr %v", err, tt.wantErr)
			}
		})
	}
}
