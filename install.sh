#!/bin/sh
set -eu

# gh-pr-notify installer
# Usage: curl -fsSL https://raw.githubusercontent.com/kylesnowschwartz/gh-pr-notify/main/install.sh | sh

REPO="kylesnowschwartz/gh-pr-notify"
INSTALL_DIR="${HOME}/.local/bin"
STATE_DIR="${HOME}/.local/state/gh-pr-notify"
PLIST_NAME="com.gh-pr-notify.plist"
PLIST_DIR="${HOME}/Library/LaunchAgents"

info() { printf '  %s\n' "$@"; }
error() {
  printf '  ERROR: %s\n' "$@" >&2
  exit 1
}

# --- Pre-flight checks ---

if [ "$(uname -s)" != "Darwin" ]; then
  error "gh-pr-notify only supports macOS"
fi

if ! command -v gh >/dev/null 2>&1; then
  error "gh CLI not found. Install it: brew install gh"
fi

if ! gh auth status >/dev/null 2>&1; then
  error "gh CLI not authenticated. Run: gh auth login"
fi

# --- Get the binary ---

install_binary() {
  mkdir -p "$INSTALL_DIR"

  # Try downloading a pre-built binary from the latest GitHub release.
  if try_download_release; then
    return 0
  fi

  # Fall back to building from source.
  if command -v go >/dev/null 2>&1; then
    build_from_source
    return 0
  fi

  error "No pre-built release found and Go is not installed. Install Go: brew install go"
}

try_download_release() {
  arch="$(uname -m)"
  case "$arch" in
  arm64) arch="arm64" ;;
  x86_64) arch="amd64" ;;
  *) return 1 ;;
  esac

  asset="gh-pr-notify-darwin-${arch}"
  url="https://github.com/${REPO}/releases/latest/download/${asset}"

  info "Checking for pre-built binary..."
  if curl -fsSL --head "$url" >/dev/null 2>&1; then
    info "Downloading ${asset}..."
    curl -fsSL -o "${INSTALL_DIR}/gh-pr-notify" "$url"
    chmod +x "${INSTALL_DIR}/gh-pr-notify"
    return 0
  fi

  info "No pre-built release found, building from source..."
  return 1
}

build_from_source() {
  info "Building from source..."
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT

  git clone --depth 1 "https://github.com/${REPO}.git" "$tmpdir/gh-pr-notify" 2>/dev/null
  (cd "$tmpdir/gh-pr-notify" && go build -o "${INSTALL_DIR}/gh-pr-notify" .)
  info "Built successfully"
}

# --- Install launchd plist ---

install_plist() {
  mkdir -p "$PLIST_DIR" "$STATE_DIR"

  cat >"${PLIST_DIR}/${PLIST_NAME}" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.gh-pr-notify</string>
	<key>ProgramArguments</key>
	<array>
		<string>${INSTALL_DIR}/gh-pr-notify</string>
	</array>
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
	</dict>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>${STATE_DIR}/gh-pr-notify.log</string>
	<key>StandardErrorPath</key>
	<string>${STATE_DIR}/gh-pr-notify.log</string>
</dict>
</plist>
PLIST
}

# --- Start the service ---

start_service() {
  # Stop existing instance if running.
  launchctl unload "${PLIST_DIR}/${PLIST_NAME}" 2>/dev/null || true

  launchctl load "${PLIST_DIR}/${PLIST_NAME}"
}

# --- Uninstall ---

uninstall() {
  info "Uninstalling gh-pr-notify..."

  launchctl unload "${PLIST_DIR}/${PLIST_NAME}" 2>/dev/null || true
  rm -f "${PLIST_DIR}/${PLIST_NAME}"
  rm -f "${INSTALL_DIR}/gh-pr-notify"
  rm -rf "$STATE_DIR"

  info "Uninstalled"
  exit 0
}

# --- Main ---

if [ "${1:-}" = "--uninstall" ]; then
  uninstall
fi

info "Installing gh-pr-notify..."
info ""

install_binary
install_plist
start_service

info ""
info "Installed and running."
info "  Binary:  ${INSTALL_DIR}/gh-pr-notify"
info "  Logs:    ${STATE_DIR}/gh-pr-notify.log"
info "  Config:  ${PLIST_DIR}/${PLIST_NAME}"
info ""
info "Polls every 60s. You'll get a notification when a PR is approved."
info "To uninstall: curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | sh -s -- --uninstall"
