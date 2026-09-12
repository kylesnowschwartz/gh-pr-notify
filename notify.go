package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// Headlines shown as the notification title.
const (
	headlineApproved = "PR Approved"
	headlineMerged   = "PR Merged"
)

// prEvent is a single PR change worth announcing.
type prEvent struct {
	headline string // headlineApproved or headlineMerged
	key      string // "envato/repo#123"
	prTitle  string
	url      string
}

// desktopNotifier posts a macOS notification for a PR event.
//
// Sound can be "default", "none" (silent), or any macOS system sound name
// (Basso, Blow, Bottle, Frog, Funk, Glass, Hero, Morse, Ping, Pop, Purr,
// Sosumi, Submarine, Tink).
type desktopNotifier interface {
	send(event prEvent, sound string) error
	// describe says, for the startup log, which mechanism is in use and what
	// clicking a notification does.
	describe() string
}

// terminalNotifierCommand is the CLI that posts clickable notifications.
// Homebrew installs it with: brew install terminal-notifier
const terminalNotifierCommand = "terminal-notifier"

// selectDesktopNotifier prefers terminal-notifier, whose notifications open the
// PR when clicked, and falls back to osascript, whose notifications cannot.
func selectDesktopNotifier() desktopNotifier {
	if path, err := exec.LookPath(terminalNotifierCommand); err == nil {
		return terminalNotifier{path: path, fallback: osascriptNotifier{}}
	}
	return osascriptNotifier{}
}

// terminalNotifier posts notifications through terminal-notifier. Clicking one
// opens the PR in the default browser. terminal-notifier hands the notification
// to macOS and exits, so a click is handled even though this process has moved on.
//
// When terminal-notifier fails, the event goes out through fallback instead so
// it still reaches the desktop, and the returned error says what went wrong.
type terminalNotifier struct {
	path     string
	fallback desktopNotifier
}

func (n terminalNotifier) describe() string {
	return "desktop notifications via terminal-notifier; clicking one opens the PR"
}

// terminalNotifierExitNotAuthorized is terminal-notifier's exit status when
// macOS has not granted it permission to post notifications.
const terminalNotifierExitNotAuthorized = 3

func (n terminalNotifier) send(event prEvent, sound string) error {
	cmd := exec.Command(n.path, terminalNotifierArgs(event, sound)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return nil
	}

	var cause string
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == terminalNotifierExitNotAuthorized {
		cause = fmt.Sprintf("terminal-notifier is not allowed to post notifications "+
			"(enable it under System Settings > Notifications, then check with: %s -diagnose)",
			terminalNotifierCommand)
	} else {
		cause = fmt.Sprintf("terminal-notifier failed: %v: %s", err, strings.TrimSpace(stderr.String()))
	}

	if fallbackErr := n.fallback.send(event, sound); fallbackErr != nil {
		return fmt.Errorf("%s; the fallback failed too: %w", cause, fallbackErr)
	}
	return fmt.Errorf("%s; this notification went out without click-to-open", cause)
}

// terminalNotifierArgs builds the argument list for one notification.
//
// The PR key doubles as the notification group, so a merge notice replaces a
// still-visible approval notice for the same PR instead of stacking under it.
func terminalNotifierArgs(event prEvent, sound string) []string {
	args := []string{
		"-title", event.headline,
		"-subtitle", plistLiteral(event.key),
		"-message", plistLiteral(event.prTitle),
		"-open", event.url,
		"-group", event.key,
	}
	if sound != "none" {
		args = append(args, "-sound", sound)
	}
	return args
}

// plistLiteral marks a free-text argument so terminal-notifier reads it as a
// plain string. It parses each value as a property list, so a value starting
// with '[', '(', '{' or a quote would otherwise be rejected or mangled. A
// leading backslash is stripped by terminal-notifier and forces plain-string
// handling for whatever follows.
func plistLiteral(s string) string {
	return `\` + s
}

// osascriptNotifier posts notifications through AppleScript's display
// notification. Clicking one does nothing useful (it activates Script Editor),
// but the text carries the PR identifier and title, which is enough to find it.
type osascriptNotifier struct{}

func (osascriptNotifier) describe() string {
	return "desktop notifications via osascript; clicking one does not open the PR " +
		"(install terminal-notifier for that: brew install terminal-notifier)"
}

func (osascriptNotifier) send(event prEvent, sound string) error {
	// AppleScript double-quoted strings need backslashes and quotes escaped.
	// Order matters: escape backslashes first, then quotes.
	escape := func(s string) string {
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `"`, `\"`)
		return s
	}

	script := fmt.Sprintf(
		`display notification "%s" with title "%s" subtitle "%s"`,
		escape(event.prTitle), escape(event.headline), escape(event.key),
	)
	if sound != "none" {
		script += fmt.Sprintf(` sound name "%s"`, escape(sound))
	}

	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("osascript notification: %w", err)
	}

	return nil
}

// barkPayload is the JSON body for Bark's POST /push endpoint.
type barkPayload struct {
	DeviceKey string `json:"device_key"`
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle"`
	Body      string `json:"body"`
	URL       string `json:"url"`
	Group     string `json:"group"`
	Sound     string `json:"sound,omitempty"`
}

var barkHTTPClient = &http.Client{Timeout: 10 * time.Second}

// sendBarkNotification pushes a notification to an iOS device via the Bark API.
// Tapping the notification opens the PR URL in Safari.
func sendBarkNotification(event prEvent, deviceKey, server, sound string) error {
	payload := barkPayload{
		DeviceKey: deviceKey,
		Title:     event.headline,
		Subtitle:  event.key,
		Body:      event.prTitle,
		URL:       event.url,
		Group:     "gh-pr-notify",
		Sound:     sound,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("bark marshal: %w", err)
	}

	resp, err := barkHTTPClient.Post(server+"/push", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("bark request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bark response: %s", resp.Status)
	}

	// Bark returns {"code": 200, "message": "success"} on success.
	// A non-200 code in the JSON body means the device key is invalid, etc.
	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("bark decode: %w", err)
	}
	if result.Code != 200 {
		return fmt.Errorf("bark API error: %d %s", result.Code, result.Message)
	}

	return nil
}
