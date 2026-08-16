package task

import "testing"

func TestDetectRefType(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want RefType
	}{
		{"github issue", "https://github.com/example/foo/issues/381", RefGitHubIssue},
		{"github pr", "https://github.com/example/foo/pull/12", RefGitHubPR},
		{"github repo top", "https://github.com/example/foo", RefWeb},
		{"github discussions", "https://github.com/example/foo/discussions/3", RefWeb},
		{"google doc", "https://docs.google.com/document/d/abc/edit", RefGoogleDoc},
		{"google sheet", "https://docs.google.com/spreadsheets/d/abc", RefGoogleSheet},
		{"google slide", "https://docs.google.com/presentation/d/abc", RefGoogleSlide},
		{"notion", "https://www.notion.so/workspace/page", RefNotion},
		{"jira", "https://example.atlassian.net/browse/ABC-1", RefJira},
		{"linear", "https://linear.app/team/issue/ENG-1", RefLinear},
		{"plain web", "https://example.com/page", RefWeb},
		{"file scheme", "file:///Users/foo/memo.md", RefFile},
		{"absolute path", "/Users/foo/memo.md", RefFile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectRefType(tt.url); got != tt.want {
				t.Errorf("DetectRefType(%q) = %q; want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestInspectSecrets(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantWarn bool
	}{
		{"clean url", "https://github.com/example/foo/issues/381", false},
		{"access_token", "https://example.com/d?access_token=x", true},
		{"api_key", "https://example.com/d?api_key=x", true},
		{"apikey", "https://example.com/d?apikey=x", true},
		{"token", "https://example.com/d?token=x", true},
		{"secret", "https://example.com/d?client_secret=x", true},
		{"signature", "https://example.com/d?signature=x", true},
		{"password in userinfo", "https://user:pass@example.com/d", true},
		{"harmless query", "https://example.com/d?page=2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InspectSecrets(tt.url)
			if (len(got) > 0) != tt.wantWarn {
				t.Errorf("InspectSecrets(%q) = %v; wantWarn %v", tt.url, got, tt.wantWarn)
			}
		})
	}
}

func TestAddReferenceDetectsTypeAndDefaultsTitle(t *testing.T) {
	task, err := New("t")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	id, err := task.AddReference("https://github.com/example/foo/issues/381", "", "")
	if err != nil {
		t.Fatalf("AddReference() error = %v", err)
	}
	if id != "ref-1" {
		t.Errorf("id = %q; want ref-1", id)
	}

	ref := task.References[0]
	if ref.Type != RefGitHubIssue {
		t.Errorf("Type = %q; want %q", ref.Type, RefGitHubIssue)
	}
	// titleが空ならURLで代替する。一覧で何も出ないより読める。
	if ref.Title != "https://github.com/example/foo/issues/381" {
		t.Errorf("Title = %q; want the url", ref.Title)
	}
}

func TestAddReferenceRejectsEmptyURL(t *testing.T) {
	task, err := New("t")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := task.AddReference("", "title", RefWeb); err == nil {
		t.Error("AddReference() with empty url should fail")
	}
}
