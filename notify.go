package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// sendNotification sends a macOS notification for an approved PR via osascript.
//
// osascript can't open URLs on click (that goes to Script Editor), but the
// notification text contains the PR identifier and title - enough to find it.
// A future upgrade to alerter (brew install vjeantet/tap/alerter) would add
// click-to-open support.
func sendNotification(pr PR) error {
	title := "PR Approved"
	subtitle := pr.Key()
	message := pr.Title

	// AppleScript double-quoted strings need backslashes and quotes escaped.
	// Order matters: escape backslashes first, then quotes.
	message = strings.ReplaceAll(message, `\`, `\\`)
	message = strings.ReplaceAll(message, `"`, `\"`)
	subtitle = strings.ReplaceAll(subtitle, `\`, `\\`)
	subtitle = strings.ReplaceAll(subtitle, `"`, `\"`)

	script := fmt.Sprintf(
		`display notification "%s" with title "%s" subtitle "%s" sound name "default"`,
		message, title, subtitle,
	)

	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("osascript notification: %w", err)
	}

	return nil
}
