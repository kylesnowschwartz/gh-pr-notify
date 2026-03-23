package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// PRState tracks the last-seen state of a PR for change detection.
type PRState struct {
	ReviewDecision string `json:"reviewDecision"`
	CommentCount   int    `json:"commentCount"`
	CommitCount    int    `json:"commitCount"`
}

// stateDir returns ~/.local/state/gh-pr-notify/, creating it if needed.
func stateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home dir: %w", err)
	}

	dir := filepath.Join(home, ".local", "state", "gh-pr-notify")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating state dir: %w", err)
	}

	return dir, nil
}

// loadState reads the state file and returns a map of PR key -> PRState.
// Returns an empty map if the file doesn't exist yet.
// Migrates from the old map[string]string format if detected.
func loadState(path string) (map[string]PRState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]PRState), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	// Try new format first. json.Unmarshal returns an error when it encounters
	// a string value where it expects a PRState object, so a type mismatch
	// falls through to the old-format migration below.
	state := make(map[string]PRState)
	if err := json.Unmarshal(data, &state); err == nil {
		return state, nil
	}

	// Fall back to old format (map[string]string) and migrate.
	oldState := make(map[string]string)
	if err := json.Unmarshal(data, &oldState); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}

	for k, v := range oldState {
		state[k] = PRState{ReviewDecision: v}
	}
	return state, nil
}

// saveState writes the state map to disk atomically.
// Writes to a temp file in the same directory, then renames.
func saveState(path string, state map[string]PRState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "state-*.json")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("writing temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}
