package store

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// writeAtomic はファイルを原子的に書き換える。
//
// 手順は tmpへ書く → fsync → rename。ClaudeとCodexから同時に更新されうるため、
// 途中で落ちても中途半端なJSONが残らないようにする。
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()

	// 以降どこで失敗してもtmpを残さない。
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("fsync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpName, path, err)
	}

	// renameそのものを永続化するためディレクトリもfsyncする。
	return syncDir(dir)
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open directory %s: %w", dir, err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("fsync directory %s: %w", dir, err)
	}
	return nil
}

// withLock はlock fileを取得して fn を実行する。
//
// flockを使うのでプロセスが異常終了してもロックは自動解放される。
// PIDを書き込む方式と違いstale lockの掃除が要らない。
func (s *Store) withLock(id string, fn func() error) error {
	if err := os.MkdirAll(s.locksDir(), 0o700); err != nil {
		return fmt.Errorf("create locks directory: %w", err)
	}

	path := s.lockPath(id)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open lock file %s: %w", path, err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("acquire lock %s: %w", path, err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	return fn()
}
