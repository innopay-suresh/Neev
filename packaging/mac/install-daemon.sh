#!/bin/bash
# Install the Neev Remote privileged daemon set on macOS so the host survives
# lock / fast-user-switch and a viewer can see the login window — the macOS
# analog of the Windows SYSTEM-service TransportMode.
#
#   sudo ./install-daemon.sh /path/to/neev-agent [ws://relay:8080/ws]
#
# Layout it creates:
#   /Library/Application Support/NeevRemote/neev-agent   (the binary)
#   /Library/LaunchDaemons/com.neev.transport.plist      (root transport)
#   /Library/LaunchAgents/com.neev.worker.plist          (per-session + LoginWindow worker)
#
# After install you MUST grant the binary Screen Recording (and Accessibility for
# input) in System Settings → Privacy & Security. No TCC prompt can appear at the
# login window, so this one-time grant is required for lock-screen capture.
set -euo pipefail

AGENT_SRC="${1:-}"
RELAY_URL="${2:-ws://172.17.17.77:8080/ws}"
HERE="$(cd "$(dirname "$0")" && pwd)"

if [[ $EUID -ne 0 ]]; then
  echo "error: must run as root (use sudo)" >&2; exit 1
fi
if [[ -z "$AGENT_SRC" || ! -f "$AGENT_SRC" ]]; then
  echo "usage: sudo $0 /path/to/neev-agent [relay-url]" >&2; exit 1
fi

SUPPORT="/Library/Application Support/NeevRemote"
DAEMON_PLIST="/Library/LaunchDaemons/com.neev.transport.plist"
AGENT_PLIST="/Library/LaunchAgents/com.neev.worker.plist"

echo "==> installing binary to $SUPPORT/neev-agent"
mkdir -p "$SUPPORT"
install -m 0755 "$AGENT_SRC" "$SUPPORT/neev-agent"

echo "==> writing $DAEMON_PLIST (relay=$RELAY_URL)"
sed "s#__RELAY_URL__#${RELAY_URL}#g" "$HERE/com.neev.transport.plist" > "$DAEMON_PLIST"
chown root:wheel "$DAEMON_PLIST"; chmod 0644 "$DAEMON_PLIST"

echo "==> writing $AGENT_PLIST"
cp "$HERE/com.neev.worker.plist" "$AGENT_PLIST"
chown root:wheel "$AGENT_PLIST"; chmod 0644 "$AGENT_PLIST"

# Reload cleanly if already present (bootout is a no-op the first time).
echo "==> (re)loading services"
launchctl bootout system "$DAEMON_PLIST" 2>/dev/null || true
launchctl bootstrap system "$DAEMON_PLIST"
launchctl enable system/com.neev.transport

# The worker LaunchAgent loads per-GUI-session. Bootstrap it into the CURRENT
# console user's Aqua session now; the LoginWindow instance loads automatically
# at the login screen. gui/<uid> is the active user's session domain.
CONSOLE_UID="$(stat -f%u /dev/console)"
if [[ -n "$CONSOLE_UID" && "$CONSOLE_UID" != "0" ]]; then
  launchctl bootout "gui/$CONSOLE_UID" "$AGENT_PLIST" 2>/dev/null || true
  launchctl bootstrap "gui/$CONSOLE_UID" "$AGENT_PLIST" 2>/dev/null || true
fi

echo ""
echo "installed. NEXT: grant Screen Recording + Accessibility to:"
echo "  $SUPPORT/neev-agent"
echo "in System Settings → Privacy & Security, then log out/in once."
echo "transport id/password are written to $SUPPORT/transport.txt"
