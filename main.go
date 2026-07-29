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
	heartbeat := flag.Duration("heartbeat", time.Hour, "how often to log an uneventful poll")
	logFile := flag.String("log-file", "", "append logs to this file with rotation (default: stderr)")
	logMaxBytes := flag.Int64("log-max-bytes", defaultLogMaxBytes, "rotate --log-file once it exceeds this size")
	sound := flag.String("sound", "default", `macOS notification sound ("none" to disable)`)
	barkKey := flag.String("bark-key", "", "Bark device key for iOS push notifications")
	barkServer := flag.String("bark-server", "https://api.day.app", "Bark server URL")
	barkSound := flag.String("bark-sound", "", "Bark notification sound name")
	flag.Parse()

	if *showVersion {
		fmt.Println("gh-pr-notify " + version)
		return
	}

	if *logFile != "" {
		writer, err := newRotatingWriter(*logFile, *logMaxBytes)
		if err != nil {
			log.Fatalf("log setup: %v", err)
		}
		defer writer.Close()

		log.SetOutput(writer)
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

	log.Printf("gh-pr-notify %s: polling every %s, heartbeat every %s", version, *interval, *heartbeat)
	if barkCfg.key != "" {
		log.Printf("gh-pr-notify: bark notifications enabled (server: %s)", barkCfg.server)
	}

	reporter := &pollReporter{heartbeat: *heartbeat}

	// Run first poll immediately, then loop.
	poll(statePath, barkCfg, *sound, reporter)

	for {
		select {
		case <-stop:
			log.Println("shutting down")
			return
		case <-time.After(*interval):
			poll(statePath, barkCfg, *sound, reporter)
		}
	}
}

// barkConfig holds optional Bark push notification settings.
type barkConfig struct {
	key    string // device key - empty means disabled
	server string // API base URL
	sound  string // notification sound name
}

// pollReporter keeps the service log quiet. An uneventful poll every 60 seconds
// grows the log without saying anything new, so a summary line is written only
// when something is announced, when the open-PR count moves, or when the
// heartbeat interval has elapsed.
type pollReporter struct {
	heartbeat time.Duration
	lastCount int
	lastLog   time.Time
}

func (r *pollReporter) report(prCount, eventCount int) {
	now := time.Now()
	if eventCount == 0 && prCount == r.lastCount && now.Sub(r.lastLog) < r.heartbeat {
		return
	}

	log.Printf("poll complete: %d open PRs, %d notifications", prCount, eventCount)
	r.lastCount = prCount
	r.lastLog = now
}

// departureAction says what to do about a PR that is no longer in the open list.
type departureAction int

const (
	announceMerge departureAction = iota
	forget
	keepWatching
)

// classifyDeparture decides how to handle a PR that dropped out of the open list,
// based on the state GitHub reports for it.
func classifyDeparture(state string) departureAction {
	switch state {
	case "MERGED":
		return announceMerge
	case "CLOSED":
		return forget
	default:
		// GitHub still calls it open, so the search result was short rather than
		// the PR having gone anywhere. Keep it and settle it on a later poll.
		return keepWatching
	}
}

// poll fetches open PRs, announces new approvals and merges, and saves the new state.
func poll(statePath string, bark barkConfig, sound string, reporter *pollReporter) {
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
	stillOpen := make(map[string]bool, len(prs))
	var events []prEvent

	for _, pr := range prs {
		key := pr.Key()
		stillOpen[key] = true

		status, err := fetchReviewStatus(pr.Repository.NameWithOwner, pr.Number)
		if err != nil {
			log.Printf("error fetching review for %s: %v", key, err)
			// Carry the last known value forward. Dropping the key would make the
			// next poll read it as new and announce an approval already announced.
			newState[key] = prevState[key]
			continue
		}

		if isApproved(status) {
			newState[key] = approved
			if prevState[key] != approved {
				events = append(events, prEvent{
					headline: headlineApproved,
					key:      key,
					prTitle:  pr.Title,
					url:      pr.URL,
				})
			}
			continue
		}

		newState[key] = ""
	}

	events = append(events, mergeEvents(prevState, stillOpen, newState, fetchOutcome)...)

	// Save before notifying. A crash between the two costs at most one
	// notification, where notifying first would replay it every poll until the
	// write succeeded.
	if err := saveState(statePath, newState); err != nil {
		log.Printf("error saving state: %v", err)
		return
	}

	for _, event := range events {
		log.Printf("%s: %s - %s", event.headline, event.key, event.prTitle)

		if err := sendNotification(event, sound); err != nil {
			log.Printf("notification error for %s: %v", event.key, err)
		}
		if bark.key != "" {
			if err := sendBarkNotification(event, bark.key, bark.server, bark.sound); err != nil {
				log.Printf("bark notification error for %s: %v", event.key, err)
			}
		}
	}

	reporter.report(len(prs), len(events))
}

// mergeEvents finds PRs that were open on the last poll and are absent now, and
// returns an event for each one GitHub confirms as merged. Absence alone is not
// treated as a merge: a truncated or failed search would otherwise announce
// merges that never happened.
//
// Keys worth another look are written back into newState; merged and closed PRs
// are left out so they stop being polled.
func mergeEvents(
	prevState map[string]string,
	stillOpen map[string]bool,
	newState map[string]string,
	lookup func(repo string, number int) (outcome, error),
) []prEvent {
	var events []prEvent

	for key := range prevState {
		if stillOpen[key] {
			continue
		}

		repo, number, err := parseKey(key)
		if err != nil {
			log.Printf("dropping unusable state key: %v", err)
			continue
		}

		result, err := lookup(repo, number)
		if err != nil {
			log.Printf("error fetching outcome for %s: %v", key, err)
			newState[key] = prevState[key]
			continue
		}

		switch classifyDeparture(result.State) {
		case announceMerge:
			events = append(events, prEvent{
				headline: headlineMerged,
				key:      key,
				prTitle:  result.Title,
				url:      result.URL,
			})
		case keepWatching:
			newState[key] = prevState[key]
		case forget:
			// Closed without merging - nothing to announce.
		}
	}

	return events
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
