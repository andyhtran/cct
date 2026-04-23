package backup

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

const lockTimeout = 5 * time.Second

type fileLock struct {
	f *os.File
}

func acquireLock(path string) (*fileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		return &fileLock{f: f}, nil
	}

	type result struct{ err error }
	ch := make(chan result, 1)
	go func() {
		ch <- result{syscall.Flock(int(f.Fd()), syscall.LOCK_EX)}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("acquire lock: %w", r.err)
		}
		return &fileLock{f: f}, nil
	case <-time.After(lockTimeout):
		_ = f.Close()
		return nil, fmt.Errorf("timeout waiting for backup lock (another backup may be running)")
	}
}

func (l *fileLock) release() {
	if l.f != nil {
		_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
		_ = l.f.Close()
	}
}
