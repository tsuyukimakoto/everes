package cli

import (
	"flag"
	"io"
	"reflect"
	"testing"
)

func TestListTerminalWidthUsesColumns(t *testing.T) {
	t.Setenv("COLUMNS", "72")
	if got := listTerminalWidth(); got != 72 {
		t.Errorf("listTerminalWidth() = %d; want 72", got)
	}
}

func TestCurrentVersionUsesLinkedVersion(t *testing.T) {
	original := Version
	Version = "0.2.0"
	t.Cleanup(func() { Version = original })

	if got := currentVersion(); got != "0.2.0" {
		t.Fatalf("currentVersion() = %q, want %q", got, "0.2.0")
	}
}

func TestResolveVersionUsesGoModuleVersion(t *testing.T) {
	if got := resolveVersion("dev", "v0.0.0-20260819234055-57f0dfd88636"); got != "v0.0.0-20260819234055-57f0dfd88636" {
		t.Fatalf("resolveVersion() = %q", got)
	}
}

func TestResolveVersionFallsBackToDev(t *testing.T) {
	if got := resolveVersion("dev", "(devel)"); got != "dev" {
		t.Fatalf("resolveVersion() = %q, want %q", got, "dev")
	}
}

// TestReorderArgs はフラグを位置引数の後ろに書いても解釈できることを確認する。
//
// 標準のflagは最初の非フラグ引数でパースを止めるため、
// everes ref add <id> <url> --title "..." のような語順が黙って無視されていた。
func TestReorderArgs(t *testing.T) {
	newFS := func() *flagSetForTest {
		fs := newFlagSet("test", io.Discard)
		title := fs.String("title", "", "")
		typeName := fs.String("type", "", "")
		asJSON := fs.Bool("json", false, "")
		return &flagSetForTest{fs: fs, title: title, typeName: typeName, asJSON: asJSON}
	}

	tests := []struct {
		name           string
		args           []string
		wantPositional []string
		wantTitle      string
		wantJSON       bool
	}{
		{
			name:           "フラグが後ろ",
			args:           []string{"01M04EKP", "https://example.com", "--title", "認証方式変更"},
			wantPositional: []string{"01M04EKP", "https://example.com"},
			wantTitle:      "認証方式変更",
		},
		{
			name:           "フラグが前",
			args:           []string{"--title", "認証方式変更", "01M04EKP", "https://example.com"},
			wantPositional: []string{"01M04EKP", "https://example.com"},
			wantTitle:      "認証方式変更",
		},
		{
			name:           "フラグが混在",
			args:           []string{"--json", "01M04EKP", "--title", "t", "https://example.com"},
			wantPositional: []string{"01M04EKP", "https://example.com"},
			wantTitle:      "t",
			wantJSON:       true,
		},
		{
			name:           "boolフラグは値を食わない",
			args:           []string{"01M04EKP", "--json"},
			wantPositional: []string{"01M04EKP"},
			wantJSON:       true,
		},
		{
			name:           "イコール形式",
			args:           []string{"01M04EKP", "--title=認証方式変更"},
			wantPositional: []string{"01M04EKP"},
			wantTitle:      "認証方式変更",
		},
		{
			name:           "ダブルダッシュ以降は位置引数",
			args:           []string{"01M04EKP", "--", "--title", "これは本文"},
			wantPositional: []string{"01M04EKP", "--title", "これは本文"},
			wantTitle:      "",
		},
		{
			name:           "位置引数のみ",
			args:           []string{"01M04EKP", "note-1"},
			wantPositional: []string{"01M04EKP", "note-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newFS()
			if err := parseFlags(h.fs, tt.args); err != nil {
				t.Fatalf("parseFlags() error = %v", err)
			}
			if got := h.fs.Args(); !reflect.DeepEqual(got, tt.wantPositional) {
				t.Errorf("positional = %q; want %q", got, tt.wantPositional)
			}
			if *h.title != tt.wantTitle {
				t.Errorf("title = %q; want %q", *h.title, tt.wantTitle)
			}
			if *h.asJSON != tt.wantJSON {
				t.Errorf("json = %v; want %v", *h.asJSON, tt.wantJSON)
			}
		})
	}
}

// TestReorderArgsKeepsNoteTextIntact は note add の本文が壊れないことを確認する。
func TestReorderArgsKeepsNoteTextIntact(t *testing.T) {
	fs := newFlagSet("note add", io.Discard)
	asJSON := fs.Bool("json", false, "")

	args := []string{"01M04EKP", "--", "refresh", "tokenについては", "A案を採用した"}
	if err := parseFlags(fs, args); err != nil {
		t.Fatalf("parseFlags() error = %v", err)
	}

	want := []string{"01M04EKP", "refresh", "tokenについては", "A案を採用した"}
	if got := fs.Args(); !reflect.DeepEqual(got, want) {
		t.Errorf("args = %q; want %q", got, want)
	}
	if *asJSON {
		t.Error("json flag should not be set")
	}
}

type flagSetForTest struct {
	fs       *flag.FlagSet
	title    *string
	typeName *string
	asJSON   *bool
}
