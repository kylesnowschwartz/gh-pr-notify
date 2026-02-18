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

func TestApprovalTransitionDetection(t *testing.T) {
	// Simulates the diff logic from poll().
	prevState := map[string]string{
		"org/repo#1": "REVIEW_REQUIRED",
		"org/repo#2": "APPROVED",
		"org/repo#3": "CHANGES_REQUESTED",
		"org/repo#4": "",
	}

	// Simulate current poll results.
	currentDecisions := map[string]string{
		"org/repo#1": "APPROVED", // transitioned -> should notify
		"org/repo#2": "APPROVED", // already approved -> no notify
		"org/repo#3": "APPROVED", // transitioned -> should notify
		"org/repo#4": "APPROVED", // transitioned -> should notify
		"org/repo#5": "APPROVED", // new PR, already approved -> should notify
	}

	var notified []string
	for key, decision := range currentDecisions {
		if decision == "APPROVED" && prevState[key] != "APPROVED" {
			notified = append(notified, key)
		}
	}

	if len(notified) != 4 {
		t.Errorf("expected 4 notifications, got %d: %v", len(notified), notified)
	}

	// org/repo#2 should NOT be in the list.
	for _, key := range notified {
		if key == "org/repo#2" {
			t.Errorf("org/repo#2 was already APPROVED, should not notify")
		}
	}
}
