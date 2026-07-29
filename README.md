# gh-pr-notify

Desktop notifications when your GitHub PRs get approved or merged. Nothing else.

GitHub's notification system is noisy -- comments, CI, mentions, review requests all compete for attention. The two moments you actually care about, approval and merge, drown in the rest. This tool watches your open PRs and pings you on each.

Merges are tracked separately from approvals because plenty of PRs never get an approval at all. On repos with no required-review rule -- most open source -- a maintainer just merges, and the only signal is that the PR is gone.

macOS only. Runs as a background service via launchd.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/kylesnowschwartz/gh-pr-notify/main/install.sh | sh
```

Requires [gh CLI](https://cli.github.com/) (`brew install gh`) and an authenticated session (`gh auth login`).

Downloads a pre-built binary or builds from source if Go is installed. Sets up a launchd service that starts on login and restarts on failure.

### With iOS push notifications (Bark)

```sh
BARK_KEY=your-device-key curl -fsSL https://raw.githubusercontent.com/kylesnowschwartz/gh-pr-notify/main/install.sh | sh
```

[Bark](https://github.com/Finb/Bark) sends push notifications to your iPhone. Get your device key from the Bark app.

### Disable sound

```sh
SOUND=none curl -fsSL https://raw.githubusercontent.com/kylesnowschwartz/gh-pr-notify/main/install.sh | sh
```

## Usage

The service runs automatically after install, polling every 60 seconds.

```sh
# Run manually
gh-pr-notify

# Custom poll interval
gh-pr-notify --interval 30s

# Silent notifications
gh-pr-notify --sound none

# Custom macOS sound (Basso, Blow, Bottle, Frog, Funk, Glass, Hero, Morse,
# Ping, Pop, Purr, Sosumi, Submarine, Tink)
gh-pr-notify --sound Submarine

# With Bark push notifications
gh-pr-notify --bark-key YOUR_DEVICE_KEY
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--interval` | `60s` | Poll interval (e.g. `30s`, `2m`) |
| `--heartbeat` | `1h` | How often to log a poll that had nothing to report |
| `--log-file` | stderr | Append logs to this file, rotating as it fills |
| `--log-max-bytes` | `4194304` | Rotate `--log-file` once it exceeds this size |
| `--sound` | `default` | macOS notification sound (`none` to disable) |
| `--bark-key` | | Bark device key for iOS push notifications |
| `--bark-server` | `https://api.day.app` | Bark server URL |
| `--bark-sound` | | Bark notification sound name |
| `--version` | | Print version and exit |

## How it works

1. Polls `gh search prs --author @me --state open` to find your open PRs
2. Reads `reviewDecision` and the submitted reviews for each via `gh pr view`
3. Compares against previous state in `~/.local/state/gh-pr-notify/state.json`
4. Notifies when a PR gains approval, and when a PR that was open on the last poll turns out to be merged

### Reading approval

`reviewDecision` on its own misses approvals. Repos without a required-review rule leave it empty even after someone approves, so an `APPROVED` entry in the reviews array counts on its own. A `CHANGES_REQUESTED` decision wins over an earlier approval, and withdrawn approvals arrive as `DISMISSED` rather than `APPROVED`, so neither triggers a notification.

### Detecting merges

A PR missing from the open list is looked up with `gh pr view` before anything is announced:

- `MERGED` -- notify, then stop tracking it
- `CLOSED` -- stop tracking it, say nothing
- still `OPEN`, or the lookup failed -- keep it and settle it on a later poll

Absence alone never counts as a merge. A truncated or failed search would otherwise announce merges that never happened. This also covers a PR approved and merged inside a single poll interval, which is never visible as open-and-approved.

### Logging

An uneventful poll every 60 seconds fills the log with nothing. A summary line is written only when something is notified, when the open-PR count changes, or once per `--heartbeat` interval.

The service passes `--log-file` so gh-pr-notify owns and rotates its own log, keeping one previous generation at `gh-pr-notify.log.1`. Disk use is capped at twice `--log-max-bytes`. launchd's own redirect cannot rotate -- it holds the file open for the life of the process -- so it points at `launchd.log` and catches only startup failures.

Run in a terminal without `--log-file` and logs go to stderr as usual.

## Files

| Path | Purpose |
|------|---------|
| `~/.local/bin/gh-pr-notify` | Binary |
| `~/.local/state/gh-pr-notify/state.json` | PR review state |
| `~/.local/state/gh-pr-notify/gh-pr-notify.log` | Service logs, rotated |
| `~/.local/state/gh-pr-notify/gh-pr-notify.log.1` | Previous log generation |
| `~/.local/state/gh-pr-notify/launchd.log` | launchd startup failures |
| `~/Library/LaunchAgents/com.gh-pr-notify.plist` | launchd config |

## Uninstall

```sh
curl -fsSL https://raw.githubusercontent.com/kylesnowschwartz/gh-pr-notify/main/install.sh | sh -s -- --uninstall
```

## License

MIT
