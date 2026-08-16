package task

import "fmt"

// Status はTaskの状態。自由入力を避けenumとして扱う。
type Status string

const (
	// StatusActive は現在作業を進めている状態。
	StatusActive Status = "active"
	// StatusWaiting は外部要因待ちの状態。ResumeWhenを原則として記録する。
	StatusWaiting Status = "waiting"
	// StatusNeedsMe はAI側では進められず人間の判断や操作が必要な状態。
	StatusNeedsMe Status = "needs_me"
	// StatusPaused は意図的に作業を止めている状態。
	StatusPaused Status = "paused"
	// StatusDone は目的達成済みの状態。
	StatusDone Status = "done"
)

// AllStatuses は一覧表示や検証で使う全statusを宣言順に返す。
// 並び順は管理UIでの表示優先度（人間の注意を要する順）に合わせている。
func AllStatuses() []Status {
	return []Status{StatusNeedsMe, StatusWaiting, StatusActive, StatusPaused, StatusDone}
}

// ParseStatus は文字列をStatusへ変換する。
func ParseStatus(s string) (Status, error) {
	for _, v := range AllStatuses() {
		if string(v) == s {
			return v, nil
		}
	}
	return "", fmt.Errorf("unknown status %q (valid: needs_me, waiting, active, paused, done)", s)
}

// Valid はStatusが既知の値かを返す。
func (s Status) Valid() bool {
	_, err := ParseStatus(string(s))
	return err == nil
}

// Label は管理UI向けの見出し表記を返す。
func (s Status) Label() string {
	switch s {
	case StatusNeedsMe:
		return "NEEDS ME"
	case StatusWaiting:
		return "WAITING"
	case StatusActive:
		return "ACTIVE"
	case StatusPaused:
		return "PAUSED"
	case StatusDone:
		return "DONE"
	default:
		return string(s)
	}
}
