package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendNotification(t *testing.T) {
	pr := PR{
		Number: 1,
		Title:  "test: add program name to startup log",
		URL:    "https://github.com/kylesnowschwartz/gh-pr-notify/pull/1",
		Repository: Repository{
			Name:          "gh-pr-notify",
			NameWithOwner: "kylesnowschwartz/gh-pr-notify",
		},
	}

	if err := sendNotification(pr, "default"); err != nil {
		t.Fatalf("sendNotification: %v", err)
	}
}

func TestSendNotificationSilent(t *testing.T) {
	pr := PR{
		Number: 1,
		Title:  "test: silent notification",
		URL:    "https://github.com/kylesnowschwartz/gh-pr-notify/pull/1",
		Repository: Repository{
			Name:          "gh-pr-notify",
			NameWithOwner: "kylesnowschwartz/gh-pr-notify",
		},
	}

	if err := sendNotification(pr, "none"); err != nil {
		t.Fatalf("sendNotification silent: %v", err)
	}
}

func TestSendNotificationEscaping(t *testing.T) {
	pr := PR{
		Number: 99,
		Title:  `fix: handle "quoted" and \backslash titles`,
		URL:    "https://github.com/test/repo/pull/99",
		Repository: Repository{
			Name:          "repo",
			NameWithOwner: "test/repo",
		},
	}

	if err := sendNotification(pr, "default"); err != nil {
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

	pr := PR{
		Number: 42,
		Title:  "feat: add dark mode",
		URL:    "https://github.com/test/repo/pull/42",
		Repository: Repository{
			Name:          "repo",
			NameWithOwner: "test/repo",
		},
	}

	err := sendBarkNotification(pr, "test-device-key", srv.URL, "birdsong")
	if err != nil {
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

func TestSendBarkNotificationEmptySound(t *testing.T) {
	var rawBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &rawBody)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code": 200, "message": "success"}`))
	}))
	defer srv.Close()

	pr := PR{
		Number:     1,
		Title:      "test",
		URL:        "https://github.com/test/repo/pull/1",
		Repository: Repository{Name: "repo", NameWithOwner: "test/repo"},
	}

	err := sendBarkNotification(pr, "key", srv.URL, "")
	if err != nil {
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

	pr := PR{
		Number:     1,
		Title:      "test",
		URL:        "https://github.com/test/repo/pull/1",
		Repository: Repository{Name: "repo", NameWithOwner: "test/repo"},
	}

	err := sendBarkNotification(pr, "bad-key", srv.URL, "")
	if err == nil {
		t.Fatal("expected error for bad device key, got nil")
	}
}
