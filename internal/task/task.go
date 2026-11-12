// Package task はEveresの中心となるTaskモデルを提供する。
//
// Taskはconversationの寿命から独立して存在する。ClaudeのsessionやCodexのthreadは
// Taskを実行するためのAgent Sessionとして扱い、Taskの主キーにはしない。
package task

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SchemaVersion はWriterが書き出す最新のschema version。
const SchemaVersion = 2

// ErrNotFound はTaskが存在しないことを示す。
var ErrNotFound = errors.New("task not found")

// Task はEveresが管理する作業の単位。
type Task struct {
	SchemaVersion int    `json:"schema_version"`
	TaskID        string `json:"task_id"`
	Revision      int    `json:"revision"`

	Title       string     `json:"title"`
	Objective   string     `json:"objective"`
	Status      Status     `json:"status"`
	Summary     string     `json:"summary"`
	NextAction  string     `json:"next_action"`
	ResumeWhen  string     `json:"resume_when"`
	CheckAt     *time.Time `json:"check_at,omitempty"`
	CheckAction string     `json:"check_action,omitempty"`

	Workspace     Workspace      `json:"workspace"`
	References    []Reference    `json:"references"`
	Notes         []Note         `json:"notes"`
	AgentSessions []AgentSession `json:"agent_sessions"`

	Timestamps Timestamps `json:"timestamps"`
}

// Workspace はTaskを実行する場所。Taskの意味自体はrepositoryと独立している。
type Workspace struct {
	Cwd       string `json:"cwd"`
	RepoRoot  string `json:"repo_root"`
	GitRemote string `json:"git_remote"`
	Branch    string `json:"branch"`
	Worktree  string `json:"worktree"`
}

// Reference はGitHub IssueやGoogle Docs等への参照。
// V1ではURLとTitleのみ使用し、Metadataは将来のAPI integration用に空で確保する。
type Reference struct {
	ID       string            `json:"id"`
	Type     RefType           `json:"type"`
	Title    string            `json:"title"`
	URL      string            `json:"url"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Note は人間向けメモ。
type Note struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// AgentSession はTaskを実行するAgentのsession。1つのTaskに複数存在してよい。
type AgentSession struct {
	Provider    string     `json:"provider"`
	SessionID   string     `json:"session_id"`
	Cwd         string     `json:"cwd,omitempty"`
	Label       string     `json:"label,omitempty"`
	Status      Status     `json:"status,omitempty"`
	Summary     string     `json:"summary,omitempty"`
	NextAction  string     `json:"next_action,omitempty"`
	ResumeWhen  string     `json:"resume_when,omitempty"`
	CheckAt     *time.Time `json:"check_at,omitempty"`
	CheckAction string     `json:"check_action,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	LastSeenAt  time.Time  `json:"last_seen_at"`
}

// Timestamps はTaskの時刻情報。
type Timestamps struct {
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

// Now は保存に使う現在時刻を返す。JSONの可読性を優先し秒精度に丸める。
func Now() time.Time {
	return time.Now().Truncate(time.Second)
}

// New はTaskを新規作成する。
func New(title string) (*Task, error) {
	if strings.TrimSpace(title) == "" {
		return nil, errors.New("title is empty")
	}
	id, err := NewULID()
	if err != nil {
		return nil, fmt.Errorf("generate task id: %w", err)
	}
	now := Now()
	return &Task{
		SchemaVersion: SchemaVersion,
		TaskID:        id,
		Revision:      1,
		Title:         title,
		Status:        StatusActive,
		References:    []Reference{},
		Notes:         []Note{},
		AgentSessions: []AgentSession{},
		Timestamps: Timestamps{
			CreatedAt: now,
			UpdatedAt: now,
		},
	}, nil
}

// Validate はTaskが保存可能な状態かを検証する。
func (t *Task) Validate() error {
	if t.SchemaVersion <= 0 {
		return errors.New("schema_version is not set")
	}
	if !ValidULID(t.TaskID) {
		return fmt.Errorf("invalid task_id %q", t.TaskID)
	}
	if t.Revision <= 0 {
		return errors.New("revision must be positive")
	}
	if strings.TrimSpace(t.Title) == "" {
		return errors.New("title is empty")
	}
	if !t.Status.Valid() {
		return fmt.Errorf("invalid status %q", t.Status)
	}
	if t.CheckAt != nil {
		if t.Status != StatusWaiting {
			return errors.New("check_at is only valid for waiting tasks")
		}
		if strings.TrimSpace(t.CheckAction) == "" {
			return errors.New("check_action is required when check_at is set")
		}
	} else if strings.TrimSpace(t.CheckAction) != "" {
		return errors.New("check_at is required when check_action is set")
	}
	for _, r := range t.References {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("reference %s: %w", r.ID, err)
		}
	}
	for _, s := range t.AgentSessions {
		if s.Provider == "" || s.SessionID == "" {
			return errors.New("agent session requires provider and session_id")
		}
		if err := s.Validate(); err != nil {
			return fmt.Errorf("agent session %s/%s: %w", s.Provider, s.SessionID, err)
		}
	}
	return nil
}

// Touch はUpdatedAtを進め、revisionを1つ上げる。
func (t *Task) Touch() {
	t.Revision++
	t.Timestamps.UpdatedAt = Now()
}

// SetStatus はstatusを変更する。doneへ移す際はCompletedAtを記録し、
// doneから戻す際はCompletedAtを消す。
func (t *Task) SetStatus(s Status) {
	t.Status = s
	if s == StatusDone {
		for i := range t.AgentSessions {
			t.AgentSessions[i].SetStatus(StatusDone)
		}
	}
	if s != StatusWaiting {
		t.CheckAt = nil
		t.CheckAction = ""
	}
	switch {
	case s == StatusDone && t.Timestamps.CompletedAt == nil:
		now := Now()
		t.Timestamps.CompletedAt = &now
	case s != StatusDone:
		t.Timestamps.CompletedAt = nil
	}
}

// CheckDue はwaitingの確認時刻を過ぎているかを返す。
// statusは変更せず、一覧で人間の注意を促すための派生状態として使う。
func (t *Task) CheckDue(now time.Time) bool {
	if t.Status != StatusWaiting {
		return false
	}
	if t.CheckAt != nil && !now.Before(*t.CheckAt) {
		return true
	}
	for i := range t.AgentSessions {
		if t.AgentSessions[i].CheckDue(now) {
			return true
		}
	}
	return false
}

// Validate はAgent Sessionの状態と確認予定の組み合わせを検証する。
// Statusが空のsessionはv1から移行した未確定状態として許容する。
func (s *AgentSession) Validate() error {
	if s.Status != "" && !s.Status.Valid() {
		return fmt.Errorf("invalid status %q", s.Status)
	}
	if s.CheckAt != nil {
		if s.Status != StatusWaiting {
			return errors.New("check_at is only valid for waiting sessions")
		}
		if strings.TrimSpace(s.CheckAction) == "" {
			return errors.New("check_action is required when check_at is set")
		}
	} else if strings.TrimSpace(s.CheckAction) != "" {
		return errors.New("check_at is required when check_action is set")
	}
	return nil
}

// SetStatus はAgent Sessionの状態を変更する。
func (s *AgentSession) SetStatus(status Status) {
	s.Status = status
	if status != StatusWaiting {
		s.CheckAt = nil
		s.CheckAction = ""
	}
	if status == StatusDone {
		if s.CompletedAt == nil {
			now := Now()
			s.CompletedAt = &now
		}
	} else {
		s.CompletedAt = nil
	}
}

// CheckDue はこのsessionの確認時刻を過ぎているかを返す。
func (s *AgentSession) CheckDue(now time.Time) bool {
	return s.Status == StatusWaiting && s.CheckAt != nil && !now.Before(*s.CheckAt)
}

// AddNote はメモを追加し、そのIDを返す。
func (t *Task) AddNote(text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", errors.New("note text is empty")
	}
	id := nextSeqID("note", len(t.Notes), func(i int) string { return t.Notes[i].ID })
	t.Notes = append(t.Notes, Note{ID: id, Text: text, CreatedAt: Now()})
	return id, nil
}

// RemoveNote はIDを指定してメモを削除する。
func (t *Task) RemoveNote(id string) error {
	for i, n := range t.Notes {
		if n.ID == id {
			t.Notes = append(t.Notes[:i], t.Notes[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("note %q not found", id)
}

// AddReference は参照を追加し、そのIDを返す。
func (t *Task) AddReference(url, title string, typ RefType) (string, error) {
	ref := Reference{
		ID:    nextSeqID("ref", len(t.References), func(i int) string { return t.References[i].ID }),
		Type:  typ,
		Title: title,
		URL:   url,
	}
	if ref.Type == "" {
		ref.Type = DetectRefType(url)
	}
	if ref.Title == "" {
		ref.Title = url
	}
	if err := ref.Validate(); err != nil {
		return "", err
	}
	t.References = append(t.References, ref)
	return ref.ID, nil
}

// RemoveReference はIDを指定して参照を削除する。
func (t *Task) RemoveReference(id string) error {
	for i, r := range t.References {
		if r.ID == id {
			t.References = append(t.References[:i], t.References[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("reference %q not found", id)
}

// AttachSession はAgent Sessionを紐付ける。既存の同一sessionがあればLastSeenAtを更新する。
// 旧sessionは削除しない。conversationが長くなり新sessionへ移行してもTaskは変わらないため。
func (t *Task) AttachSession(s AgentSession) {
	now := Now()
	if s.Status == "" {
		s.Status = StatusActive
	}
	if t.Status == StatusDone {
		s.SetStatus(StatusDone)
	}
	for i := range t.AgentSessions {
		if t.AgentSessions[i].Provider == s.Provider && t.AgentSessions[i].SessionID == s.SessionID {
			t.AgentSessions[i].LastSeenAt = now
			if s.Label != "" {
				t.AgentSessions[i].Label = s.Label
			}
			if s.Cwd != "" {
				t.AgentSessions[i].Cwd = s.Cwd
			}
			if t.AgentSessions[i].Status == "" {
				t.AgentSessions[i].Status = s.Status
			}
			t.RecomputeStatus()
			return
		}
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.LastSeenAt = now
	t.AgentSessions = append(t.AgentSessions, s)
	t.RecomputeStatus()
}

// DetachSession は指定したAgent SessionだけをTaskから外す。
// provider側のconversationやsessionファイルは削除しない。
func (t *Task) DetachSession(provider, sessionID string) error {
	for i, s := range t.AgentSessions {
		if s.Provider == provider && s.SessionID == sessionID {
			t.AgentSessions = append(t.AgentSessions[:i], t.AgentSessions[i+1:]...)
			t.RecomputeStatus()
			return nil
		}
	}
	return fmt.Errorf("session %s/%s is not attached", provider, sessionID)
}

// UpdateSession は指定したAgent Sessionを更新し、Taskの集約statusを再計算する。
func (t *Task) UpdateSession(provider, sessionID string, update func(*AgentSession) error) error {
	for i := range t.AgentSessions {
		s := &t.AgentSessions[i]
		if s.Provider != provider || s.SessionID != sessionID {
			continue
		}
		if err := update(s); err != nil {
			return err
		}
		s.LastSeenAt = Now()
		if err := s.Validate(); err != nil {
			return err
		}
		t.RecomputeStatus()
		return nil
	}
	return fmt.Errorf("session %s/%s is not attached", provider, sessionID)
}

// RecomputeStatus は明示statusを持つsessionからTask statusを集約する。
// v1由来でstatusが空のsessionは推測せず集約から除外する。
func (t *Task) RecomputeStatus() {
	has := map[Status]bool{}
	for _, s := range t.AgentSessions {
		if s.Status != "" {
			has[s.Status] = true
		}
	}
	var status Status
	switch {
	case has[StatusActive]:
		status = StatusActive
	case has[StatusNeedsMe]:
		status = StatusNeedsMe
	case has[StatusWaiting]:
		status = StatusWaiting
	case has[StatusPaused]:
		status = StatusPaused
	default:
		return
	}
	t.SetStatus(status)
}

// Complete はTaskと紐づく全sessionを完了にする。
func (t *Task) Complete() {
	t.SetStatus(StatusDone)
}

// LatestSession は最後に使われたAgent Sessionを返す。管理UIのdefaultに使う。
func (t *Task) LatestSession() (AgentSession, bool) {
	if len(t.AgentSessions) == 0 {
		return AgentSession{}, false
	}
	sessions := make([]AgentSession, len(t.AgentSessions))
	copy(sessions, t.AgentSessions)
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].LastSeenAt.After(sessions[j].LastSeenAt)
	})
	return sessions[0], true
}

// nextSeqID は "ref-3" 形式のIDを採番する。既存の最大連番+1を使い、
// 削除後の再採番でIDが衝突しないようにする。
func nextSeqID(prefix string, n int, at func(int) string) string {
	max := 0
	for i := 0; i < n; i++ {
		s := strings.TrimPrefix(at(i), prefix+"-")
		if v, err := strconv.Atoi(s); err == nil && v > max {
			max = v
		}
	}
	return fmt.Sprintf("%s-%d", prefix, max+1)
}
