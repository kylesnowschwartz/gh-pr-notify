package main

import "testing"

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

	if err := sendNotification(pr); err != nil {
		t.Fatalf("sendNotification: %v", err)
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

	if err := sendNotification(pr); err != nil {
		t.Fatalf("sendNotification with special chars: %v", err)
	}
}
