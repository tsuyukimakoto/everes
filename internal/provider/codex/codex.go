// Package codex はCodex固有の処理を閉じ込める。
package codex

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/tsuyukimakoto/everes/internal/provider"
)

// Name はprovider識別子。
const Name = "codex"

// Provider はCodex用のAdapter。
type Provider struct {
	sessionsDir string
}

func init() {
	provider.Register(New())
}

// New はAdapterを生成する。
func New() *Provider {
	return &Provider{sessionsDir: defaultSessionsDir()}
}

func defaultSessionsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex", "sessions")
}

// Name はprovider識別子を返す。
func (p *Provider) Name() string { return Name }

// AppURL はCodexアプリでthreadを表示する公式deep linkを返す。
func (p *Provider) AppURL(s provider.Session) (string, error) {
	if s.SessionID == "" {
		return "", errors.New("codex session ID is required")
	}
	return (&url.URL{Scheme: "codex", Host: "threads", Path: "/" + s.SessionID}).String(), nil
}

// DetectCurrentSession はcwdに対応する保存済みthreadのうち最後に更新されたものを返す。
func (p *Provider) DetectCurrentSession(cwd string) (*provider.Session, error) {
	if p.sessionsDir == "" {
		return nil, provider.ErrNoCurrentSession
	}

	newest, err := p.newestThread(cwd)
	if err != nil {
		return nil, err
	}
	if newest == "" {
		return nil, provider.ErrNoCurrentSession
	}

	return &provider.Session{
		Provider:  Name,
		SessionID: newest,
		Cwd:       cwd,
	}, nil
}

// SessionExists はthreadの記録が残っているかを返す。
func (p *Provider) SessionExists(s provider.Session) (bool, error) {
	path, err := p.threadFile(s.SessionID)
	if err != nil {
		return false, err
	}
	return path != "", nil
}

// SessionMetadata はthreadファイルの所在と更新時刻を返す。
func (p *Provider) SessionMetadata(s provider.Session) (map[string]any, error) {
	path, err := p.threadFile(s.SessionID)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return map[string]any{"exists": false}, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat thread file: %w", err)
	}
	metadata := map[string]any{
		"exists":      true,
		"path":        path,
		"size_bytes":  info.Size(),
		"modified_at": info.ModTime().Format(time.RFC3339),
	}
	if sessionMeta, err := sessionMetadataFromFile(path); err == nil {
		if sessionMeta.ThreadSource != "" {
			metadata["thread_source"] = sessionMeta.ThreadSource
		}
		if sessionMeta.ParentThreadID != "" {
			metadata["parent_thread_id"] = sessionMeta.ParentThreadID
		}
	}
	return metadata, nil
}

// OpenSession は codex resume でthreadを再開する。
//
// 要件上の優先順位はCodex App Server、Codex SDK、Codex CLIの順だが、
// V1ではCLIのみを実装する。App Server対応を足す際もこのパッケージ内で完結させる。
func (p *Provider) OpenSession(s provider.Session) error {
	bin, err := exec.LookPath("codex")
	if err != nil {
		return fmt.Errorf("codex CLI not found in PATH: %w", err)
	}

	cmd := exec.Command(bin, "resume", s.SessionID)
	if s.Cwd != "" {
		cmd.Dir = s.Cwd
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("resume codex thread %s: %w", s.SessionID, err)
	}
	return nil
}

// newestThread はcwdに一致するthreadのうち最後に更新されたIDを返す。
// 無ければ空文字を返す。
func (p *Provider) newestThread(cwd string) (string, error) {
	type candidate struct {
		id      string
		modTime time.Time
	}
	var candidates []candidate

	err := filepath.WalkDir(p.sessionsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		id := threadIDFromFile(d.Name())
		if id == "" {
			return nil
		}
		meta, err := sessionMetadataFromFile(path)
		if err != nil || !samePath(meta.Cwd, cwd) || (meta.ThreadSource != "" && meta.ThreadSource != "user") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		candidates = append(candidates, candidate{id: id, modTime: info.ModTime()})
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("walk codex sessions directory: %w", err)
	}
	if len(candidates) == 0 {
		return "", nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	return candidates[0].id, nil
}

// threadFile はthread IDに対応するファイルを探す。見つからなければ空文字を返す。
func (p *Provider) threadFile(id string) (string, error) {
	if p.sessionsDir == "" || id == "" {
		return "", nil
	}

	var found string
	err := filepath.WalkDir(p.sessionsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if threadIDFromFile(d.Name()) == id {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("walk codex sessions directory: %w", err)
	}
	return found, nil
}

var threadFilenamePattern = regexp.MustCompile(
	`(?i)([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\.(?:jsonl|json)$`,
)

// threadIDFromFile はrolloutファイル名の末尾からthread IDを取り出す。
func threadIDFromFile(name string) string {
	match := threadFilenamePattern.FindStringSubmatch(name)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

type sessionFileMetadata struct {
	Cwd            string
	ThreadSource   string
	ParentThreadID string
}

// sessionMetadataFromFile はrolloutの先頭にあるsession metadataを返す。
// transcriptの形式変更や破損を検出した場合は、そのファイルを候補にしない。
func sessionMetadataFromFile(path string) (sessionFileMetadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return sessionFileMetadata{}, fmt.Errorf("open codex session metadata: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return sessionFileMetadata{}, fmt.Errorf("read codex session metadata: %w", err)
		}
		return sessionFileMetadata{}, errors.New("codex session metadata is empty")
	}

	var record struct {
		Type    string `json:"type"`
		Payload struct {
			Cwd            string `json:"cwd"`
			ThreadSource   string `json:"thread_source"`
			ParentThreadID string `json:"parent_thread_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
		return sessionFileMetadata{}, fmt.Errorf("decode codex session metadata: %w", err)
	}
	if record.Type != "session_meta" || record.Payload.Cwd == "" {
		return sessionFileMetadata{}, errors.New("codex session metadata does not contain cwd")
	}
	return sessionFileMetadata{
		Cwd:            record.Payload.Cwd,
		ThreadSource:   record.Payload.ThreadSource,
		ParentThreadID: record.Payload.ParentThreadID,
	}, nil
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
