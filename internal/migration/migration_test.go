package migration

import (
	"strings"
	"testing"
)

func TestMigrateLatestIsUnchanged(t *testing.T) {
	doc := map[string]any{
		"schema_version": float64(Latest),
		"title":          "認証方式の変更",
	}

	got, err := Migrate(doc)
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if v, _ := got["schema_version"].(float64); int(v) != Latest {
		t.Errorf("schema_version = %v; want %d", got["schema_version"], Latest)
	}
	if got["title"] != "認証方式の変更" {
		t.Errorf("title = %v; want unchanged", got["title"])
	}
}

func TestMigrateV1PreservesLegacySessionWithoutGuessingStatus(t *testing.T) {
	doc := map[string]any{
		"schema_version": float64(1),
		"status":         "waiting",
		"agent_sessions": []any{map[string]any{
			"provider": "codex", "session_id": "legacy",
		}},
	}
	got, err := Migrate(doc)
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if int(got["schema_version"].(float64)) != 2 {
		t.Fatalf("schema_version = %v; want 2", got["schema_version"])
	}
	session := got["agent_sessions"].([]any)[0].(map[string]any)
	if _, ok := session["status"]; ok {
		t.Fatalf("legacy session status was guessed: %#v", session["status"])
	}
}

func TestMigrateRejectsNewerVersion(t *testing.T) {
	doc := map[string]any{"schema_version": float64(Latest + 1)}

	_, err := Migrate(doc)
	if err == nil {
		t.Fatal("Migrate() with newer schema_version should fail")
	}
	// 古いバイナリで新しいファイルを壊さないよう、更新を促すメッセージにする。
	if !strings.Contains(err.Error(), "update everes") {
		t.Errorf("error = %v; want a message telling the user to update", err)
	}
}

func TestMigrateRejectsInvalidVersion(t *testing.T) {
	tests := []struct {
		name string
		doc  map[string]any
	}{
		{"missing", map[string]any{"title": "t"}},
		{"zero", map[string]any{"schema_version": float64(0)}},
		{"negative", map[string]any{"schema_version": float64(-1)}},
		{"wrong type", map[string]any{"schema_version": "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Migrate(tt.doc); err == nil {
				t.Error("Migrate() should fail")
			}
		})
	}
}

func TestMigrateAppliesV1ToV2(t *testing.T) {
	doc := map[string]any{"schema_version": float64(1)}
	got, err := Migrate(doc)
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if int(got["schema_version"].(float64)) != Latest {
		t.Fatalf("schema_version = %v; want %d", got["schema_version"], Latest)
	}
}
