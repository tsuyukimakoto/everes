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
		"│  現在地  リリース候補を確認中",
		"│  次の作業  公開前の確認項目を整理する",
		"├─ Sessions",
		"│  ├─ ACTIVE   ↗ codex:実装 — 現在地: 署名済みbuildを作成中 / 次の作業: archiveを検証する",
		"│  └─ WAITING  claude:レビュー — 現在地: metadataの確認を依頼済み / 再開条件: レビューが返ったら修正する",
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
