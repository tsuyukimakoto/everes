package codex

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsuyukimakoto/everes/internal/provider"
)

func TestAppURLTargetsCodexThread(t *testing.T) {
	p := New()
	got, err := p.AppURL(provider.Session{SessionID: "01900000-0000-7000-8000-000000000001"})
	if err != nil {
		t.Fatalf("AppURL() error = %v", err)
	}
	want := "codex://threads/01900000-0000-7000-8000-000000000001"
	if got != want {
		t.Errorf("AppURL() = %q; want %q", got, want)
	}
}

func TestThreadIDFromFile(t *testing.T) {
	tests := []struct {
		name string
		file string
		want string
	}{
		{
			name: "rollout filename",
			file: "rollout-2025-01-02T14-24-53-01900000-0000-7000-8000-000000000001.jsonl",
			want: "01900000-0000-7000-8000-000000000001",
		},
		{
			name: "plain uuid filename",
			file: "01900000-0000-7000-8000-000000000001.jsonl",
			want: "01900000-0000-7000-8000-000000000001",
		},
		{name: "non session file", file: "history.jsonl", want: ""},
		{name: "unsupported extension", file: "01900000-0000-7000-8000-000000000001.txt", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := threadIDFromFile(tt.file); got != tt.want {
				t.Errorf("threadIDFromFile(%q) = %q; want %q", tt.file, got, tt.want)
			}
		})
	}
}

func TestDetectCurrentSessionMatchesCwd(t *testing.T) {
	root := t.TempDir()
	wantedCwd := "/Users/example/projects/wanted"
	otherCwd := "/Users/example/projects/other"

	wanted := writeSessionFile(t, root,
		"rollout-2025-01-02T12-00-00-01900000-0000-7000-8000-000000000001.jsonl",
		"01900000-0000-7000-8000-000000000001", wantedCwd, time.Now().Add(-time.Hour))
	writeSessionFile(t, root,
		"rollout-2025-01-02T13-00-00-01900000-0000-7000-8000-000000000002.jsonl",
		"01900000-0000-7000-8000-000000000002", otherCwd, time.Now())

	p := &Provider{sessionsDir: root}
	got, err := p.DetectCurrentSession(wantedCwd)
	if err != nil {
		t.Fatalf("DetectCurrentSession() error = %v", err)
	}
	if got.SessionID != "01900000-0000-7000-8000-000000000001" {
		t.Errorf("SessionID = %q; want UUID from matching cwd", got.SessionID)
	}

	metadata, err := p.SessionMetadata(*got)
	if err != nil {
		t.Fatalf("SessionMetadata() error = %v", err)
	}
	if metadata["path"] != wanted {
		t.Errorf("metadata path = %q; want %q", metadata["path"], wanted)
	}
}

func TestDetectCurrentSessionIgnoresMalformedMetadata(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root,
		"rollout-2025-01-02T12-00-00-01900000-0000-7000-8000-000000000001.jsonl")
	if err := os.WriteFile(path, []byte("not json\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	p := &Provider{sessionsDir: root}
	if _, err := p.DetectCurrentSession("/Users/example/projects/wanted"); err == nil {
		t.Fatal("DetectCurrentSession() should fail when no session metadata matches")
	}
}

func TestDetectCurrentSessionIgnoresSubagentThreads(t *testing.T) {
	root := t.TempDir()
	cwd := "/Users/example/projects/wanted"

	writeSessionFileWithSource(t, root,
		"rollout-2025-01-02T12-00-00-01900000-0000-7000-8000-000000000001.jsonl",
		"01900000-0000-7000-8000-000000000001", cwd, "user", time.Now().Add(-time.Hour))
	writeSessionFileWithSource(t, root,
		"rollout-2025-01-02T13-00-00-01900000-0000-7000-8000-000000000002.jsonl",
		"01900000-0000-7000-8000-000000000002", cwd, "subagent", time.Now())

	p := &Provider{sessionsDir: root}
	got, err := p.DetectCurrentSession(cwd)
	if err != nil {
		t.Fatalf("DetectCurrentSession() error = %v", err)
	}
	if got.SessionID != "01900000-0000-7000-8000-000000000001" {
		t.Errorf("SessionID = %q; want user thread", got.SessionID)
	}
}

func writeSessionFile(t *testing.T, root, name, id, cwd string, modTime time.Time) string {
	return writeSessionFileWithSource(t, root, name, id, cwd, "", modTime)
}

func writeSessionFileWithSource(t *testing.T, root, name, id, cwd, threadSource string, modTime time.Time) string {
	t.Helper()
	path := filepath.Join(root, name)
	source := ""
	if threadSource != "" {
		source = `,"thread_source":"` + threadSource + `"`
	}
	data := []byte(`{"type":"session_meta","payload":{"id":"` + id + `","cwd":"` + cwd + `"` + source + `}}` + "\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}
	return path
}
