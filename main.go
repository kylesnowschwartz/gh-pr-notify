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

// poll fetches open PRs, compares against saved state, notifies on new approvals,
// and saves the new state.
func poll(statePath string, bark barkConfig, sound string) {
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
			if err := sendNotification(pr, sound); err != nil {
				log.Printf("notification error for %s: %v", key, err)
			}
			if bark.key != "" {
				if err := sendBarkNotification(pr, bark.key, bark.server, bark.sound); err != nil {
					log.Printf("bark notification error for %s: %v", key, err)
				}
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
