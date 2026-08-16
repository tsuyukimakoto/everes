// Package cli はeveresコマンドのエントリーポイントを提供する。
//
// SkillはこのCLIだけを呼ぶ。JSONファイルをAgent自身に直接編集させないことで、
// validation・locking・session mappingをEveres側に集約する。
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tsuyukimakoto/everes/internal/store"
	"github.com/tsuyukimakoto/everes/internal/task"

	// Adapterはinitで自身をregistryへ登録する。
	_ "github.com/tsuyukimakoto/everes/internal/provider/claude"
	_ "github.com/tsuyukimakoto/everes/internal/provider/codex"
)

// Version はビルド時に -ldflags で差し替える。
var Version = "dev"

const usage = `everes - 並行するAIコーディング作業のTask Context Manager

usage:
  everes <command> [arguments]

commands:
  task create              Taskを作成し、現在のsessionへ紐付ける
  task list                Taskを一覧する
  task show <task-id>      Taskの詳細を表示する
  task update <task-id>    Taskの状態を更新する
  task done <task-id>      Taskを完了にする

  session current          現在のsessionに紐づくTaskを表示する
  session list <task-id>   Taskに紐づくAgent sessionを確認する
  session attach <task-id> 現在のsessionを既存Taskへ紐付ける
  session update <task-id> Agent sessionの状態を更新する
  session done <task-id>   現在のAgent sessionを完了にする
  session detach <task-id> Agent sessionの紐付けを外す
  session open <task-id>   Taskに紐づくAgent sessionを再開する

  ref add <task-id> <url>       参照を追加する
  ref remove <task-id> <ref-id> 参照を削除する

  note add <task-id> <text>      メモを追加する
  note remove <task-id> <note-id> メモを削除する

  current                  session current の alias
  version                  バージョンを表示する

各コマンドの詳細は everes <command> -h を参照。
`

// Run はコマンドを実行し、プロセスの終了コードを返す。
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	err := dispatch(args)
	if err == nil {
		return 0
	}
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}

	fmt.Fprintf(os.Stderr, "everes: %v\n", err)
	if errors.Is(err, store.ErrRevisionConflict) {
		fmt.Fprintln(os.Stderr,
			"hint: 他のsessionが先に更新しています。everes task show で最新を確認してください。")
	}
	return 1
}

func dispatch(args []string) error {
	cmd, rest := args[0], args[1:]

	switch cmd {
	case "task":
		return runTask(rest)
	case "session":
		return runSession(rest)
	case "ref":
		return runRef(rest)
	case "note":
		return runNote(rest)
	case "hook":
		return runHook(rest)
	case "current":
		// 利用頻度が高いためaliasを設けている。
		return runSessionCurrent(rest)
	case "version", "--version", "-v":
		fmt.Println(Version)
		return nil
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
	}
}

// openStore はデータディレクトリを開く。
func openStore() (*store.Store, error) {
	s, err := store.Default()
	if err != nil {
		return nil, err
	}
	if err := s.Init(); err != nil {
		return nil, err
	}
	return s, nil
}

// resolveTask は短縮IDを解決してTaskを読み込む。
func resolveTask(s *store.Store, ref string) (*task.Task, error) {
	id, err := s.ResolveID(ref)
	if err != nil {
		return nil, err
	}
	return s.Get(id)
}

// newFlagSet は共通の体裁を持つFlagSetを返す。
func newFlagSet(name string, out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(out)
	return fs
}

// requireArg はサブコマンドの必須位置引数を取り出す。
func requireArg(fs *flag.FlagSet, index int, name string) (string, error) {
	if fs.NArg() <= index {
		return "", fmt.Errorf("%s is required", name)
	}
	return fs.Arg(index), nil
}

// parseFlags はフラグを位置引数より後ろに書いても解釈できるようにParseする。
//
// 標準のflagは最初の非フラグ引数でパースを止めるため、
// everes ref add <task-id> <url> --title "..." のような自然な語順が通らない。
// Skillが生成するコマンドで語順に気を遣わせたくないので、事前に並べ替える。
func parseFlags(fs *flag.FlagSet, args []string) error {
	return fs.Parse(reorderArgs(fs, args))
}

// reorderArgs はフラグ（とその値）を前へ、位置引数を後ろへ寄せる。
// "--" 以降はすべて位置引数として扱う。
func reorderArgs(fs *flag.FlagSet, args []string) []string {
	var flags, positional []string

	for i := 0; i < len(args); i++ {
		a := args[i]

		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) < 2 || !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			continue
		}

		flags = append(flags, a)
		if strings.Contains(a, "=") {
			continue
		}
		if takesValue(fs, a) && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}

	return append(flags, positional...)
}

// takesValue はそのフラグが後続の値を必要とするかを返す。boolフラグは取らない。
func takesValue(fs *flag.FlagSet, arg string) bool {
	f := fs.Lookup(strings.TrimLeft(arg, "-"))
	if f == nil {
		return false
	}
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return !(ok && bf.IsBoolFlag())
}
