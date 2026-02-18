# gh-pr-notify

A lightweight CLI that polls your open GitHub PRs and sends a macOS desktop notification when one gets approved.

## Problem

GitHub's notification system is noisy. You get notified about comments, CI, mentions, review requests - everything. When a PR gets approved, that signal drowns in the noise. Existing tools (Trailer, Gitify, Neat) are full notification managers. Nothing does just the one thing: tell me when my PR got the green tick.

## Approach

Shell script wrapping `gh` CLI. No compiled binary, no dependencies beyond `gh` and `terminal-notifier`. Runs as a background poller via launchd.

### Why not Go?

A compiled Go binary would be cleaner in some ways, but for a personal tool that shells out to `gh` anyway, a shell script is:
- Faster to iterate on
- No build step
- Trivially inspectable and editable
- Portable to any machine with `gh` installed

If this outgrows a shell script, rewrite in Go then.

## Data Source

### `gh search prs` (cross-repo discovery)

Finds all your open PRs across every repo you have access to:

```sh
gh search prs --author @me --state open --json number,title,url,repository
```

Returns:
```json
[
  {
    "number": 1202,
    "repository": {
      "name": "author-finances",
      "nameWithOwner": "envato/author-finances"
    },
    "title": "feat(ATH-1743): Dual-write W-form submissions",
    "url": "https://github.com/envato/author-finances/pull/1202"
  }
]
```

Note: `gh search prs` does NOT return `reviewDecision`. Need a second call per PR.

### `gh pr view` (review status per PR)

```sh
gh pr view 1202 --repo envato/author-finances --json number,reviewDecision,latestReviews
```

Returns:
```json
{
  "number": 1202,
  "reviewDecision": "REVIEW_REQUIRED",
  "latestReviews": []
}
```

`reviewDecision` values:
- `"REVIEW_REQUIRED"` - needs review (or has changes-requested)
- `"APPROVED"` - all required reviewers approved
- `"CHANGES_REQUESTED"` - reviewer requested changes
- `""` (empty string) - no review rules configured on the repo

The transition we care about: anything -> `"APPROVED"`.

### Alternative: GitHub Notifications API

```sh
gh api /notifications --method GET -f all=false -f participating=true
```

Returns notification objects with `reason` field (e.g., `"state_change"`, `"review_requested"`). But there's no `reason` value for "your PR was approved" specifically - you'd have to chase the `subject.url` to a PR and check its review status anyway. Two API calls either way, and the notifications API is noisier. Stick with the direct approach.

## Architecture

```
gh-pr-notify
  |
  +-- poll loop (every 60s)
  |     |
  |     +-- gh search prs --author @me --state open
  |     |     -> list of (repo, number, title, url)
  |     |
  |     +-- for each PR: gh pr view --json reviewDecision
  |     |     -> current approval state
  |     |
  |     +-- diff against previous state (stored in state file)
  |     |
  |     +-- if any PR transitioned to APPROVED:
  |           -> terminal-notifier with PR title + URL
  |
  +-- state file (~/.local/state/gh-pr-notify/state.json)
        stores: { "envato/repo#123": "REVIEW_REQUIRED", ... }
```

## Notification

Using `terminal-notifier` (MIT, available via `brew install terminal-notifier`):

```sh
terminal-notifier \
  -title "PR Approved" \
  -subtitle "envato/author-finances" \
  -message "feat(ATH-1743): Dual-write W-form submissions" \
  -open "https://github.com/envato/author-finances/pull/1202" \
  -sound default \
  -group "gh-pr-notify"
```

Clicking the notification opens the PR in the browser. The `-group` flag deduplicates - won't stack multiple notifications for the same PR.

Fallback if `terminal-notifier` isn't installed:

```sh
osascript -e 'display notification "PR #1202 approved" with title "gh-pr-notify"'
```

`osascript` works without installs but can't open URLs on click and has limited customization.

## State Management

State file at `~/.local/state/gh-pr-notify/state.json` (XDG-compliant):

```json
{
  "envato/author-finances#1202": "REVIEW_REQUIRED",
  "envato/author-warehouse#1242": "CHANGES_REQUESTED"
}
```

On each poll:
1. Fetch current open PRs and their `reviewDecision`
2. Load previous state
3. For each PR: if previous != `APPROVED` and current == `APPROVED`, notify
4. Write current state (only open PRs - closed/merged PRs drop out naturally)

PRs that close or merge disappear from `gh search prs --state open`, so they get pruned from state automatically on next write.

## Rate Limiting

`gh search prs` = 1 API call.
`gh pr view` per PR = 1 GraphQL call each.

With 5 open PRs polling every 60 seconds: 6 calls/minute = 360 calls/hour. GitHub's authenticated rate limit is 5,000/hour for REST, 5,000 points/hour for GraphQL. Well within limits.

If someone has 20+ open PRs, consider batching with a single GraphQL query instead of N `gh pr view` calls. Out of scope for v1.

## Installation

```sh
# Dependencies
brew install gh terminal-notifier

# Script
cp gh-pr-notify ~/.local/bin/gh-pr-notify
chmod +x ~/.local/bin/gh-pr-notify

# launchd (auto-start on login, restarts on failure)
cp com.gh-pr-notify.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.gh-pr-notify.plist
```

## File Layout

```
gh-pr-notify/
  gh-pr-notify          # the script
  com.gh-pr-notify.plist  # launchd plist
  README.md             # usage instructions (later)
```

## Non-Goals

- Notifying on comments, CI status, merge conflicts, or anything other than approval
- Tracking PRs you're reviewing (only PRs you authored)
- Web UI, menu bar app, or system tray
- Supporting Linux/Windows (macOS only for now - the notification mechanism is platform-specific)
- Configurable poll interval via CLI flags (just edit the script or plist)

## Prior Art

| Tool | Why not use it |
|------|---------------|
| [Trailer](https://github.com/ptsochantaris/trailer) | Full notification manager. Menu bar app with iOS companion. Way more than needed. |
| [Gitify](https://github.com/gitify-app/gitify) | Electron app for all GitHub notifications. No approval-specific filtering. |
| [gh-notify-desktop](https://github.com/benelan/gh-notify-desktop) | Shell script, MIT. Sends desktop notifications for all GitHub notifications. Closest to what we want but no approval filtering - you'd get notified about everything. |
| [gh-notify](https://github.com/meiji163/gh-notify) | TUI viewer (325 stars). No push notifications - you run it interactively. |
| [reviewGOOSE](https://github.com/codeGROOVE-dev/goose) | Go menu bar app. Tracks PR review status but focused on showing who's blocking whom, not push notifications on approval. GPL-3.0. |
| [github-pr-monitor](https://github.com/whostolebenfrog/github-pr-monitor) | Go system tray app. Detects approvals but focused on "PRs you need to review". Zero stars, no license. |
| [Neat](https://neat.run/) | macOS notification manager. Closed source. Does too much. |
| [CatLight](https://catlight.io/) | Cross-platform desktop app. Commercial. Overkill. |
| [octobox](https://github.com/octobox/octobox) | Self-hosted Rails app for notification triage. Way too heavy. |
| [Patrick Desjardins' script](https://patrickdesjardins.com/blog/git-notification-macos-pull-request) | Python polling script. Similar idea but notifies on "needs review", not "got approved". |

## Open Questions

1. **Sound or no sound?** `terminal-notifier -sound default` plays a chime. Might be annoying if you have many PRs getting approved at once. Could make it configurable.
2. **Notify on CHANGES_REQUESTED too?** Useful to know when a reviewer requests changes, not just approves. Easy to add but increases noise.
3. **Log file?** Might want a log at `~/.local/state/gh-pr-notify/gh-pr-notify.log` for debugging launchd issues.
