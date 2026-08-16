package claude

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsuyukimakoto/everes/internal/provider"
)

// TestEncodeProjectPath はsession保存ディレクトリ名の変換規則を固定する。
//
// Claude Codeの非公開仕様に依存するため、観測した変換規則を期待値として固定する。
func TestEncodeProjectPath(t *testing.T) {
	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{
			name: "アンダースコアもハイフンになる",
			cwd:  "/Users/example.user/tmp/everes_task_manager",
			want: "-Users-example-user-tmp-everes-task-manager",
		},
		{
			name: "ドット始まりは連続ハイフンになる",
			cwd:  "/Users/example.user/.claude",
			want: "-Users-example-user--claude",
		},
		{
			name: "英数字は保たれる",
			cwd:  "/Users/foo/projects/app2",
			want: "-Users-foo-projects-app2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := encodeProjectPath(tt.cwd); got != tt.want {
				t.Errorf("encodeProjectPath(%q) = %q; want %q", tt.cwd, got, tt.want)
			}
		})
	}
}

func TestDetectCurrentSessionPicksNewest(t *testing.T) {
	root := t.TempDir()
	cwd := "/Users/foo/dev/app"
	dir := filepath.Join(root, encodeProjectPath(cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	now := time.Now()
	files := []struct {
		name    string
		modTime time.Time
	}{
		{"old-session.jsonl", now.Add(-2 * time.Hour)},
		{"newest-session.jsonl", now},
		{"older-session.jsonl", now.Add(-5 * time.Hour)},
		{"not-a-session.txt", now.Add(time.Hour)},
	}
	for _, f := range files {
		path := filepath.Join(dir, f.name)
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		if err := os.Chtimes(path, f.modTime, f.modTime); err != nil {
			t.Fatalf("Chtimes() error = %v", err)
		}
	}

	p := &Provider{projectsDir: root}
	got, err := p.DetectCurrentSession(cwd)
	if err != nil {
		t.Fatalf("DetectCurrentSession() error = %v", err)
	}
	if got.SessionID != "newest-session" {
		t.Errorf("SessionID = %q; want newest-session", got.SessionID)
	}
	if got.Provider != Name {
		t.Errorf("Provider = %q; want %q", got.Provider, Name)
	}
}

func TestDetectCurrentSessionWithoutProject(t *testing.T) {
	p := &Provider{projectsDir: t.TempDir()}

	_, err := p.DetectCurrentSession("/Users/foo/never/used")
	if !errors.Is(err, provider.ErrNoCurrentSession) {
		t.Errorf("DetectCurrentSession() error = %v; want ErrNoCurrentSession", err)
	}
}

func TestDetectCurrentSessionEmptyProjectDir(t *testing.T) {
	root := t.TempDir()
	cwd := "/Users/foo/dev/app"
	if err := os.MkdirAll(filepath.Join(root, encodeProjectPath(cwd)), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	p := &Provider{projectsDir: root}
	_, err := p.DetectCurrentSession(cwd)
	if !errors.Is(err, provider.ErrNoCurrentSession) {
		t.Errorf("DetectCurrentSession() error = %v; want ErrNoCurrentSession", err)
	}
}

func TestSessionExists(t *testing.T) {
	root := t.TempDir()
	cwd := "/Users/foo/dev/app"
	dir := filepath.Join(root, encodeProjectPath(cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "s1.jsonl"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	p := &Provider{projectsDir: root}

	tests := []struct {
		name string
		sess provider.Session
		want bool
	}{
		{"cwd付きで存在", provider.Session{SessionID: "s1", Cwd: cwd}, true},
		{"cwdなしでも走査で見つかる", provider.Session{SessionID: "s1"}, true},
		{"cwdが違っても走査で見つかる", provider.Session{SessionID: "s1", Cwd: "/other/path"}, true},
		{"存在しない", provider.Session{SessionID: "missing", Cwd: cwd}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.SessionExists(tt.sess)
			if err != nil {
				t.Fatalf("SessionExists() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("SessionExists(%+v) = %v; want %v", tt.sess, got, tt.want)
			}
		})
	}
}
