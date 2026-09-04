package task

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// RefType は参照先の種別。将来拡張できるよう文字列で持つ。
type RefType string

const (
	RefGitHubIssue RefType = "github_issue"
	RefGitHubPR    RefType = "github_pr"
	RefGoogleDoc   RefType = "google_doc"
	RefGoogleSheet RefType = "google_sheet"
	RefGoogleSlide RefType = "google_slide"
	RefNotion      RefType = "notion"
	RefJira        RefType = "jira"
	RefLinear      RefType = "linear"
	RefWeb         RefType = "web"
	RefFile        RefType = "file"
	RefOther       RefType = "other"
)

// AllRefTypes は既知の参照種別を返す。
func AllRefTypes() []RefType {
	return []RefType{
		RefGitHubIssue, RefGitHubPR,
		RefGoogleDoc, RefGoogleSheet, RefGoogleSlide,
		RefNotion, RefJira, RefLinear,
		RefWeb, RefFile, RefOther,
	}
}

// ParseRefType は文字列をRefTypeへ変換する。
func ParseRefType(s string) (RefType, error) {
	for _, v := range AllRefTypes() {
		if string(v) == s {
			return v, nil
		}
	}
	return "", fmt.Errorf("unknown reference type %q", s)
}

var (
	githubIssueRe = regexp.MustCompile(`^/[^/]+/[^/]+/issues/\d+`)
	githubPRRe    = regexp.MustCompile(`^/[^/]+/[^/]+/pull/\d+`)
)

// DetectRefType はURLから参照種別を推測する。判別できないものはRefOtherを返す。
func DetectRefType(raw string) RefType {
	u, err := url.Parse(raw)
	if err != nil {
		return RefOther
	}

	if u.Scheme == "file" || strings.HasPrefix(raw, "/") {
		return RefFile
	}

	host := strings.ToLower(u.Hostname())
	switch {
	case host == "github.com":
		switch {
		case githubIssueRe.MatchString(u.Path):
			return RefGitHubIssue
		case githubPRRe.MatchString(u.Path):
			return RefGitHubPR
		}
		return RefWeb
	case host == "docs.google.com":
		switch {
		case strings.HasPrefix(u.Path, "/document"):
			return RefGoogleDoc
		case strings.HasPrefix(u.Path, "/spreadsheets"):
			return RefGoogleSheet
		case strings.HasPrefix(u.Path, "/presentation"):
			return RefGoogleSlide
		}
		return RefWeb
	case host == "www.notion.so" || strings.HasSuffix(host, ".notion.site"):
		return RefNotion
	case strings.HasSuffix(host, ".atlassian.net"):
		return RefJira
	case host == "linear.app":
		return RefLinear
	case u.Scheme == "http" || u.Scheme == "https":
		return RefWeb
	}
	return RefOther
}

// Validate は参照が保存可能かを検証する。
func (r Reference) Validate() error {
	if strings.TrimSpace(r.URL) == "" {
		return errors.New("url is empty")
	}
	if r.Type == "" {
		return errors.New("type is empty")
	}
	if _, err := ParseRefType(string(r.Type)); err != nil {
		return err
	}
	return nil
}

// secretParamRe はquery parameterのキーが機密情報を含みそうかを判定する。
var secretParamRe = regexp.MustCompile(`(?i)(token|secret|password|passwd|api[-_]?key|access[-_]?key|auth|credential|signature|sig)`)

// InspectSecrets はURLに機密情報が埋まっている疑いがあれば警告文言を返す。
//
// Everesはtokenの類をTask metadataに保存しない方針であり、referenceとして
// 渡されたURLについては保存を拒否せず警告のみ行う。判断は人間に委ねる。
func InspectSecrets(raw string) []string {
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}

	var warnings []string
	for key := range u.Query() {
		if secretParamRe.MatchString(key) {
			warnings = append(warnings, fmt.Sprintf("query parameter %q may contain sensitive information", key))
		}
	}
	if u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			warnings = append(warnings, "the URL contains an embedded password")
		}
	}
	return warnings
}
