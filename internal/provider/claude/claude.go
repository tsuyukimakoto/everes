// Package claude はClaude Code固有の処理を閉じ込める。
//
// session保存レイアウトと再開方法はClaude Codeの非公開仕様に依存する。
// Everes coreはこれらを知らない。仕様変更で壊れた場合はこのパッケージのみ差し替える。
package claude

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tsuyukimakoto/everes/internal/provider"
)

// Name はprovider識別子。
const Name = "claude"

// Provider はClaude Code用のAdapter。
type Provider struct {
	// projectsDir はsession保存先。テストで差し替える。
	projectsDir string
}

func init() {
	provider.Register(New())
}

// New はAdapterを生成する。
func New() *Provider {
	return &Provider{projectsDir: defaultProjectsDir()}
}

func defaultProjectsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects")
}

// Name はprovider識別子を返す。
func (p *Provider) Name() string { return Name }

// encodeProjectPath はcwdをsession保存ディレクトリ名へ変換する。
//
// Claude Codeは英数字以外をすべて "-" に置き換えたディレクトリ名を使う。
//
//	/Users/foo/.claude              -> -Users-foo--claude
//	/Users/foo/tmp/everes_task_mgr  -> -Users-foo-tmp-everes-task-mgr
//
// 非公開仕様のため、変換規則が変わった場合はここだけを直す。
func encodeProjectPath(cwd string) string {
	var b strings.Builder
	b.Grow(len(cwd))
	for _, r := range cwd {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// DetectCurrentSession はcwdに対応するsessionのうち最後に更新されたものを返す。
//
// 同じディレクトリで複数sessionを同時に走らせている場合、この推定は当てにならない。
// そのためhookがcontextへ出力したsession IDを明示的に渡す経路を主とし、
// これはあくまで補助手段とする。
func (p *Provider) DetectCurrentSession(cwd string) (*provider.Session, error) {
	if p.projectsDir == "" || cwd == "" {
		return nil, provider.ErrNoCurrentSession
	}

	dir := filepath.Join(p.projectsDir, encodeProjectPath(cwd))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, provider.ErrNoCurrentSession
		}
		return nil, fmt.Errorf("read claude project directory: %w", err)
	}

	type candidate struct {
		id      string
		modTime time.Time
	}
	var candidates []candidate
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{
			id:      strings.TrimSuffix(e.Name(), ".jsonl"),
			modTime: info.ModTime(),
		})
	}
	if len(candidates) == 0 {
		return nil, provider.ErrNoCurrentSession
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.After(candidates[j].modTime)
	})

	return &provider.Session{
		Provider:  Name,
		SessionID: candidates[0].id,
		Cwd:       cwd,
	}, nil
}

// SessionExists はsessionのログファイルが残っているかを返す。
func (p *Provider) SessionExists(s provider.Session) (bool, error) {
	path, err := p.sessionFile(s)
	if err != nil {
		return false, err
	}
	return path != "", nil
}

// SessionMetadata はsessionログの所在と更新時刻を返す。
func (p *Provider) SessionMetadata(s provider.Session) (map[string]any, error) {
	path, err := p.sessionFile(s)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return map[string]any{"exists": false}, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat session file: %w", err)
	}
	return map[string]any{
		"exists":      true,
		"path":        path,
		"size_bytes":  info.Size(),
		"modified_at": info.ModTime().Format(time.RFC3339),
		"project_dir": filepath.Base(filepath.Dir(path)),
	}, nil
}

// OpenSession は claude --resume でsessionを再開する。
func (p *Provider) OpenSession(s provider.Session) error {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude CLI not found in PATH: %w", err)
	}

	cmd := exec.Command(bin, "--resume", s.SessionID)
	if s.Cwd != "" {
		cmd.Dir = s.Cwd
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("resume claude session %s: %w", s.SessionID, err)
	}
	return nil
}

// sessionFile はsessionのログファイルパスを探す。見つからなければ空文字を返す。
func (p *Provider) sessionFile(s provider.Session) (string, error) {
	if p.projectsDir == "" {
		return "", nil
	}

	// cwdが分かっていれば直接引く。
	if s.Cwd != "" {
		path := filepath.Join(p.projectsDir, encodeProjectPath(s.Cwd), s.SessionID+".jsonl")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// cwdが不明、または移動している場合は全プロジェクトを走査する。
	entries, err := os.ReadDir(p.projectsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read claude projects directory: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(p.projectsDir, e.Name(), s.SessionID+".jsonl")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", nil
}
