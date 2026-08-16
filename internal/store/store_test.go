package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tsuyukimakoto/everes/internal/task"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := New(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	return s
}

func newTestTask(t *testing.T, title string) *task.Task {
	t.Helper()
	tk, err := task.New(title)
	if err != nil {
		t.Fatalf("task.New() error = %v", err)
	}
	return tk
}

func TestCreateAndGet(t *testing.T) {
	s := newTestStore(t)
	tk := newTestTask(t, "認証方式の変更")
	tk.Objective = "OAuthベースへ移行する"
	tk.SetStatus(task.StatusWaiting)
	tk.ResumeWhen = "PR #381 のレビュー後"

	if err := s.Create(tk); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := s.Get(tk.TaskID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Title != tk.Title {
		t.Errorf("Title = %q; want %q", got.Title, tk.Title)
	}
	if got.Status != task.StatusWaiting {
		t.Errorf("Status = %q; want waiting", got.Status)
	}
	if got.ResumeWhen != tk.ResumeWhen {
		t.Errorf("ResumeWhen = %q; want %q", got.ResumeWhen, tk.ResumeWhen)
	}
}

func TestCreateRejectsDuplicate(t *testing.T) {
	s := newTestStore(t)
	tk := newTestTask(t, "t")

	if err := s.Create(tk); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := s.Create(tk); err == nil {
		t.Error("Create() with duplicate id should fail")
	}
}

func TestGetNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Get("01M04EKP5KWJHQTHRDXZK9FWYD")
	if !errors.Is(err, task.ErrNotFound) {
		t.Errorf("Get() error = %v; want task.ErrNotFound", err)
	}
}

func TestUpdateIncrementsRevision(t *testing.T) {
	s := newTestStore(t)
	tk := newTestTask(t, "t")
	if err := s.Create(tk); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated, err := s.Update(tk.TaskID, 0, func(tk *task.Task) error {
		tk.Summary = "レビュー待ち"
		return nil
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Revision != 2 {
		t.Errorf("Revision = %d; want 2", updated.Revision)
	}
	if updated.Summary != "レビュー待ち" {
		t.Errorf("Summary = %q; want レビュー待ち", updated.Summary)
	}
}

func TestUpdateRevisionConflict(t *testing.T) {
	s := newTestStore(t)
	tk := newTestTask(t, "t")
	if err := s.Create(tk); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// 別のsessionが先に更新した状況を作る。
	if _, err := s.Update(tk.TaskID, 0, func(tk *task.Task) error {
		tk.Summary = "先に入った更新"
		return nil
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// revision 1 を前提にした更新は拒否される。
	_, err := s.Update(tk.TaskID, 1, func(tk *task.Task) error {
		tk.Summary = "後から来た更新"
		return nil
	})
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("Update() error = %v; want ErrRevisionConflict", err)
	}

	got, err := s.Get(tk.TaskID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Summary != "先に入った更新" {
		t.Errorf("Summary = %q; conflicting update must not be applied", got.Summary)
	}
}

func TestUpdateMutateErrorLeavesTaskUnchanged(t *testing.T) {
	s := newTestStore(t)
	tk := newTestTask(t, "t")
	if err := s.Create(tk); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	wantErr := errors.New("mutate failed")
	if _, err := s.Update(tk.TaskID, 0, func(tk *task.Task) error {
		tk.Summary = "書き換えたが失敗する"
		return wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("Update() error = %v; want %v", err, wantErr)
	}

	got, err := s.Get(tk.TaskID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Summary != "" {
		t.Errorf("Summary = %q; want empty (update must not persist)", got.Summary)
	}
	if got.Revision != 1 {
		t.Errorf("Revision = %d; want 1", got.Revision)
	}
}

// TestConcurrentUpdates は同時更新が失われないことを確認する。
// ClaudeとCodexから同時に更新される状況を模す。
func TestConcurrentUpdates(t *testing.T) {
	s := newTestStore(t)
	tk := newTestTask(t, "t")
	if err := s.Create(tk); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Update(tk.TaskID, 0, func(tk *task.Task) error {
				if _, err := tk.AddNote("note"); err != nil {
					return err
				}
				return nil
			}); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent Update() error = %v", err)
	}

	got, err := s.Get(tk.TaskID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(got.Notes) != n {
		t.Errorf("Notes length = %d; want %d (updates were lost)", len(got.Notes), n)
	}

	// IDが重複していないこと。
	seen := map[string]bool{}
	for _, note := range got.Notes {
		if seen[note.ID] {
			t.Fatalf("duplicate note id %q", note.ID)
		}
		seen[note.ID] = true
	}
}

func TestListOrdersByStatusThenUpdatedAt(t *testing.T) {
	s := newTestStore(t)

	statuses := []task.Status{
		task.StatusDone,
		task.StatusActive,
		task.StatusNeedsMe,
		task.StatusPaused,
		task.StatusWaiting,
	}
	for _, st := range statuses {
		tk := newTestTask(t, string(st))
		tk.SetStatus(st)
		if err := s.Create(tk); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	tasks, err := s.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != len(statuses) {
		t.Fatalf("List() length = %d; want %d", len(tasks), len(statuses))
	}

	// 人間の注意を要する順に並ぶ。
	want := []task.Status{
		task.StatusNeedsMe,
		task.StatusWaiting,
		task.StatusActive,
		task.StatusPaused,
		task.StatusDone,
	}
	for i, w := range want {
		if tasks[i].Status != w {
			t.Errorf("tasks[%d].Status = %q; want %q", i, tasks[i].Status, w)
		}
	}
}

func TestListPlacesOverdueWaitingBeforeFutureWaiting(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	future := newTestTask(t, "future")
	future.SetStatus(task.StatusWaiting)
	futureAt := now.Add(24 * time.Hour)
	future.CheckAt = &futureAt
	future.CheckAction = "明日のCIを確認する"
	if err := s.Create(future); err != nil {
		t.Fatalf("Create(future) error = %v", err)
	}

	overdue := newTestTask(t, "overdue")
	overdue.SetStatus(task.StatusWaiting)
	overdueAt := now.Add(-24 * time.Hour)
	overdue.CheckAt = &overdueAt
	overdue.CheckAction = "PRのレビュー状況を確認する"
	if err := s.Create(overdue); err != nil {
		t.Fatalf("Create(overdue) error = %v", err)
	}

	tasks, err := s.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 2 || tasks[0].Title != "overdue" {
		t.Fatalf("List() order = %v; want overdue first", []string{tasks[0].Title, tasks[1].Title})
	}
}

func TestListEmpty(t *testing.T) {
	s := newTestStore(t)
	tasks, err := s.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("List() length = %d; want 0", len(tasks))
	}
}

func TestResolveID(t *testing.T) {
	s := newTestStore(t)
	tk := newTestTask(t, "t")
	if err := s.Create(tk); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"full id", tk.TaskID, tk.TaskID, false},
		{"prefix", tk.TaskID[:8], tk.TaskID, false},
		{"lowercase prefix", strings.ToLower(tk.TaskID[:8]), tk.TaskID, false},
		{"unknown", "01ZZZZZZ", "", true},
		{"empty", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ResolveID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveID(%q) error = %v; wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ResolveID(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveIDAmbiguous(t *testing.T) {
	s := newTestStore(t)

	// 同じ接頭辞を持つ2件を人為的に作る。
	for _, suffix := range []string{"A", "B"} {
		tk := newTestTask(t, "t")
		tk.TaskID = "01M04EKP5KWJHQTHRDXZK9FWY" + suffix
		if err := s.Create(tk); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	if _, err := s.ResolveID("01M04EKP"); err == nil {
		t.Error("ResolveID() with ambiguous prefix should fail")
	}
}

func TestAtomicWriteLeavesNoTempFile(t *testing.T) {
	s := newTestStore(t)
	tk := newTestTask(t, "t")
	if err := s.Create(tk); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	entries, err := os.ReadDir(s.tasksDir())
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Errorf("tasks directory has %d entries; want 1", len(entries))
	}
}

func TestWrittenJSONMatchesSchemaShape(t *testing.T) {
	s := newTestStore(t)
	tk := newTestTask(t, "t")
	if err := s.Create(tk); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(s.tasksDir(), tk.TaskID+".json"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	for _, key := range []string{
		"schema_version", "task_id", "revision", "title", "status", "timestamps",
	} {
		if _, ok := doc[key]; !ok {
			t.Errorf("written json is missing required key %q", key)
		}
	}
	if v, _ := doc["schema_version"].(float64); int(v) != task.SchemaVersion {
		t.Errorf("schema_version = %v; want %d", doc["schema_version"], task.SchemaVersion)
	}
}
