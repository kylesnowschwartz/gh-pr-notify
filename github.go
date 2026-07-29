package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// approved is the state value stored for a PR that has the green tick, and the
// review state gh reports for an approving review.
const approved = "APPROVED"

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

// parseKey splits a state key like "envato/repo#123" back into its repo and number.
func parseKey(key string) (repo string, number int, err error) {
	repo, num, found := strings.Cut(key, "#")
	if !found || repo == "" {
		return "", 0, fmt.Errorf("malformed key %q", key)
	}

	number, err = strconv.Atoi(num)
	if err != nil {
		return "", 0, fmt.Errorf("malformed PR number in key %q", key)
	}

	return repo, number, nil
}

// fetchOpenPRs returns all open PRs authored by the authenticated user.
func fetchOpenPRs() ([]PR, error) {
	cmd := exec.Command("gh", "search", "prs",
		"--author", "@me",
		"--state", "open",
		"--limit", "100",
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

// Review is a single submitted review on a PR. State is one of APPROVED,
// CHANGES_REQUESTED, COMMENTED, or DISMISSED.
type Review struct {
	State string `json:"state"`
}

// reviewStatus holds the review fields gh returns for an open PR.
type reviewStatus struct {
	// Decision is GitHub's rolled-up verdict: APPROVED, REVIEW_REQUIRED,
	// CHANGES_REQUESTED, or empty on repos with no required-review rule.
	Decision string   `json:"reviewDecision"`
	Reviews  []Review `json:"reviews"`
}

// fetchReviewStatus returns the review decision and submitted reviews for one PR.
func fetchReviewStatus(repo string, number int) (reviewStatus, error) {
	cmd := exec.Command("gh", "pr", "view",
		strconv.Itoa(number),
		"--repo", repo,
		"--json", "reviewDecision,reviews",
	)

	out, err := cmd.Output()
	if err != nil {
		return reviewStatus{}, fmt.Errorf("gh pr view %s#%d: %w", repo, number, err)
	}

	var status reviewStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return reviewStatus{}, fmt.Errorf("parsing review status for %s#%d: %w", repo, number, err)
	}

	return status, nil
}

// isApproved reports whether a PR carries the green tick.
//
// reviewDecision alone is not enough. Repos without a required-review rule leave
// it empty even after someone approves, so an approving review in the reviews
// array counts on its own. A CHANGES_REQUESTED decision always wins, since it
// means a later review superseded the approval. Withdrawn approvals arrive as
// DISMISSED rather than APPROVED, so they do not count.
func isApproved(status reviewStatus) bool {
	if status.Decision == "CHANGES_REQUESTED" {
		return false
	}
	if status.Decision == approved {
		return true
	}

	for _, review := range status.Reviews {
		if review.State == approved {
			return true
		}
	}

	return false
}

// outcome describes how a PR left the open list.
type outcome struct {
	State string `json:"state"` // MERGED, CLOSED, or OPEN
	Title string `json:"title"`
	URL   string `json:"url"`
}

// fetchOutcome returns the final state of a PR that is no longer in the open
// list, along with the title and URL needed to announce it.
func fetchOutcome(repo string, number int) (outcome, error) {
	cmd := exec.Command("gh", "pr", "view",
		strconv.Itoa(number),
		"--repo", repo,
		"--json", "state,title,url",
	)

	out, err := cmd.Output()
	if err != nil {
		return outcome{}, fmt.Errorf("gh pr view %s#%d: %w", repo, number, err)
	}

	var result outcome
	if err := json.Unmarshal(out, &result); err != nil {
		return outcome{}, fmt.Errorf("parsing outcome for %s#%d: %w", repo, number, err)
	}

	return result, nil
}
