package main

import (
	"bytes"
	"encoding/json"
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

// sendNotification sends a macOS notification for a PR event via osascript.
//
// Sound can be "default", "none" (silent), or any macOS system sound name
// (Basso, Blow, Bottle, Frog, Funk, Glass, Hero, Morse, Ping, Pop, Purr,
// Sosumi, Submarine, Tink). Pass "none" to suppress the sound entirely.
//
// osascript can't open URLs on click (that goes to Script Editor), but the
// notification text contains the PR identifier and title - enough to find it.
func sendNotification(event prEvent, sound string) error {
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
