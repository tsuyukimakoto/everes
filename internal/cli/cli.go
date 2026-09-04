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
	"runtime/debug"
	"strings"

	"github.com/tsuyukimakoto/everes/internal/store"
	"github.com/tsuyukimakoto/everes/internal/task"

	// Adapterはinitで自身をregistryへ登録する。
	_ "github.com/tsuyukimakoto/everes/internal/provider/claude"
	_ "github.com/tsuyukimakoto/everes/internal/provider/codex"
)

// Version はリポジトリ内のビルド時に -ldflags で差し替える。
var Version = "dev"

const usage = `everes - Task Context Manager for parallel AI coding work

usage:
  everes <command> [arguments]

commands:
  task create              Create a Task and attach the current session
  task list                List Tasks
  task show <task-id>      Show Task details
  task update <task-id>    Update a Task
  task done <task-id>      Complete a Task

  session current          Show the Task attached to the current session
  session list <task-id>   List Agent Sessions attached to a Task
  session attach <task-id> Attach the current session to an existing Task
  session update <task-id> Update an Agent Session
  session done <task-id>   Complete the current Agent Session
  session detach <task-id> Detach an Agent Session
  session open <task-id>   Resume an Agent Session attached to a Task

  ref add <task-id> <url>        Add a reference
  ref remove <task-id> <ref-id>  Remove a reference

  note add <task-id> <text>       Add a note
  note remove <task-id> <note-id> Remove a note

  current                  Alias for session current
  version                  Show the version

Run everes <command> -h for command-specific help.
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
			"hint: another session updated the Task first; run everes task show to inspect the latest state")
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
		fmt.Println(currentVersion())
		return nil
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
	}
}

func currentVersion() string {
	moduleVersion := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		moduleVersion = info.Main.Version
	}
	return resolveVersion(Version, moduleVersion)
}

func resolveVersion(linkedVersion, moduleVersion string) string {
	if linkedVersion != "" && linkedVersion != "dev" {
		return linkedVersion
	}
	if moduleVersion != "" && moduleVersion != "(devel)" {
		return moduleVersion
	}
	return "dev"
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
