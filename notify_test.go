package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// osascript tests post real notifications to the desktop, so they only run when
// asked for: GH_PR_NOTIFY_TEST_DESKTOP=1 go test -run Notification
func requireDesktopOptIn(t *testing.T) {
	t.Helper()
	if os.Getenv("GH_PR_NOTIFY_TEST_DESKTOP") != "1" {
		t.Skip("set GH_PR_NOTIFY_TEST_DESKTOP=1 to post real desktop notifications")
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

	if err := sendNotification(event, "default"); err != nil {
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

	if err := sendNotification(event, "default"); err != nil {
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

	if err := sendNotification(event, "none"); err != nil {
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

	if err := sendNotification(event, "default"); err != nil {
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
