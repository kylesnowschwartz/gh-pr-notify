package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	interval := flag.Duration("interval", 60*time.Second, "poll interval (e.g. 30s, 2m)")
	flag.Parse()

	if err := checkDependencies(); err != nil {
		log.Fatalf("dependency check failed: %v", err)
	}

	dir, err := stateDir()
	if err != nil {
		log.Fatalf("state dir: %v", err)
	}
	statePath := filepath.Join(dir, "state.json")

	// Clean shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("gh-pr-notify: polling every %s", *interval)

	// Run first poll immediately, then loop.
	poll(statePath)

	for {
		select {
		case <-stop:
			log.Println("shutting down")
			return
		case <-time.After(*interval):
			poll(statePath)
		}
	}
}

// poll fetches open PRs, compares against saved state, notifies on new approvals,
// and saves the new state.
func poll(statePath string) {
	prs, err := fetchOpenPRs()
	if err != nil {
		log.Printf("error fetching PRs: %v", err)
		return
	}

	prevState, err := loadState(statePath)
	if err != nil {
		log.Printf("error loading state: %v", err)
		return
	}

	newState := make(map[string]string, len(prs))
	approvedCount := 0

	for _, pr := range prs {
		decision, err := fetchReviewDecision(pr.Repository.NameWithOwner, pr.Number)
		if err != nil {
			log.Printf("error fetching review for %s: %v", pr.Key(), err)
			continue
		}

		key := pr.Key()
		newState[key] = decision

		if decision == "APPROVED" && prevState[key] != "APPROVED" {
			approvedCount++
			log.Printf("APPROVED: %s - %s", key, pr.Title)
			if err := sendNotification(pr); err != nil {
				log.Printf("notification error for %s: %v", key, err)
			}
		}
	}

	if err := saveState(statePath, newState); err != nil {
		log.Printf("error saving state: %v", err)
		return
	}

	log.Printf("poll complete: %d PRs, %d new approvals", len(prs), approvedCount)
}

// checkDependencies verifies gh is installed and authenticated.
func checkDependencies() error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found in PATH - install with: brew install gh")
	}

	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh not authenticated - run: gh auth login")
	}

	return nil
}
