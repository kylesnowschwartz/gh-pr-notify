package main

import (
	"encoding/json"
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

	want := map[string]PRState{
		"envato/repo#123": {ReviewDecision: "REVIEW_REQUIRED", CommentCount: 3, CommitCount: 1},
		"envato/repo#456": {ReviewDecision: "APPROVED", CommentCount: 0, CommitCount: 5},
		"other/thing#7":   {},
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
			t.Errorf("state[%q] = %+v, want %+v", k, got[k], v)
		}
	}
}

func TestSaveStateAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	initial := map[string]PRState{"a/b#1": {ReviewDecision: "REVIEW_REQUIRED"}}
	if err := saveState(path, initial); err != nil {
		t.Fatalf("saveState initial: %v", err)
	}

	updated := map[string]PRState{
		"a/b#1": {ReviewDecision: "APPROVED", CommentCount: 2, CommitCount: 1},
		"c/d#2": {},
	}
	if err := saveState(path, updated); err != nil {
		t.Fatalf("saveState updated: %v", err)
	}

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

	got, err := loadState(path)
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}
	if got["a/b#1"].ReviewDecision != "APPROVED" {
		t.Errorf("state[a/b#1].ReviewDecision = %q, want APPROVED", got["a/b#1"].ReviewDecision)
	}
	if got["a/b#1"].CommentCount != 2 {
		t.Errorf("state[a/b#1].CommentCount = %d, want 2", got["a/b#1"].CommentCount)
	}
}

func TestLoadStateMigratesOldFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	oldState := map[string]string{
		"org/repo#1": "APPROVED",
		"org/repo#2": "REVIEW_REQUIRED",
		"org/repo#3": "",
	}
	data, err := json.MarshalIndent(oldState, "", "  ")
	if err != nil {
		t.Fatalf("marshal old state: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write old state: %v", err)
	}

	got, err := loadState(path)
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	if got["org/repo#1"].ReviewDecision != "APPROVED" {
		t.Errorf("repo#1 decision = %q, want APPROVED", got["org/repo#1"].ReviewDecision)
	}
	if got["org/repo#1"].CommentCount != 0 {
		t.Errorf("repo#1 commentCount = %d, want 0", got["org/repo#1"].CommentCount)
	}
	if got["org/repo#2"].ReviewDecision != "REVIEW_REQUIRED" {
		t.Errorf("repo#2 decision = %q, want REVIEW_REQUIRED", got["org/repo#2"].ReviewDecision)
	}
}

func TestTransitionDetection(t *testing.T) {
	prev := map[string]PRState{
		"org/repo#1": {ReviewDecision: "REVIEW_REQUIRED", CommentCount: 2, CommitCount: 1},
		"org/repo#2": {ReviewDecision: "APPROVED", CommentCount: 5, CommitCount: 3},
		"org/repo#3": {ReviewDecision: "", CommentCount: 0, CommitCount: 1},
	}

	current := map[string]PRState{
		"org/repo#1": {ReviewDecision: "APPROVED", CommentCount: 4, CommitCount: 1},          // approved + 2 comments
		"org/repo#2": {ReviewDecision: "APPROVED", CommentCount: 5, CommitCount: 5},          // 2 new commits only
		"org/repo#3": {ReviewDecision: "CHANGES_REQUESTED", CommentCount: 1, CommitCount: 1}, // changes requested + 1 comment
		"org/repo#4": {ReviewDecision: "APPROVED", CommentCount: 10, CommitCount: 3},         // first-seen, skip
	}

	type notification struct {
		key   string
		title string
	}
	var notifications []notification

	for key, cur := range current {
		prevState, seen := prev[key]
		if !seen {
			continue
		}

		if cur.ReviewDecision == "APPROVED" && prevState.ReviewDecision != "APPROVED" {
			notifications = append(notifications, notification{key, "PR Approved"})
		}
		if cur.ReviewDecision == "CHANGES_REQUESTED" && prevState.ReviewDecision != "CHANGES_REQUESTED" {
			notifications = append(notifications, notification{key, "Changes Requested"})
		}
		if cur.CommentCount > prevState.CommentCount {
			notifications = append(notifications, notification{key, "New Activity"})
		}
		if cur.CommitCount > prevState.CommitCount {
			notifications = append(notifications, notification{key, "New Commits"})
		}
	}

	// repo#1: approved + new activity = 2
	// repo#2: new commits = 1
	// repo#3: changes requested + new activity = 2
	// repo#4: first-seen, skipped
	if len(notifications) != 5 {
		t.Errorf("expected 5 notifications, got %d: %v", len(notifications), notifications)
	}

	for _, n := range notifications {
		if n.key == "org/repo#4" {
			t.Errorf("org/repo#4 is first-seen, should not notify")
		}
	}
}
