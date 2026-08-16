// Package store はEveres専用ディレクトリへのTask永続化を担う。
//
// Claude CodeやCodexの内部保存ディレクトリにはファイルを置かない。
// AIツール側の内部形式変更、cleanup、archiveの影響を避けるためである。
package store

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnvHome はEveresのデータディレクトリを上書きする環境変数。テストでも使う。
const EnvHome = "EVERES_HOME"

// Store はEveresのデータディレクトリを扱う。
type Store struct {
	root string
}

// New は指定したrootを使うStoreを返す。
func New(root string) *Store {
	return &Store{root: root}
}

// Default は EVERES_HOME か ~/.everes を使うStoreを返す。
func Default() (*Store, error) {
	if v := os.Getenv(EnvHome); v != "" {
		return New(v), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	return New(filepath.Join(home, ".everes")), nil
}

// Root はデータディレクトリのパスを返す。
func (s *Store) Root() string { return s.root }

func (s *Store) tasksDir() string { return filepath.Join(s.root, "tasks") }
func (s *Store) locksDir() string { return filepath.Join(s.root, "locks") }

func (s *Store) sessionsDir(provider string) string {
	return filepath.Join(s.root, "sessions", provider)
}

func (s *Store) taskPath(id string) string {
	return filepath.Join(s.tasksDir(), id+".json")
}

func (s *Store) sessionPath(provider, sessionID string) string {
	return filepath.Join(s.sessionsDir(provider), sessionID+".json")
}

func (s *Store) lockPath(id string) string {
	return filepath.Join(s.locksDir(), id+".lock")
}

// Init は必要なディレクトリを作成する。既に存在する場合は何もしない。
//
// sessions/ は provider ごとに作られるため、書き込み時に必要な分だけ用意する。
func (s *Store) Init() error {
	dirs := []string{s.tasksDir(), s.locksDir()}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
	}
	return nil
}
