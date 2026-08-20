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

# The agent's bundled libraries MUST travel with it.
#
# They are referenced as @loader_path/<name>, i.e. resolved next to the binary
# wherever it ends up — so copying the binary alone leaves dyld with nothing to
# resolve, and it kills the process before main(). launchd then crash-loops it
# forever with `last exit reason = OS_REASON_DYLD`: an installed, correctly
# signed daemon that never starts, which is the same end state as the Homebrew
# path bug these libraries were bundled to fix.
#
# FATAL when a library is missing, rather than installing a daemon that cannot
# possibly run. That failure is invisible from the outside — the app quietly
# falls back to hosting itself, and what the user sees is a blank share card and
# a viewer stuck at "Requesting the device".
AGENT_DIR="$(dirname "$AGENT_SRC")"
shopt -s nullglob
DYLIBS=("$AGENT_DIR"/*.dylib)
shopt -u nullglob
if [ ${#DYLIBS[@]} -gt 0 ]; then
  echo "==> installing ${#DYLIBS[@]} bundled librar$([ ${#DYLIBS[@]} -eq 1 ] && echo y || echo ies) to $SUPPORT"
  for lib in "${DYLIBS[@]}"; do
    install -m 0644 "$lib" "$SUPPORT/$(basename "$lib")"
  done
fi
# Verify against what the binary actually asks for, not against what we happened
# to copy: a library added later must not be able to go missing silently.
MISSING=""
while read -r ref; do
  case "$ref" in
    @loader_path/*) ;;
    *) continue ;;
  esac
  name="${ref#@loader_path/}"
  [ -f "$SUPPORT/$name" ] || MISSING="$MISSING $name"
done < <(otool -L "$SUPPORT/neev-agent" 2>/dev/null | tail -n +2 | awk '{print $1}')
if [ -n "$MISSING" ]; then
  echo "error: the agent needs these libraries beside it and they are not in" >&2
  echo "       the payload:$MISSING" >&2
  echo "       Refusing to install a daemon that cannot start." >&2
  exit 1
fi

# The menu-bar session controls live NEXT TO the agent, because the worker
# resolves them relative to its own executable. Without this a macOS host has
# no way to end a session or to speak to the viewer.
VOICE_SRC="$(dirname "$AGENT_SRC")/NeevVoice.app"
if [ -d "$VOICE_SRC" ]; then
  echo "==> installing session controls to $SUPPORT/NeevVoice.app"
  rm -rf "$SUPPORT/NeevVoice.app"
  cp -R "$VOICE_SRC" "$SUPPORT/NeevVoice.app"
else
  echo "==> NeevVoice.app not found beside the agent — this host will have no"
  echo "    session bar and no microphone control"
fi

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
