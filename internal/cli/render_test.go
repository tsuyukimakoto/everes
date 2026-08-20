package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/tsuyukimakoto/everes/internal/task"
)

func TestRenderListShowsTaskAndSessionsAsHierarchy(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.Local)
	tk, err := task.New("公開準備")
	if err != nil {
		t.Fatalf("task.New() error = %v", err)
	}
	tk.Summary = "リリース候補を確認中"
	tk.NextAction = "公開前の確認項目を整理する"
	tk.Timestamps.UpdatedAt = now.Add(-20 * time.Minute)
	tk.AgentSessions = []task.AgentSession{
		{
			Provider:   "codex",
			SessionID:  "01900000-0000-7000-8000-000000000001",
			Label:      "実装",
			Status:     task.StatusActive,
			Summary:    "署名済みbuildを作成中",
			NextAction: "archiveを検証する",
		},
		{
			Provider:   "claude",
			SessionID:  "01900000-0000-7000-8000-000000000002",
			Label:      "レビュー",
			Status:     task.StatusWaiting,
			Summary:    "metadataの確認を依頼済み",
			ResumeWhen: "レビューが返ったら修正する",
		},
	}
	tk.RecomputeStatus()

	var out strings.Builder
	renderList(&out, []*task.Task{tk}, now)
	got := out.String()

	for _, want := range []string{
		"╭─ " + task.ShortID(tk.TaskID) + "  公開準備",
		"│  Task  ACTIVE (2 sessions)",
		"│  現在地\n│    リリース候補を確認中",
		"│  次の作業\n│    公開前の確認項目を整理する",
		"├─ Sessions",
		"│  ┌─ ACTIVE  ↗ codex:実装",
		"│  │  現在地\n│  │    署名済みbuildを作成中",
		"│  │  次の作業\n│  │    archiveを検証する",
		"│  ┌─ WAITING  claude:レビュー",
		"│  │  現在地\n│  │    metadataの確認を依頼済み",
		"│  │  再開条件\n│  │    レビューが返ったら修正する",
		"╰─",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("renderList() =\n%s\nwant substring %q", got, want)
		}
	}

	for _, unwanted := range []string{"sessions:", "session status:"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("renderList() =\n%s\nmust not contain legacy summary %q", got, unwanted)
		}
	}
}

func TestRenderListWrapsStagedFieldsToTerminalWidth(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.Local)
	tk, err := task.New("表示確認")
	if err != nil {
		t.Fatalf("task.New() error = %v", err)
	}
	tk.Summary = "日本語の長い説明でもセッションごとの範囲が分かるように折り返して表示する"
	tk.AgentSessions = []task.AgentSession{{
		Provider:  "codex",
		SessionID: "session-1",
		Label:     "実装",
		Status:    task.StatusActive,
		Summary:   "横に長い文字列を本文専用の行へ分離して表示する",
	}}
	tk.RecomputeStatus()

	var out strings.Builder
	renderListWithWidth(&out, []*task.Task{tk}, now, false, 40)
	for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
		if got := terminalDisplayWidth(line); got > 40 {
			t.Errorf("line width = %d; want <= 40:\n%s", got, line)
		}
	}
	if !strings.Contains(out.String(), "│    日本語の長い説明でもセッションごと\n│    の範囲が分かるように折り返して表示") {
		t.Errorf("rendered list did not wrap task summary with continuation indent:\n%s", out.String())
	}
}

func TestWrapTerminalTextDoesNotStartLineWithJapanesePunctuation(t *testing.T) {
	got := wrapTerminalText("あいうえお。かき", 10)
	want := []string{"あいうえ", "お。かき"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("wrapTerminalText() = %q; want %q", got, want)
	}
	for _, line := range got {
		if terminalDisplayWidth(line) > 10 {
			t.Errorf("line width = %d; want <= 10: %q", terminalDisplayWidth(line), line)
		}
	}
}
