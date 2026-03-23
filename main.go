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

// version is set at build time via -ldflags "-X main.version=v0.1.0".
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	interval := flag.Duration("interval", 60*time.Second, "poll interval (e.g. 30s, 2m)")
	sound := flag.String("sound", "default", `macOS notification sound ("none" to disable)`)
	barkKey := flag.String("bark-key", "", "Bark device key for iOS push notifications")
	barkServer := flag.String("bark-server", "https://api.day.app", "Bark server URL")
	barkSound := flag.String("bark-sound", "", "Bark notification sound name")
	flag.Parse()

	if *showVersion {
		fmt.Println("gh-pr-notify " + version)
		return
	}

	if err := checkDependencies(); err != nil {
		log.Fatalf("dependency check failed: %v", err)
	}

	dir, err := stateDir()
	if err != nil {
		log.Fatalf("state dir: %v", err)
	}
	statePath := filepath.Join(dir, "state.json")

	barkCfg := barkConfig{
		key:    *barkKey,
		server: *barkServer,
		sound:  *barkSound,
	}

	// Clean shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("gh-pr-notify: polling every %s", *interval)
	if barkCfg.key != "" {
		log.Printf("gh-pr-notify: bark notifications enabled (server: %s)", barkCfg.server)
	}

	// Run first poll immediately, then loop.
	poll(statePath, barkCfg, *sound)

	for {
		select {
		case <-stop:
			log.Println("shutting down")
			return
		case <-time.After(*interval):
			poll(statePath, barkCfg, *sound)
		}
	}
}

// barkConfig holds optional Bark push notification settings.
type barkConfig struct {
	key    string // device key - empty means disabled
	server string // API base URL
	sound  string // notification sound name
}

// poll fetches involved PRs, compares against saved state, notifies on changes,
// and saves the new state.
func poll(statePath string, bark barkConfig, sound string) {
	prs, err := fetchInvolvedPRs()
	if err != nil {
		log.Printf("error fetching PRs: %v", err)
		return
	}

	prevState, err := loadState(statePath)
	if err != nil {
		log.Printf("error loading state: %v", err)
		return
	}

	newState := make(map[string]PRState, len(prs))
	notifyCount := 0

	for _, pr := range prs {
		details, err := fetchPRDetails(pr.Repository.NameWithOwner, pr.Number)
		if err != nil {
			log.Printf("error fetching details for %s: %v", pr.Key(), err)
			continue
		}

		key := pr.Key()
		newState[key] = details

		prev, seen := prevState[key]
		if !seen {
			// First-seen PR: save as baseline, don't notify.
			continue
		}

		if details.ReviewDecision == "APPROVED" && prev.ReviewDecision != "APPROVED" {
			notifyCount++
			log.Printf("APPROVED: %s - %s", key, pr.Title)
			notify(pr, "PR Approved", sound, bark)
		}
		if details.ReviewDecision == "CHANGES_REQUESTED" && prev.ReviewDecision != "CHANGES_REQUESTED" {
			notifyCount++
			log.Printf("CHANGES_REQUESTED: %s - %s", key, pr.Title)
			notify(pr, "Changes Requested", sound, bark)
		}
		if details.CommentCount > prev.CommentCount {
			delta := details.CommentCount - prev.CommentCount
			notifyCount++
			log.Printf("NEW_ACTIVITY: %s - %s (+%d comments/reviews)", key, pr.Title, delta)
			notify(pr, "New Activity", sound, bark)
		}
		if details.CommitCount > prev.CommitCount {
			delta := details.CommitCount - prev.CommitCount
			notifyCount++
			log.Printf("NEW_COMMITS: %s - %s (+%d commits)", key, pr.Title, delta)
			notify(pr, "New Commits", sound, bark)
		}
	}

	if err := saveState(statePath, newState); err != nil {
		log.Printf("error saving state: %v", err)
		return
	}

	log.Printf("poll complete: %d PRs, %d notifications", len(prs), notifyCount)
}

// notify sends a notification via macOS and optionally Bark.
func notify(pr PR, title, sound string, bark barkConfig) {
	if err := sendNotification(pr, title, sound); err != nil {
		log.Printf("notification error for %s: %v", pr.Key(), err)
	}
	if bark.key != "" {
		if err := sendBarkNotification(pr, title, bark.key, bark.server, bark.sound); err != nil {
			log.Printf("bark notification error for %s: %v", pr.Key(), err)
		}
	}
}

// checkDependencies verifies gh and terminal-notifier are installed, and gh is authenticated.
func checkDependencies() error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found in PATH - install with: brew install gh")
	}
	if _, err := exec.LookPath("terminal-notifier"); err != nil {
		return fmt.Errorf("terminal-notifier not found in PATH - install with: brew install terminal-notifier")
	}

	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh not authenticated - run: gh auth login")
	}

	return nil
}
