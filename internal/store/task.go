package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tsuyukimakoto/everes/internal/migration"
	"github.com/tsuyukimakoto/everes/internal/task"
)

// ErrRevisionConflict は期待したrevisionと保存済みrevisionが食い違ったことを示す。
var ErrRevisionConflict = errors.New("revision conflict")

// Create はTaskを新規保存する。既に同一IDが存在する場合はエラーを返す。
func (s *Store) Create(t *task.Task) error {
	if err := t.Validate(); err != nil {
		return fmt.Errorf("validate task: %w", err)
	}
	if err := s.Init(); err != nil {
		return err
	}

	return s.withLock(t.TaskID, func() error {
		if _, err := os.Stat(s.taskPath(t.TaskID)); err == nil {
			return fmt.Errorf("task %s already exists", t.TaskID)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat task file: %w", err)
		}
		return s.writeTask(t)
	})
}

// Get はTaskを読み込む。存在しない場合は task.ErrNotFound を返す。
func (s *Store) Get(id string) (*task.Task, error) {
	data, err := os.ReadFile(s.taskPath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s: %w", id, task.ErrNotFound)
		}
		return nil, fmt.Errorf("read task %s: %w", id, err)
	}
	return decodeTask(data)
}

// List は保存済みのTaskをすべて読み込む。
//
// 並び順はstatusの優先度（needs_me → waiting → active → paused → done）、
// 同一status内はupdated_atの新しい順とする。管理UIの一覧性を優先している。
func (s *Store) List() ([]*task.Task, error) {
	entries, err := os.ReadDir(s.tasksDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read tasks directory: %w", err)
	}

	var tasks []*task.Task
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		t, err := s.Get(id)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}

	order := statusOrder()
	now := time.Now()
	sort.SliceStable(tasks, func(i, j int) bool {
		oi, oj := order[tasks[i].Status], order[tasks[j].Status]
		if oi != oj {
			return oi < oj
		}
		if tasks[i].Status == task.StatusWaiting {
			dueI, dueJ := tasks[i].CheckDue(now), tasks[j].CheckDue(now)
			if dueI != dueJ {
				return dueI
			}
		}
		return tasks[i].Timestamps.UpdatedAt.After(tasks[j].Timestamps.UpdatedAt)
	})
	return tasks, nil
}

// Update はTaskを読み込み、mutateを適用して保存する。
//
// lockを取ってからread-modify-writeするため、同一ホスト上の同時更新は直列化される。
// expectedRevisionが正の値なら、保存済みrevisionと一致しない限り書き込みを拒否する。
// GUIが表示した内容を元に更新をかける場合など、lockの外で状態が進んだ場合に効く。
func (s *Store) Update(id string, expectedRevision int, mutate func(*task.Task) error) (*task.Task, error) {
	var updated *task.Task

	err := s.withLock(id, func() error {
		t, err := s.Get(id)
		if err != nil {
			return err
		}
		if expectedRevision > 0 && t.Revision != expectedRevision {
			return fmt.Errorf("%w: expected revision %d but stored revision is %d",
				ErrRevisionConflict, expectedRevision, t.Revision)
		}
		if err := mutate(t); err != nil {
			return err
		}
		t.Touch()
		if err := t.Validate(); err != nil {
			return fmt.Errorf("validate task: %w", err)
		}
		if err := s.writeTask(t); err != nil {
			return err
		}
		updated = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// Exists はTaskが存在するかを返す。
func (s *Store) Exists(id string) (bool, error) {
	_, err := os.Stat(s.taskPath(id))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat task %s: %w", id, err)
}

// ResolveID は短縮IDや前方一致からTask IDを解決する。
//
// UIでは短縮表示を許すが、内部では常に完全なIDを使う。その橋渡しを担う。
func (s *Store) ResolveID(prefix string) (string, error) {
	if prefix == "" {
		return "", errors.New("task id is empty")
	}
	if ok, err := s.Exists(prefix); err != nil {
		return "", err
	} else if ok {
		return prefix, nil
	}

	entries, err := os.ReadDir(s.tasksDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%s: %w", prefix, task.ErrNotFound)
		}
		return "", fmt.Errorf("read tasks directory: %w", err)
	}

	upper := strings.ToUpper(prefix)
	var matched []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".json")
		if strings.HasPrefix(id, upper) {
			matched = append(matched, id)
		}
	}

	switch len(matched) {
	case 0:
		return "", fmt.Errorf("%s: %w", prefix, task.ErrNotFound)
	case 1:
		return matched[0], nil
	default:
		sort.Strings(matched)
		return "", fmt.Errorf("task id %q is ambiguous: %s", prefix, strings.Join(matched, ", "))
	}
}

func (s *Store) writeTask(t *task.Task) error {
	t.SchemaVersion = task.SchemaVersion
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("encode task %s: %w", t.TaskID, err)
	}
	data = append(data, '\n')
	return writeAtomic(s.taskPath(t.TaskID), data, 0o600)
}

// decodeTask はschema migrationを挟んでTaskへ変換する。
func decodeTask(data []byte) (*task.Task, error) {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decode task json: %w", err)
	}

	migrated, err := migration.Migrate(doc)
	if err != nil {
		return nil, err
	}

	normalized, err := json.Marshal(migrated)
	if err != nil {
		return nil, fmt.Errorf("encode migrated task: %w", err)
	}

	var t task.Task
	if err := json.Unmarshal(normalized, &t); err != nil {
		return nil, fmt.Errorf("decode migrated task: %w", err)
	}
	return &t, nil
}

func statusOrder() map[task.Status]int {
	order := make(map[task.Status]int)
	for i, s := range task.AllStatuses() {
		order[s] = i
	}
	return order
}

// TaskFiles はtasksディレクトリ内のファイルパスを返す。診断用。
func (s *Store) TaskFiles() ([]string, error) {
	entries, err := os.ReadDir(s.tasksDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read tasks directory: %w", err)
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			paths = append(paths, filepath.Join(s.tasksDir(), e.Name()))
		}
	}
	return paths, nil
}
