package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingWriterAppendsBelowCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")

	writer, err := newRotatingWriter(path, 1024)
	if err != nil {
		t.Fatalf("newRotatingWriter: %v", err)
	}
	defer writer.Close()

	for range 3 {
		if _, err := writer.Write([]byte("a line\n")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got := strings.Count(string(content), "a line"); got != 3 {
		t.Errorf("found %d lines, want 3", got)
	}

	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Error("nothing should have rotated below the cap")
	}
}

func TestRotatingWriterRotatesAtCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")

	// A 20-byte cap with 10-byte writes rotates on the third write.
	writer, err := newRotatingWriter(path, 20)
	if err != nil {
		t.Fatalf("newRotatingWriter: %v", err)
	}
	defer writer.Close()

	for _, line := range []string{"aaaaaaaaa\n", "bbbbbbbbb\n", "ccccccccc\n"} {
		if _, err := writer.Write([]byte(line)); err != nil {
			t.Fatalf("Write(%q): %v", line, err)
		}
	}

	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile current: %v", err)
	}
	if string(current) != "ccccccccc\n" {
		t.Errorf("current log = %q, want the newest line only", string(current))
	}

	previous, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("ReadFile previous: %v", err)
	}
	if string(previous) != "aaaaaaaaa\nbbbbbbbbb\n" {
		t.Errorf("previous log = %q, want the first two lines", string(previous))
	}
}

// Only one previous generation is kept, so disk use stays bounded.
func TestRotatingWriterKeepsOneGeneration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	writer, err := newRotatingWriter(path, 12)
	if err != nil {
		t.Fatalf("newRotatingWriter: %v", err)
	}
	defer writer.Close()

	for _, line := range []string{"first\n", "second\n", "third\n", "fourth\n", "fifth\n"} {
		if _, err := writer.Write([]byte(line)); err != nil {
			t.Fatalf("Write(%q): %v", line, err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 2 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("expected 2 log files, got %d: %v", len(entries), names)
	}
}

// A restart must continue the existing file rather than rotate immediately.
func TestRotatingWriterResumesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")

	if err := os.WriteFile(path, []byte("earlier run\n"), 0o644); err != nil {
		t.Fatalf("seeding log: %v", err)
	}

	writer, err := newRotatingWriter(path, 1024)
	if err != nil {
		t.Fatalf("newRotatingWriter: %v", err)
	}
	defer writer.Close()

	if _, err := writer.Write([]byte("this run\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "earlier run\nthis run\n" {
		t.Errorf("log = %q, want both lines", string(content))
	}
}

// An oversized log from before rotation existed must be moved aside on the
// first write, not appended to forever.
func TestRotatingWriterRotatesAnAlreadyOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")

	if err := os.WriteFile(path, []byte(strings.Repeat("x", 500)), 0o644); err != nil {
		t.Fatalf("seeding oversized log: %v", err)
	}

	writer, err := newRotatingWriter(path, 100)
	if err != nil {
		t.Fatalf("newRotatingWriter: %v", err)
	}
	defer writer.Close()

	if _, err := writer.Write([]byte("fresh\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(content) != "fresh\n" {
		t.Errorf("current log = %q, want only the new line", string(content))
	}
}

func TestRotatingWriterRejectsBadCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.log")

	if _, err := newRotatingWriter(path, 0); err == nil {
		t.Error("expected an error for a zero size cap")
	}
}
