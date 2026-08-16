package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/tsuyukimakoto/everes/internal/task"
)

func runRef(args []string) error {
	if len(args) == 0 {
		return errors.New("ref requires a subcommand (add, remove)")
	}
	switch args[0] {
	case "add":
		return runRefAdd(args[1:])
	case "remove":
		return runRefRemove(args[1:])
	default:
		return fmt.Errorf("unknown ref subcommand %q", args[0])
	}
}

func runRefAdd(args []string) error {
	fs := newFlagSet("ref add", os.Stderr)
	title := fs.String("title", "", "参照のタイトル")
	typeName := fs.String("type", "", "参照種別 (省略時はURLから推測)")
	asJSON := fs.Bool("json", false, "JSONで出力する")

	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ref, err := requireArg(fs, 0, "task-id")
	if err != nil {
		return err
	}
	url, err := requireArg(fs, 1, "url")
	if err != nil {
		return err
	}

	var refType task.RefType
	if *typeName != "" {
		refType, err = task.ParseRefType(*typeName)
		if err != nil {
			return err
		}
	}

	// tokenを含むURLは保存を拒否せず警告する。判断は人間に委ねる。
	for _, w := range task.InspectSecrets(url) {
		fmt.Fprintf(os.Stderr, "warn: %s\n", w)
	}

	s, err := openStore()
	if err != nil {
		return err
	}
	id, err := s.ResolveID(ref)
	if err != nil {
		return err
	}

	var refID string
	updated, err := s.Update(id, 0, func(t *task.Task) error {
		refID, err = t.AddReference(url, *title, refType)
		return err
	})
	if err != nil {
		return err
	}

	if *asJSON {
		return writeJSON(os.Stdout, newTaskSummary(updated))
	}
	fmt.Printf("added %s to %s\n", refID, task.ShortID(updated.TaskID))
	return nil
}

func runRefRemove(args []string) error {
	fs := newFlagSet("ref remove", os.Stderr)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ref, err := requireArg(fs, 0, "task-id")
	if err != nil {
		return err
	}
	refID, err := requireArg(fs, 1, "ref-id")
	if err != nil {
		return err
	}

	s, err := openStore()
	if err != nil {
		return err
	}
	id, err := s.ResolveID(ref)
	if err != nil {
		return err
	}

	updated, err := s.Update(id, 0, func(t *task.Task) error {
		return t.RemoveReference(refID)
	})
	if err != nil {
		return err
	}

	fmt.Printf("removed %s from %s\n", refID, task.ShortID(updated.TaskID))
	return nil
}

func runNote(args []string) error {
	if len(args) == 0 {
		return errors.New("note requires a subcommand (add, remove)")
	}
	switch args[0] {
	case "add":
		return runNoteAdd(args[1:])
	case "remove":
		return runNoteRemove(args[1:])
	default:
		return fmt.Errorf("unknown note subcommand %q", args[0])
	}
}

func runNoteAdd(args []string) error {
	fs := newFlagSet("note add", os.Stderr)
	asJSON := fs.Bool("json", false, "JSONで出力する")

	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ref, err := requireArg(fs, 0, "task-id")
	if err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return errors.New("text is required")
	}
	// 引用符なしで複数語を渡せるようにする。
	text := strings.Join(fs.Args()[1:], " ")

	s, err := openStore()
	if err != nil {
		return err
	}
	id, err := s.ResolveID(ref)
	if err != nil {
		return err
	}

	var noteID string
	updated, err := s.Update(id, 0, func(t *task.Task) error {
		noteID, err = t.AddNote(text)
		return err
	})
	if err != nil {
		return err
	}

	if *asJSON {
		return writeJSON(os.Stdout, newTaskSummary(updated))
	}
	fmt.Printf("added %s to %s\n", noteID, task.ShortID(updated.TaskID))
	return nil
}

func runNoteRemove(args []string) error {
	fs := newFlagSet("note remove", os.Stderr)
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	ref, err := requireArg(fs, 0, "task-id")
	if err != nil {
		return err
	}
	noteID, err := requireArg(fs, 1, "note-id")
	if err != nil {
		return err
	}

	s, err := openStore()
	if err != nil {
		return err
	}
	id, err := s.ResolveID(ref)
	if err != nil {
		return err
	}

	updated, err := s.Update(id, 0, func(t *task.Task) error {
		return t.RemoveNote(noteID)
	})
	if err != nil {
		return err
	}

	fmt.Printf("removed %s from %s\n", noteID, task.ShortID(updated.TaskID))
	return nil
}
