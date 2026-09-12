package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Desktop tests post real notifications, so they only run when asked for:
// GH_PR_NOTIFY_TEST_DESKTOP=1 go test -run Notification
func requireDesktopOptIn(t *testing.T) {
	t.Helper()
	if os.Getenv("GH_PR_NOTIFY_TEST_DESKTOP") != "1" {
		t.Skip("set GH_PR_NOTIFY_TEST_DESKTOP=1 to post real desktop notifications")
	}
}

func TestTerminalNotifierArgs(t *testing.T) {
	event := prEvent{
		headline: headlineApproved,
		key:      "test/repo#42",
		prTitle:  "feat: add dark mode",
		url:      "https://github.com/test/repo/pull/42",
	}

	got := terminalNotifierArgs(event, "Submarine")
	want := []string{
		"-title", "PR Approved",
		"-subtitle", `\test/repo#42`,
		"-message", `\feat: add dark mode`,
		"-open", "https://github.com/test/repo/pull/42",
		"-group", "test/repo#42",
		"-sound", "Submarine",
	}

	if !slices.Equal(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
}

func TestTerminalNotifierArgsSilent(t *testing.T) {
	event := prEvent{headline: headlineMerged, key: "test/repo#1", prTitle: "t", url: "https://example.test/1"}

	got := terminalNotifierArgs(event, "none")

	if slices.Contains(got, "-sound") {
		t.Errorf("silent notification must not pass -sound, got %q", got)
	}
}

// terminal-notifier parses each value as a property list. Titles that open with
// a bracket or quote are common on GitHub and must survive as plain strings.
func TestTerminalNotifierArgsBracketedTitle(t *testing.T) {
	event := prEvent{
		headline: headlineApproved,
		key:      "test/repo#7",
		prTitle:  `[WIP] "quoted" (parens) {braces}`,
		url:      "https://example.test/7",
	}

	got := terminalNotifierArgs(event, "default")

	i := slices.Index(got, "-message")
	if i < 0 || i+1 >= len(got) {
		t.Fatalf("no -message in %q", got)
	}
	if got[i+1] != `\[WIP] "quoted" (parens) {braces}` {
		t.Errorf("message = %q, want a leading backslash before the bracket", got[i+1])
	}
}

func TestSelectDesktopNotifierFallsBackWithoutTerminalNotifier(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if _, ok := selectDesktopNotifier().(osascriptNotifier); !ok {
		t.Error("expected osascriptNotifier when terminal-notifier is not on PATH")
	}
}

func TestSelectDesktopNotifierPrefersTerminalNotifier(t *testing.T) {
	dir := t.TempDir()
	stub := dir + "/" + terminalNotifierCommand
	if err := os.WriteFile(stub, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	n, ok := selectDesktopNotifier().(terminalNotifier)
	if !ok {
		t.Fatal("expected terminalNotifier when terminal-notifier is on PATH")
	}
	if n.path != stub {
		t.Errorf("path = %q, want %q", n.path, stub)
	}
}

// recordingNotifier stands in for the fallback and remembers what it was asked to send.
type recordingNotifier struct {
	events []prEvent
	err    error
}

func (r *recordingNotifier) send(event prEvent, _ string) error {
	r.events = append(r.events, event)
	return r.err
}

func (r *recordingNotifier) describe() string { return "recording" }

// stubTerminalNotifier writes a script that exits with the given status and
// returns its path.
func stubTerminalNotifier(t *testing.T, exitCode int) string {
	t.Helper()
	path := t.TempDir() + "/" + terminalNotifierCommand
	script := "#!/bin/sh\necho stub-stderr >&2\nexit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTerminalNotifierSuccessSkipsFallback(t *testing.T) {
	fallback := &recordingNotifier{}
	n := terminalNotifier{path: stubTerminalNotifier(t, 0), fallback: fallback}

	if err := n.send(prEvent{key: "a/b#1"}, "none"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(fallback.events) != 0 {
		t.Errorf("fallback was used on success: %+v", fallback.events)
	}
}

func TestTerminalNotifierNotAuthorizedFallsBack(t *testing.T) {
	fallback := &recordingNotifier{}
	n := terminalNotifier{path: stubTerminalNotifier(t, terminalNotifierExitNotAuthorized), fallback: fallback}
	event := prEvent{headline: headlineApproved, key: "a/b#1", prTitle: "t", url: "https://example.test/1"}

	err := n.send(event, "none")

	if err == nil {
		t.Fatal("expected an error telling the operator to fix the permission")
	}
	if !strings.Contains(err.Error(), "System Settings > Notifications") {
		t.Errorf("error lacks the permission hint: %v", err)
	}
	if len(fallback.events) != 1 || fallback.events[0] != event {
		t.Errorf("fallback events = %+v, want exactly the failed event", fallback.events)
	}
}

func TestTerminalNotifierOtherFailureFallsBack(t *testing.T) {
	fallback := &recordingNotifier{}
	n := terminalNotifier{path: stubTerminalNotifier(t, 2), fallback: fallback}

	err := n.send(prEvent{key: "a/b#1"}, "none")

	if err == nil {
		t.Fatal("expected an error for a non-zero exit")
	}
	if !strings.Contains(err.Error(), "stub-stderr") {
		t.Errorf("error should carry terminal-notifier's stderr: %v", err)
	}
	if len(fallback.events) != 1 {
		t.Errorf("fallback should have been used once, got %+v", fallback.events)
	}
}

func TestTerminalNotifierReportsFallbackFailure(t *testing.T) {
	fallback := &recordingNotifier{err: errors.New("osascript broke")}
	n := terminalNotifier{path: stubTerminalNotifier(t, 5), fallback: fallback}

	err := n.send(prEvent{key: "a/b#1"}, "none")

	if err == nil || !strings.Contains(err.Error(), "osascript broke") {
		t.Errorf("error should include the fallback failure, got: %v", err)
	}
}

func TestTerminalNotifierPostsRealNotification(t *testing.T) {
	requireDesktopOptIn(t)
	path, err := exec.LookPath(terminalNotifierCommand)
	if err != nil {
		t.Skip("terminal-notifier not installed")
	}

	event := prEvent{
		headline: headlineApproved,
		key:      "kylesnowschwartz/gh-pr-notify#1",
		prTitle:  "[test] click me to open the PR",
		url:      "https://github.com/kylesnowschwartz/gh-pr-notify/pull/1",
	}

	n := terminalNotifier{path: path, fallback: &recordingNotifier{}}
	if err := n.send(event, "default"); err != nil {
		t.Fatalf("terminalNotifier.send: %v", err)
	}
}

func TestSendNotificationApproved(t *testing.T) {
	requireDesktopOptIn(t)

	event := prEvent{
		headline: headlineApproved,
		key:      "kylesnowschwartz/gh-pr-notify#1",
		prTitle:  "test: add program name to startup log",
		url:      "https://github.com/kylesnowschwartz/gh-pr-notify/pull/1",
	}

	if err := (osascriptNotifier{}).send(event, "default"); err != nil {
		t.Fatalf("sendNotification: %v", err)
	}
}

func TestSendNotificationMerged(t *testing.T) {
	requireDesktopOptIn(t)

	event := prEvent{
		headline: headlineMerged,
		key:      "kylesnowschwartz/gh-pr-notify#2",
		prTitle:  "feat: notify on merge",
		url:      "https://github.com/kylesnowschwartz/gh-pr-notify/pull/2",
	}

	if err := (osascriptNotifier{}).send(event, "default"); err != nil {
		t.Fatalf("sendNotification: %v", err)
	}
}

func TestSendNotificationSilent(t *testing.T) {
	requireDesktopOptIn(t)

	event := prEvent{
		headline: headlineApproved,
		key:      "kylesnowschwartz/gh-pr-notify#1",
		prTitle:  "test: silent notification",
		url:      "https://github.com/kylesnowschwartz/gh-pr-notify/pull/1",
	}

	if err := (osascriptNotifier{}).send(event, "none"); err != nil {
		t.Fatalf("sendNotification silent: %v", err)
	}
}

func TestSendNotificationEscaping(t *testing.T) {
	requireDesktopOptIn(t)

	event := prEvent{
		headline: headlineApproved,
		key:      "test/repo#99",
		prTitle:  `fix: handle "quoted" and \backslash titles`,
		url:      "https://github.com/test/repo/pull/99",
	}

	if err := (osascriptNotifier{}).send(event, "default"); err != nil {
		t.Fatalf("sendNotification with special chars: %v", err)
	}
}

func TestSendBarkNotification(t *testing.T) {
	var received barkPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/push" {
			t.Errorf("expected /push, got %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(body, &received); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code": 200, "message": "success"}`))
	}))
	defer srv.Close()

	event := prEvent{
		headline: headlineApproved,
		key:      "test/repo#42",
		prTitle:  "feat: add dark mode",
		url:      "https://github.com/test/repo/pull/42",
	}

	if err := sendBarkNotification(event, "test-device-key", srv.URL, "birdsong"); err != nil {
		t.Fatalf("sendBarkNotification: %v", err)
	}

	// Verify payload fields.
	if received.DeviceKey != "test-device-key" {
		t.Errorf("device_key = %q, want %q", received.DeviceKey, "test-device-key")
	}
	if received.Title != "PR Approved" {
		t.Errorf("title = %q, want %q", received.Title, "PR Approved")
	}
	if received.Subtitle != "test/repo#42" {
		t.Errorf("subtitle = %q, want %q", received.Subtitle, "test/repo#42")
	}
	if received.Body != "feat: add dark mode" {
		t.Errorf("body = %q, want %q", received.Body, "feat: add dark mode")
	}
	if received.URL != "https://github.com/test/repo/pull/42" {
		t.Errorf("url = %q, want %q", received.URL, "https://github.com/test/repo/pull/42")
	}
	if received.Group != "gh-pr-notify" {
		t.Errorf("group = %q, want %q", received.Group, "gh-pr-notify")
	}
	if received.Sound != "birdsong" {
		t.Errorf("sound = %q, want %q", received.Sound, "birdsong")
	}
}

func TestSendBarkNotificationMergedHeadline(t *testing.T) {
	var received barkPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code": 200, "message": "success"}`))
	}))
	defer srv.Close()

	event := prEvent{
		headline: headlineMerged,
		key:      "umputun/revdiff#261",
		prTitle:  "feat: add a thing",
		url:      "https://github.com/umputun/revdiff/pull/261",
	}

	if err := sendBarkNotification(event, "key", srv.URL, ""); err != nil {
		t.Fatalf("sendBarkNotification: %v", err)
	}

	if received.Title != "PR Merged" {
		t.Errorf("title = %q, want %q", received.Title, "PR Merged")
	}
	if received.Subtitle != "umputun/revdiff#261" {
		t.Errorf("subtitle = %q, want %q", received.Subtitle, "umputun/revdiff#261")
	}
}

func TestSendBarkNotificationEmptySound(t *testing.T) {
	var rawBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &rawBody)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code": 200, "message": "success"}`))
	}))
	defer srv.Close()

	event := prEvent{
		headline: headlineApproved,
		key:      "test/repo#1",
		prTitle:  "test",
		url:      "https://github.com/test/repo/pull/1",
	}

	if err := sendBarkNotification(event, "key", srv.URL, ""); err != nil {
		t.Fatalf("sendBarkNotification: %v", err)
	}

	// Sound field should be omitted from JSON when empty (omitempty tag).
	if _, exists := rawBody["sound"]; exists {
		t.Errorf("sound field should be omitted when empty, got %v", rawBody["sound"])
	}
}

func TestSendBarkNotificationServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code": 400, "message": "device key not found"}`))
	}))
	defer srv.Close()

	event := prEvent{
		headline: headlineApproved,
		key:      "test/repo#1",
		prTitle:  "test",
		url:      "https://github.com/test/repo/pull/1",
	}

	if err := sendBarkNotification(event, "bad-key", srv.URL, ""); err == nil {
		t.Fatal("expected error for bad device key, got nil")
	}
}
