package main

import (
	"errors"
	"testing"
	"time"
)

func TestClassifyDeparture(t *testing.T) {
	tests := []struct {
		state string
		want  departureAction
	}{
		{state: "MERGED", want: announceMerge},
		{state: "CLOSED", want: forget},
		{state: "OPEN", want: keepWatching},
		{state: "", want: keepWatching},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			if got := classifyDeparture(tt.state); got != tt.want {
				t.Errorf("classifyDeparture(%q) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestMergeEventsAnnouncesMergedPR(t *testing.T) {
	prevState := map[string]string{"umputun/revdiff#261": approved}
	newState := map[string]string{}

	lookup := func(repo string, number int) (outcome, error) {
		if repo != "umputun/revdiff" || number != 261 {
			t.Fatalf("looked up %s#%d, want umputun/revdiff#261", repo, number)
		}
		return outcome{State: "MERGED", Title: "feat: add a thing", URL: "https://example.test/261"}, nil
	}

	events := mergeEvents(prevState, map[string]bool{}, newState, lookup)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d: %+v", len(events), events)
	}
	if events[0].headline != headlineMerged {
		t.Errorf("headline = %q, want %q", events[0].headline, headlineMerged)
	}
	if events[0].prTitle != "feat: add a thing" {
		t.Errorf("prTitle = %q, want %q", events[0].prTitle, "feat: add a thing")
	}
	if events[0].url != "https://example.test/261" {
		t.Errorf("url = %q, want %q", events[0].url, "https://example.test/261")
	}

	// The key must not survive, or the merge is announced again next poll.
	if _, kept := newState["umputun/revdiff#261"]; kept {
		t.Error("merged PR was kept in state, which would re-notify")
	}
}

func TestMergeEventsIgnoresClosedPR(t *testing.T) {
	prevState := map[string]string{"a/b#1": ""}
	newState := map[string]string{}

	lookup := func(string, int) (outcome, error) {
		return outcome{State: "CLOSED", Title: "abandoned"}, nil
	}

	events := mergeEvents(prevState, map[string]bool{}, newState, lookup)

	if len(events) != 0 {
		t.Errorf("expected no events for a closed PR, got %+v", events)
	}
	if _, kept := newState["a/b#1"]; kept {
		t.Error("closed PR should be dropped from state")
	}
}

func TestMergeEventsSkipsStillOpenPRs(t *testing.T) {
	prevState := map[string]string{"a/b#1": approved}
	newState := map[string]string{"a/b#1": approved}

	lookup := func(string, int) (outcome, error) {
		t.Fatal("lookup should not run for a PR still in the open list")
		return outcome{}, nil
	}

	if events := mergeEvents(prevState, map[string]bool{"a/b#1": true}, newState, lookup); len(events) != 0 {
		t.Errorf("expected no events, got %+v", events)
	}
}

// A short or partly failed search must not be read as a merge.
func TestMergeEventsKeepsPRGitHubStillCallsOpen(t *testing.T) {
	prevState := map[string]string{"a/b#1": approved}
	newState := map[string]string{}

	lookup := func(string, int) (outcome, error) {
		return outcome{State: "OPEN", Title: "still going"}, nil
	}

	events := mergeEvents(prevState, map[string]bool{}, newState, lookup)

	if len(events) != 0 {
		t.Errorf("expected no events for a PR GitHub still calls open, got %+v", events)
	}
	if newState["a/b#1"] != approved {
		t.Errorf("state[a/b#1] = %q, want %q carried forward", newState["a/b#1"], approved)
	}
}

func TestMergeEventsCarriesForwardOnLookupError(t *testing.T) {
	prevState := map[string]string{"a/b#1": approved}
	newState := map[string]string{}

	lookup := func(string, int) (outcome, error) {
		return outcome{}, errors.New("gh pr view: exit status 1")
	}

	events := mergeEvents(prevState, map[string]bool{}, newState, lookup)

	if len(events) != 0 {
		t.Errorf("expected no events when the lookup fails, got %+v", events)
	}
	if newState["a/b#1"] != approved {
		t.Errorf("state[a/b#1] = %q, want %q retained for the next poll", newState["a/b#1"], approved)
	}
}

func TestMergeEventsDropsUnusableKey(t *testing.T) {
	prevState := map[string]string{"nonsense": approved}
	newState := map[string]string{}

	lookup := func(string, int) (outcome, error) {
		t.Fatal("lookup should not run for an unparseable key")
		return outcome{}, nil
	}

	if events := mergeEvents(prevState, map[string]bool{}, newState, lookup); len(events) != 0 {
		t.Errorf("expected no events, got %+v", events)
	}
	if _, kept := newState["nonsense"]; kept {
		t.Error("unusable key should not be carried forward")
	}
}

func TestPollReporterStaysQuietWhenNothingChanges(t *testing.T) {
	reporter := &pollReporter{heartbeat: time.Hour}

	// First report always writes, establishing the baseline.
	reporter.report(12, 0)
	baseline := reporter.lastLog
	if baseline.IsZero() {
		t.Fatal("first report should have logged")
	}

	// Same count, no events, inside the heartbeat window: no new log line.
	reporter.report(12, 0)
	if !reporter.lastLog.Equal(baseline) {
		t.Error("an uneventful repeat poll should not log")
	}

	// A changed count breaks the silence.
	reporter.report(13, 0)
	if reporter.lastLog.Equal(baseline) {
		t.Error("a changed PR count should log")
	}
	if reporter.lastCount != 13 {
		t.Errorf("lastCount = %d, want 13", reporter.lastCount)
	}
}

func TestPollReporterLogsEvents(t *testing.T) {
	reporter := &pollReporter{heartbeat: time.Hour}
	reporter.report(5, 0)
	baseline := reporter.lastLog

	reporter.report(5, 1)
	if reporter.lastLog.Equal(baseline) {
		t.Error("a poll with notifications should always log")
	}
}

func TestPollReporterHeartbeat(t *testing.T) {
	reporter := &pollReporter{heartbeat: time.Hour}
	reporter.report(5, 0)

	// Backdate the last log past the heartbeat window.
	reporter.lastLog = reporter.lastLog.Add(-2 * time.Hour)
	stale := reporter.lastLog

	reporter.report(5, 0)
	if reporter.lastLog.Equal(stale) {
		t.Error("an uneventful poll past the heartbeat window should log")
	}
}
