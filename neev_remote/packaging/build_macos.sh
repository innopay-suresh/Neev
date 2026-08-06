#!/usr/bin/env bash
# Builds Neev Remote for macOS and produces:
#   dist/NeevRemote-macos.zip   (portable .app, just unzip & run)
#   dist/NeevRemote-macos.dmg   (drag-to-Applications disk image)
#   dist/NeevRemote-macos.pkg   (installer package -> /Applications)
#
# Run on macOS with full Xcode + CocoaPods installed.
set -euo pipefail
# Resolved BEFORE the cd below: $0 is relative to the caller's working
# directory, so re-deriving it later silently points at the wrong place. That
# broke the macOS build for two releases.
SCRIPTS_DIR="$(cd "$(dirname "$0")" && pwd)/macos_scripts"
cd "$(dirname "$0")/.."

APP="Neev Remote"
OUT="dist"
mkdir -p "$OUT"

echo "==> flutter build macos --release"
RELAY_DEFINE=""
[ -n "${RELAY_URL:-}" ] && RELAY_DEFINE="--dart-define=RELAY_URL=$RELAY_URL"
flutter build macos --release $RELAY_DEFINE

APP_PATH="$(find build/macos/Build/Products/Release -maxdepth 1 -name '*.app' | head -1)"
[ -n "$APP_PATH" ] || { echo "build .app not found"; exit 1; }

# ---- Bundle the switch-user/lock-screen daemon payload into the .app so the app
# can install it with an admin prompt (the macOS analog of the Windows installer
# bundling neev-host.exe). CI builds neev-agent (darwin/arm64) at the repo root;
# skipped gracefully on local builds that didn't build the Go agent. ----
REPO_ROOT="$(cd .. && pwd)"
AGENT_BIN="$REPO_ROOT/neev-agent"
if [ -f "$AGENT_BIN" ]; then
  echo "==> bundling neev-agent + launchd payload into app Resources/daemon"
  DAEMON_DST="$APP_PATH/Contents/Resources/daemon"
  mkdir -p "$DAEMON_DST"
  install -m 0755 "$AGENT_BIN" "$DAEMON_DST/neev-agent"
  # Give the agent a STABLE code-sign identifier so Screen Recording / Accessibility
  # TCC grants stick to it (the linker's default 'a.out' identity is not durable).
  # Ad-hoc here; a Developer-ID re-sign downstream keeps the same identifier.
  codesign --force --sign - --identifier com.neev.agent --timestamp=none \
    "$DAEMON_DST/neev-agent" 2>/dev/null || echo "   (agent re-sign skipped)"
  # The host's macOS session controls (menu bar): "Remote session active",
  # a microphone toggle, and End session. Built here rather than checked in so
  # the shipped bundle always matches the source.
  # FATAL if it fails. This was a warning once, and a Swift error that only
  # appeared on the CI toolchain shipped a macOS package with no host controls
  # at all — while the release notes said the feature was in it. A red build is
  # recoverable in minutes; a package that quietly lacks an advertised feature
  # is not noticed until a user goes looking for it.
  if ! bash "$REPO_ROOT/packaging/mac/NeevVoice/build.sh" "$DAEMON_DST"; then
    echo "ERROR: NeevVoice.app failed to build — refusing to package a macOS"
    echo "       build with no session bar, microphone, or sound controls."
    exit 1
  fi
  echo "   bundled NeevVoice.app (host session controls)"
  # Prove it landed, rather than trusting the build script's exit code.
  if [ ! -x "$DAEMON_DST/NeevVoice.app/Contents/MacOS/NeevVoice" ]; then
    echo "ERROR: NeevVoice.app built but its binary is missing from the bundle."
    exit 1
  fi
  cp "$REPO_ROOT/packaging/mac/com.neev.transport.plist" "$DAEMON_DST/"
  cp "$REPO_ROOT/packaging/mac/com.neev.worker.plist" "$DAEMON_DST/"
  install -m 0755 "$REPO_ROOT/packaging/mac/install-daemon.sh" "$DAEMON_DST/"
  # Adding files invalidated the app signature flutter applied — re-seal ad-hoc so
  # the bundle stays consistent (proper Developer-ID signing happens downstream).
  codesign --force --sign - --timestamp=none \
    --entitlements macos/Runner/Release.entitlements "$APP_PATH" 2>/dev/null || \
    echo "   (ad-hoc re-sign skipped)"
else
  echo "==> neev-agent not found at $AGENT_BIN — skipping daemon bundle (app-only build)"
fi

echo "==> portable zip"
rm -f "$OUT/NeevRemote-macos.zip"
ditto -c -k --sequesterRsrc --keepParent "$APP_PATH" "$OUT/NeevRemote-macos.zip"

echo "==> dmg"
rm -f "$OUT/NeevRemote-macos.dmg"
STAGE="$(mktemp -d)"
cp -R "$APP_PATH" "$STAGE/"
ln -s /Applications "$STAGE/Applications"
hdiutil create -volname "$APP" -srcfolder "$STAGE" -ov -format UDZO \
  "$OUT/NeevRemote-macos.dmg"
rm -rf "$STAGE"

echo "==> pkg installer"
rm -f "$OUT/NeevRemote-macos.pkg"
# --scripts wires in a postinstall that installs the host daemon automatically.
# Without it the package dropped the app in /Applications and stopped, leaving
# TransportMode — user-switch and login-window hosting, and the host's session
# controls — to be found by hand in Settings. Anyone who missed it had an app
# that looked installed and silently lacked half its features.
if [ ! -x "$SCRIPTS_DIR/postinstall" ]; then
  echo "ERROR: $SCRIPTS_DIR/postinstall missing or not executable" >&2
  exit 1
fi
pkgbuild --install-location /Applications \
  --identifier com.neev.neev_remote \
  --version 1.0.0 \
  --scripts "$SCRIPTS_DIR" \
  --component "$APP_PATH" \
  "$OUT/NeevRemote-macos.pkg"

echo "==> done:"
ls -lh "$OUT"/NeevRemote-macos.*
