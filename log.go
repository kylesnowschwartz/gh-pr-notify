package main

import (
	"fmt"
	"os"
	"sync"
)

// defaultLogMaxBytes caps a single log file. One previous generation is kept
// alongside it, so the log costs at most twice this on disk.
const defaultLogMaxBytes = 4 << 20 // 4 MiB

// rotatingWriter appends to a log file and starts a fresh one once it outgrows
// maxBytes, keeping a single previous generation at path + ".1".
//
// launchd's StandardOutPath cannot rotate: it holds the file open for the life
// of the process, so nothing outside can reclaim the space. Owning the file here
// is what makes rotation possible.
type rotatingWriter struct {
	path     string
	maxBytes int64

	mu   sync.Mutex
	file *os.File
	size int64
}

func newRotatingWriter(path string, maxBytes int64) (*rotatingWriter, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("log size cap must be positive, got %d", maxBytes)
	}

	w := &rotatingWriter{path: path, maxBytes: maxBytes}
	if err := w.open(); err != nil {
		return nil, err
	}

	return w, nil
}

func (w *rotatingWriter) open() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("opening log file %s: %w", w.path, err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return fmt.Errorf("measuring log file %s: %w", w.path, err)
	}

	w.file = file
	w.size = info.Size()

	return nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.size > 0 && w.size+int64(len(p)) > w.maxBytes {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	n, err := w.file.Write(p)
	w.size += int64(n)

	return n, err
}

// rotate closes the current file, moves it aside, and opens a fresh one. The
// previous generation is replaced, so two files exist at most.
func (w *rotatingWriter) rotate() error {
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("closing log file before rotating: %w", err)
	}

	if err := os.Rename(w.path, w.path+".1"); err != nil && !os.IsNotExist(err) {
		// Reopen so logging survives a failed rotation rather than going dark.
		if openErr := w.open(); openErr != nil {
			return fmt.Errorf("rotating log file: %w (reopen also failed: %v)", err, openErr)
		}
		return fmt.Errorf("rotating log file: %w", err)
	}

	return w.open()
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.file.Close()
}
