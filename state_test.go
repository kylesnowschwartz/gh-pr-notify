package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStateMissingFile(t *testing.T) {
	state, err := loadState("/tmp/gh-pr-notify-test-nonexistent.json")
	if err != nil {
		t.Fatalf("loadState on missing file: %v", err)
	}
	if len(state) != 0 {
		t.Fatalf("expected empty state, got %d entries", len(state))
	}
}

func TestSaveAndLoadState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	want := map[string]string{
		"envato/repo#123": "REVIEW_REQUIRED",
		"envato/repo#456": "APPROVED",
		"other/thing#7":   "",
	}

	if err := saveState(path, want); err != nil {
		t.Fatalf("saveState: %v", err)
	}

	got, err := loadState(path)
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d entries, got %d", len(want), len(got))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("state[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestSaveStateAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Write initial state.
	initial := map[string]string{"a/b#1": "REVIEW_REQUIRED"}
	if err := saveState(path, initial); err != nil {
		t.Fatalf("saveState initial: %v", err)
	}

	// Write updated state.
	updated := map[string]string{"a/b#1": "APPROVED", "c/d#2": ""}
	if err := saveState(path, updated); err != nil {
		t.Fatalf("saveState updated: %v", err)
	}

	// Verify no temp files left behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Fatalf("expected 1 file, got %d: %v", len(entries), names)
	}

	// Verify contents.
	got, err := loadState(path)
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}
	if got["a/b#1"] != "APPROVED" {
		t.Errorf("state[a/b#1] = %q, want APPROVED", got["a/b#1"])
	}
}

// State files written before merge notification existed hold raw reviewDecision
// values. They must still load and compare correctly against the stored
// "APPROVED" marker, since a load error stops every poll.
func TestLoadStateReadsPreMergeValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	legacy := `{
  "envato/repo#1": "REVIEW_REQUIRED",
  "envato/repo#2": "APPROVED",
  "envato/repo#3": ""
}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatalf("writing legacy state: %v", err)
	}

	state, err := loadState(path)
	if err != nil {
		t.Fatalf("loadState on a pre-merge state file: %v", err)
	}

	if len(state) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(state))
	}
	if state["envato/repo#2"] != approved {
		t.Errorf("state[envato/repo#2] = %q, want %q", state["envato/repo#2"], approved)
	}
	if state["envato/repo#1"] == approved {
		t.Error("REVIEW_REQUIRED should not read as approved")
	}
	if state["envato/repo#3"] == approved {
		t.Error("an empty decision should not read as approved")
	}
}
