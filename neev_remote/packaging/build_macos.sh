#!/usr/bin/env bash
# Builds Neev Remote for macOS and produces:
#   dist/NeevRemote-macos.zip   (portable .app, just unzip & run)
#   dist/NeevRemote-macos.dmg   (drag-to-Applications disk image)
#   dist/NeevRemote-macos.pkg   (installer package -> /Applications)
#
# Run on macOS with full Xcode + CocoaPods installed.
set -euo pipefail
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
pkgbuild --install-location /Applications \
  --identifier com.neev.neev_remote \
  --version 1.0.0 \
  --component "$APP_PATH" \
  "$OUT/NeevRemote-macos.pkg"

echo "==> done:"
ls -lh "$OUT"/NeevRemote-macos.*
