// Package maintenance coordinates offline administrative operations with the
// running MemoDrive API process.
package maintenance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrActiveWriter = errors.New("active writer detected")

type WriterLock struct {
	file *os.File
}

func AcquireWriterLock(databasePath string) (*WriterLock, error) {
	lockPath := databasePath + ".writer.lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create writer lock directory: %w", err)
	}
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open writer lock: %w", err)
	}
	if err := tryLock(file); err != nil {
		_ = file.Close()
		if isLockContended(err) {
			return nil, ErrActiveWriter
		}
		return nil, fmt.Errorf("acquire writer lock: %w", err)
	}
	return &WriterLock{file: file}, nil
}

func (l *WriterLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := unlock(l.file)
	closeErr := l.file.Close()
	l.file = nil
	if unlockErr != nil {
		return fmt.Errorf("release writer lock: %w", unlockErr)
	}
	return closeErr
}
