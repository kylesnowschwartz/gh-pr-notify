package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

// PR represents an open pull request from gh search.
type PR struct {
	Number     int        `json:"number"`
	Title      string     `json:"title"`
	URL        string     `json:"url"`
	Repository Repository `json:"repository"`
}

// Repository identifies the repo a PR belongs to.
type Repository struct {
	Name          string `json:"name"`
	NameWithOwner string `json:"nameWithOwner"`
}

// Key returns a stable identifier like "envato/repo#123" for state tracking.
func (pr PR) Key() string {
	return pr.Repository.NameWithOwner + "#" + strconv.Itoa(pr.Number)
}

// fetchOpenPRs returns all open PRs authored by the authenticated user.
func fetchOpenPRs() ([]PR, error) {
	cmd := exec.Command("gh", "search", "prs",
		"--author", "@me",
		"--state", "open",
		"--json", "number,title,url,repository",
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh search prs: %w", err)
	}

	var prs []PR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parsing PR list: %w", err)
	}

	return prs, nil
}

// reviewDecision holds the JSON response from gh pr view.
type reviewDecision struct {
	ReviewDecision string `json:"reviewDecision"`
}

// fetchReviewDecision returns the reviewDecision string for a single PR.
// Possible values: "APPROVED", "REVIEW_REQUIRED", "CHANGES_REQUESTED", or "" (no rules).
func fetchReviewDecision(repo string, number int) (string, error) {
	cmd := exec.Command("gh", "pr", "view",
		strconv.Itoa(number),
		"--repo", repo,
		"--json", "reviewDecision",
	)

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh pr view %s#%d: %w", repo, number, err)
	}

	var rd reviewDecision
	if err := json.Unmarshal(out, &rd); err != nil {
		return "", fmt.Errorf("parsing review decision for %s#%d: %w", repo, number, err)
	}

	return rd.ReviewDecision, nil
}
