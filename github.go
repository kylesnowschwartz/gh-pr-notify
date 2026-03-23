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

// Key returns a stable identifier like "owner/repo#123" for state tracking.
func (pr PR) Key() string {
	return pr.Repository.NameWithOwner + "#" + strconv.Itoa(pr.Number)
}

// fetchInvolvedPRs returns open PRs where the authenticated user is the author
// or a requested reviewer. Results are deduplicated by PR key.
func fetchInvolvedPRs() ([]PR, error) {
	authored, err := searchPRs("--author", "@me")
	if err != nil {
		return nil, fmt.Errorf("authored PRs: %w", err)
	}

	reviewing, err := searchPRs("--review-requested", "@me")
	if err != nil {
		return nil, fmt.Errorf("reviewer PRs: %w", err)
	}

	seen := make(map[string]bool, len(authored))
	for _, pr := range authored {
		seen[pr.Key()] = true
	}

	merged := make([]PR, len(authored))
	copy(merged, authored)
	for _, pr := range reviewing {
		if !seen[pr.Key()] {
			merged = append(merged, pr)
			seen[pr.Key()] = true
		}
	}

	return merged, nil
}

func searchPRs(filterFlag, filterValue string) ([]PR, error) {
	cmd := exec.Command("gh", "search", "prs",
		filterFlag, filterValue,
		"--state", "open",
		"--limit", "200",
		"--json", "number,title,url,repository",
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh search prs %s %s: %w", filterFlag, filterValue, err)
	}

	var prs []PR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parsing PR list: %w", err)
	}

	return prs, nil
}

// prDetails holds the JSON response from gh pr view with expanded fields.
type prDetails struct {
	ReviewDecision string          `json:"reviewDecision"`
	Comments       json.RawMessage `json:"comments"`
	Commits        json.RawMessage `json:"commits"`
	Reviews        json.RawMessage `json:"reviews"`
}

// fetchPRDetails returns the current state of a PR: review decision,
// combined comment+review count, and commit count.
func fetchPRDetails(repo string, number int) (PRState, error) {
	cmd := exec.Command("gh", "pr", "view",
		strconv.Itoa(number),
		"--repo", repo,
		"--json", "reviewDecision,comments,commits,reviews",
	)

	out, err := cmd.Output()
	if err != nil {
		return PRState{}, fmt.Errorf("gh pr view %s#%d: %w", repo, number, err)
	}

	var details prDetails
	if err := json.Unmarshal(out, &details); err != nil {
		return PRState{}, fmt.Errorf("parsing details for %s#%d: %w", repo, number, err)
	}

	// Count array lengths without fully parsing the objects.
	var comments, commits, reviews []json.RawMessage
	json.Unmarshal(details.Comments, &comments)
	json.Unmarshal(details.Commits, &commits)
	json.Unmarshal(details.Reviews, &reviews)

	return PRState{
		ReviewDecision: details.ReviewDecision,
		CommentCount:   len(comments) + len(reviews),
		CommitCount:    len(commits),
	}, nil
}
