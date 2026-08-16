package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/tsuyukimakoto/everes/internal/task"
)

// SessionIndex は (provider, session_id) からTaskを逆引きするためのインデックス。
//
// Task情報のsource of truthはtasks/<task-id>.json であり、こちらは索引に過ぎない。
type SessionIndex struct {
	SchemaVersion int    `json:"schema_version"`
	Provider      string `json:"provider"`
	SessionID     string `json:"session_id"`
	TaskID        string `json:"task_id"`
	Cwd           string `json:"cwd,omitempty"`
}

// safeIDRe はファイル名として安全なsession IDのみを許可する。
// パストラバーサルを避けるため、英数字とハイフン・アンダースコアに限る。
var safeIDRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func validateSessionKey(provider, sessionID string) error {
	if !safeIDRe.MatchString(provider) {
		return fmt.Errorf("invalid provider %q", provider)
	}
	if !safeIDRe.MatchString(sessionID) {
		return fmt.Errorf("invalid session id %q", sessionID)
	}
	if strings.HasPrefix(provider, ".") || strings.HasPrefix(sessionID, ".") {
		return errors.New("provider and session id must not start with a dot")
	}
	return nil
}

// PutSession はsession indexを保存する。既存があれば上書きする。
func (s *Store) PutSession(idx SessionIndex) error {
	if err := validateSessionKey(idx.Provider, idx.SessionID); err != nil {
		return err
	}
	if !task.ValidULID(idx.TaskID) {
		return fmt.Errorf("invalid task_id %q", idx.TaskID)
	}

	idx.SchemaVersion = task.SchemaVersion
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("encode session index: %w", err)
	}
	data = append(data, '\n')
	return writeAtomic(s.sessionPath(idx.Provider, idx.SessionID), data, 0o600)
}

// LookupSession は (provider, session_id) からsession indexを引く。
// 見つからない場合は task.ErrNotFound を返す。
func (s *Store) LookupSession(provider, sessionID string) (*SessionIndex, error) {
	if err := validateSessionKey(provider, sessionID); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(s.sessionPath(provider, sessionID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("session %s/%s: %w", provider, sessionID, task.ErrNotFound)
		}
		return nil, fmt.Errorf("read session index: %w", err)
	}

	var idx SessionIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("decode session index: %w", err)
	}
	return &idx, nil
}

// RemoveSession はsession indexを削除する。Task本体には影響しない。
func (s *Store) RemoveSession(provider, sessionID string) error {
	if err := validateSessionKey(provider, sessionID); err != nil {
		return err
	}
	if err := os.Remove(s.sessionPath(provider, sessionID)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("session %s/%s: %w", provider, sessionID, task.ErrNotFound)
		}
		return fmt.Errorf("remove session index: %w", err)
	}
	return nil
}

// ListSessions は指定providerのsession indexをすべて返す。
func (s *Store) ListSessions(provider string) ([]SessionIndex, error) {
	entries, err := os.ReadDir(s.sessionsDir(provider))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions directory: %w", err)
	}

	var out []SessionIndex
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		idx, err := s.LookupSession(provider, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *idx)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SessionID < out[j].SessionID })
	return out, nil
}
