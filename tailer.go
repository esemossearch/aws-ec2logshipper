package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"syscall"
	"time"
)

// Tailer follows a log file and emits new lines to out, reopening on rotation.
type Tailer struct {
	path string
	out  chan<- string
	file *os.File
	sc   *bufio.Scanner
	ino  uint64
}

// NewTailer creates a Tailer for path that writes lines to out.
func NewTailer(path string, out chan<- string) *Tailer {
	return &Tailer{path: path, out: out}
}

// open opens the file, stores its inode, and seeks to the end.
func (t *Tailer) open() error {
	f, err := os.Open(t.path)
	if err != nil {
		return err
	}

	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}

	s, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		f.Close()
		return fmt.Errorf("not a syscall.Stat_t")
	}
	t.ino = s.Ino

	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		f.Close()
		return err
	}

	t.file = f
	t.sc = bufio.NewScanner(f)
	t.sc.Buffer(make([]byte, 4096), 1024*1024)
	return nil
}

// Run reads new lines from the log file until ctx is cancelled.
func (t *Tailer) Run(ctx context.Context) error {
	if err := t.open(); err != nil {
		return err
	}
	defer t.file.Close()

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := t.checkRotate(); err != nil {
				return err
			}
		}

		for t.sc.Scan() {
			select {
			case <-ctx.Done():
				return nil
			case t.out <- t.sc.Text():
			}
		}

		err := t.sc.Err()
		if err != nil {
			if errors.Is(err, bufio.ErrTooLong) {
				slog.Warn("line too long, skipping")
				t.sc = bufio.NewScanner(t.file)
				t.sc.Buffer(make([]byte, 4096), 1024*1024)
				continue
			}
			return err
		}
	}
}

// checkRotate reopens the file when it has been truncated or replaced (inode changed).
func (t *Tailer) checkRotate() error {
	fi, err := os.Stat(t.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	s, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("not a syscall.Stat_t")
	}

	pos, err := t.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	if s.Ino != t.ino || fi.Size() < pos {
		t.file.Close()
		return t.open()
	}

	return nil
}
