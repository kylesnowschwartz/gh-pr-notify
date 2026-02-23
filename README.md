# gh-pr-notify

Desktop notifications when your GitHub PRs get approved. Nothing else.

GitHub's notification system is noisy -- comments, CI, mentions, review requests all compete for attention. When a PR gets approved, that signal drowns. This tool watches your open PRs and pings you when one gets the green tick.

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
| `--sound` | `default` | macOS notification sound (`none` to disable) |
| `--bark-key` | | Bark device key for iOS push notifications |
| `--bark-server` | `https://api.day.app` | Bark server URL |
| `--bark-sound` | | Bark notification sound name |
| `--version` | | Print version and exit |

## How it works

1. Polls `gh search prs --author @me --state open` to find your open PRs
2. Checks `reviewDecision` for each via `gh pr view`
3. Compares against previous state in `~/.local/state/gh-pr-notify/state.json`
4. Sends a desktop notification when any PR transitions to APPROVED

Closed and merged PRs drop out of state automatically.

## Files

| Path | Purpose |
|------|---------|
| `~/.local/bin/gh-pr-notify` | Binary |
| `~/.local/state/gh-pr-notify/state.json` | PR review state |
| `~/.local/state/gh-pr-notify/gh-pr-notify.log` | Service logs |
| `~/Library/LaunchAgents/com.gh-pr-notify.plist` | launchd config |

## Uninstall

```sh
curl -fsSL https://raw.githubusercontent.com/kylesnowschwartz/gh-pr-notify/main/install.sh | sh -s -- --uninstall
```

## License

MIT
