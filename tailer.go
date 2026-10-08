package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Tailer follows matching log files and emits new lines to out, reopening on rotation.
type Tailer struct {
	pattern string
	mode    string
	out     chan<- string
}

type fileTailer struct {
	path       string
	out        chan<- string
	file       *os.File
	sc         *bufio.Scanner
	ino        uint64
	startAtEnd bool
}

type tailWorker struct {
	cancel context.CancelFunc
	id     uint64
}

type tailResult struct {
	path string
	id   uint64
	err  error
}

type logFileMatch struct {
	path    string
	modTime time.Time
}

// NewTailer creates a Tailer for pattern that writes lines to out.
func NewTailer(pattern, mode string, out chan<- string) *Tailer {
	return &Tailer{pattern: pattern, mode: mode, out: out}
}

// Run reads new lines from matching log files until ctx is cancelled.
func (t *Tailer) Run(ctx context.Context) error {
	workers := make(map[string]tailWorker)
	results := make(chan tailResult)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	defer func() {
		for _, worker := range workers {
			worker.cancel()
		}
	}()

	var nextID uint64
	initial := true
	waiting := false
	reconcile := func() error {
		matches, err := t.matches()
		if err != nil {
			return err
		}
		desired := make(map[string]bool, len(matches))
		for _, match := range matches {
			desired[match.path] = true
		}

		if len(desired) == 0 {
			if !waiting {
				slog.Info("no log files match; waiting", "pattern", t.pattern)
				waiting = true
			}
		} else {
			waiting = false
		}

		for path, worker := range workers {
			if !desired[path] {
				worker.cancel()
				delete(workers, path)
				slog.Info("stopped following log file", "path", path)
			}
		}
		for path := range desired {
			if _, ok := workers[path]; ok {
				continue
			}
			nextID++
			workerCtx, cancel := context.WithCancel(ctx)
			worker := tailWorker{cancel: cancel, id: nextID}
			workers[path] = worker
			startAtEnd := initial
			slog.Info("starting log file tailer", "path", path, "skip_existing", startAtEnd)
			go func(path string, id uint64) {
				err := (&fileTailer{path: path, out: t.out, startAtEnd: startAtEnd}).run(workerCtx)
				select {
				case results <- tailResult{path: path, id: id, err: err}:
				case <-ctx.Done():
				}
			}(path, worker.id)
		}
		initial = false
		return nil
	}

	if err := reconcile(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case result := <-results:
			worker, ok := workers[result.path]
			if !ok || worker.id != result.id {
				continue
			}
			worker.cancel()
			delete(workers, result.path)
			if result.err != nil {
				slog.Warn("log file tailer stopped; retrying", "path", result.path, "error", result.err)
			}
		case <-ticker.C:
			if err := reconcile(); err != nil {
				return err
			}
		}
	}
}

func (t *Tailer) matches() ([]logFileMatch, error) {
	paths, err := filepath.Glob(t.pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid log file pattern: %w", err)
	}
	matches := make([]logFileMatch, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		matches = append(matches, logFileMatch{path: path, modTime: info.ModTime()})
	}
	if t.mode != "latest" || len(matches) < 2 {
		return matches, nil
	}
	latest := matches[0]
	for _, match := range matches[1:] {
		if match.modTime.After(latest.modTime) || match.modTime.Equal(latest.modTime) && match.path > latest.path {
			latest = match
		}
	}
	return []logFileMatch{latest}, nil
}

// open opens the file, stores its inode, and chooses the initial offset.
func (t *fileTailer) open() error {
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

	var offset int64
	if t.startAtEnd {
		offset, err = f.Seek(0, io.SeekEnd)
		if err != nil {
			f.Close()
			return err
		}
	}

	slog.Info("log file opened", "path", t.path, "start_offset", offset)
	t.file = f
	t.sc = bufio.NewScanner(f)
	t.sc.Buffer(make([]byte, 4096), 1024*1024)
	t.startAtEnd = false
	return nil
}

func (t *fileTailer) run(ctx context.Context) error {
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
				slog.Warn("line too long, skipping", "path", t.path)
				t.sc = bufio.NewScanner(t.file)
				t.sc.Buffer(make([]byte, 4096), 1024*1024)
				continue
			}
			return err
		}

		t.sc = bufio.NewScanner(t.file)
		t.sc.Buffer(make([]byte, 4096), 1024*1024)
	}
}

// checkRotate reopens the file when it has been truncated or replaced (inode changed).
func (t *fileTailer) checkRotate() error {
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
