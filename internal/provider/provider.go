// Package provider はAgent固有の処理をEveres coreから隔離する。
//
// Everes coreはClaudeやCodexの起動方法・保存形式を知らない。
// 非公開仕様に依存する処理は各Adapterの内部に閉じ込め、壊れた際に
// そのAdapterだけを差し替えられるようにする。
package provider

import (
	"errors"
	"fmt"
	"os"
	"sort"
)

// ErrNoCurrentSession は現在のsessionを特定できなかったことを示す。
var ErrNoCurrentSession = errors.New("current session could not be detected")

// ErrNotSupported はAdapterがその操作に対応していないことを示す。
var ErrNotSupported = errors.New("operation is not supported by this provider")

// Session はAgentのsession/threadを指す。
type Session struct {
	Provider  string
	SessionID string
	Cwd       string
	Label     string
}

// Provider はAgent固有処理のinterface。
type Provider interface {
	// Name はproviderの識別子を返す。
	Name() string
	// DetectCurrentSession は実行中のsessionを推定する。
	DetectCurrentSession(cwd string) (*Session, error)
	// OpenSession は既存のsessionを再開する。
	OpenSession(s Session) error
	// SessionExists はsessionの実体が残っているかを返す。
	SessionExists(s Session) (bool, error)
	// SessionMetadata は表示や診断に使う付随情報を返す。
	SessionMetadata(s Session) (map[string]any, error)
}

// AppLinkProvider はproviderのdesktop appでsessionを開くdeep linkを提供する。
// desktop appへの導線がないproviderは実装しない。
type AppLinkProvider interface {
	AppURL(s Session) (string, error)
}

// EnvProvider と EnvSessionID はsession検出を明示的に上書きする環境変数。
//
// hookがcontextへ出力したsession IDをAIが引き回す経路が主となるが、
// 手動確認やテストのために環境変数でも指定できるようにしている。
const (
	EnvProvider  = "EVERES_PROVIDER"
	EnvSessionID = "EVERES_SESSION_ID"
)

// registry は利用可能なProviderを名前で保持する。
var registry = map[string]Provider{}

// Register はProviderを登録する。各Adapterのinitから呼ぶ。
func Register(p Provider) {
	registry[p.Name()] = p
}

// Get は名前からProviderを取得する。
func Get(name string) (Provider, error) {
	p, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q (available: %v)", name, Names())
	}
	return p, nil
}

// SessionAppURL はproviderが公開するdesktop app用deep linkを返す。
func SessionAppURL(s Session) (string, error) {
	p, err := Get(s.Provider)
	if err != nil {
		return "", err
	}
	linker, ok := p.(AppLinkProvider)
	if !ok {
		return "", fmt.Errorf("%w: %s does not provide a desktop app link", ErrNotSupported, s.Provider)
	}
	return linker.AppURL(s)
}

// Names は登録済みProvider名を返す。
func Names() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// All は登録済みProviderを名前順で返す。
func All() []Provider {
	ps := make([]Provider, 0, len(registry))
	for _, n := range Names() {
		ps = append(ps, registry[n])
	}
	return ps
}

// SessionFromEnv は環境変数による明示指定を読む。指定がなければ nil を返す。
func SessionFromEnv() *Session {
	p, id := os.Getenv(EnvProvider), os.Getenv(EnvSessionID)
	if p == "" || id == "" {
		return nil
	}
	cwd, _ := os.Getwd()
	return &Session{Provider: p, SessionID: id, Cwd: cwd}
}

// DetectCurrentSession は環境変数、次いで各Adapterの推定でsessionを特定する。
//
// 複数Adapterが候補を返した場合は特定できないものとして扱う。誤ったTaskへ
// 紐付けるくらいなら検出失敗を返すほうが安全である。
func DetectCurrentSession(cwd string) (*Session, error) {
	if s := SessionFromEnv(); s != nil {
		return s, nil
	}

	var found []*Session
	for _, p := range All() {
		s, err := p.DetectCurrentSession(cwd)
		if err != nil {
			if errors.Is(err, ErrNoCurrentSession) {
				continue
			}
			return nil, err
		}
		if s != nil {
			found = append(found, s)
		}
	}

	switch len(found) {
	case 0:
		return nil, ErrNoCurrentSession
	case 1:
		return found[0], nil
	default:
		names := make([]string, 0, len(found))
		for _, s := range found {
			names = append(names, s.Provider)
		}
		return nil, fmt.Errorf("%w: multiple providers matched (%v); specify --provider and --session-id",
			ErrNoCurrentSession, names)
	}
}
