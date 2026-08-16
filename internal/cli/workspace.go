package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tsuyukimakoto/everes/internal/task"
)

// detectWorkspace はcwdからgit情報を収集する。
//
// gitが無い、またはリポジトリ外の場合も失敗にはしない。Taskの意味はrepositoryと
// 独立しており、workspaceは付随情報に過ぎないためである。
func detectWorkspace(cwd string) task.Workspace {
	ws := task.Workspace{Cwd: cwd}
	if cwd == "" {
		return ws
	}

	root := gitOutput(cwd, "rev-parse", "--show-toplevel")
	if root == "" {
		return ws
	}
	ws.RepoRoot = root
	ws.GitRemote = gitOutput(cwd, "remote", "get-url", "origin")

	if branch := gitOutput(cwd, "rev-parse", "--abbrev-ref", "HEAD"); branch != "HEAD" {
		ws.Branch = branch
	}

	// git-common-dirがrepo直下の.gitと異なる場合、このcheckoutはworktreeである。
	commonDir := gitOutput(cwd, "rev-parse", "--git-common-dir")
	if commonDir != "" && !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(root, commonDir)
	}
	if commonDir != "" && filepath.Clean(commonDir) != filepath.Join(root, ".git") {
		ws.Worktree = root
	}

	return ws
}

// gitOutput はgitコマンドの標準出力を1行で返す。失敗時は空文字を返す。
func gitOutput(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// currentDir はcwdを返す。取得できない場合は空文字を返す。
func currentDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}
