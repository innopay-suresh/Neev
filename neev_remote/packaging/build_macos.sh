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

# The pkg version MUST change between releases.
#
# It was hardcoded to 1.0.0, so every build claimed to be the same version. With
# a receipt already present, `installer` then treated an install as a no-op
# upgrade: it printed "The upgrade was successful" and wrote NOTHING. Deleting
# the app did not help either, because the stale receipt still claimed it was
# installed — the machine ended up with a receipt, no app, and a postinstall
# reporting it could not find the bundle it was supposed to configure.
#
# Derived from the build tag (r161-... -> 161) so it rises with every release.
PKG_VERSION="$(sed -n 's/.*buildTag = .*[·-] *r\([0-9][0-9]*\).*/\1/p' \
  "$(cd "$(dirname "$0")/.." && pwd)/lib/core/constants/app_constants.dart" | head -1)"
if [ -z "$PKG_VERSION" ]; then
  echo "ERROR: could not derive a pkg version from buildTag — refusing to build a" >&2
  echo "       package that installer may treat as already installed." >&2
  exit 1
fi
PKG_VERSION="1.0.$PKG_VERSION"
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

  # Make the agent self-contained.
  #
  # It is built on a CI runner with Homebrew, so cgo linked it against
  # /opt/homebrew/... dylibs. Those paths do not exist on a user's Mac: dyld
  # cannot resolve them, kills the process before main(), and launchd
  # crash-loops it forever with `last exit reason = OS_REASON_DYLD` — an
  # installed, correctly signed daemon that can never start. It read as
  # machine-specific because a dev Mac with Homebrew runs it fine.
  #
  # Copy every non-system dylib in beside the binary and repoint it at
  # @loader_path. Transitive deps are followed, so this stays correct if a
  # dependency ever gains one of its own. MUST run BEFORE signing: rewriting
  # load commands invalidates a signature.
  bundle_dylibs() {
    local target="$1" dir; dir="$(dirname "$target")"
    local dep name
    while read -r dep; do
      case "$dep" in
        /opt/homebrew/*|/usr/local/*) ;;
        *) continue ;;
      esac
      name="$(basename "$dep")"
      if [ ! -f "$dir/$name" ]; then
        install -m 0644 "$dep" "$dir/$name"
        chmod u+w "$dir/$name"
        install_name_tool -id "@loader_path/$name" "$dir/$name" 2>/dev/null || true
        bundle_dylibs "$dir/$name"   # follow this library's own deps
      fi
      install_name_tool -change "$dep" "@loader_path/$name" "$target" 2>/dev/null || true
    done < <(otool -L "$target" | tail -n +2 | awk '{print $1}')
  }
  bundle_dylibs "$DAEMON_DST/neev-agent"

  # Refuse to ship a daemon that cannot start on a machine without Homebrew.
  if otool -L "$DAEMON_DST/neev-agent" | grep -qE "/opt/homebrew|/usr/local"; then
    echo "ERROR: neev-agent still references build-machine dylibs:" >&2
    otool -L "$DAEMON_DST/neev-agent" | grep -E "/opt/homebrew|/usr/local" >&2
    exit 1
  fi
  # Sign with a STABLE identity so the machine's Screen Recording grant survives
  # updates. An identifier alone is not enough: without an explicit designated
  # requirement, codesign pins to the binary's cdhash, which changes every build
  # and silently invalidates the grant while System Settings still shows it
  # enabled. See packaging/mac/sign-stable.sh.
  # Sign the bundled dylibs FIRST. Under the hardened runtime an unsigned
  # dependency is refused at load time, which would swap one dyld failure for
  # another. Signing them after the agent would also invalidate the agent's own
  # signature, so order matters here.
  for lib in "$DAEMON_DST"/*.dylib; do
    [ -e "$lib" ] || continue
    bash "$REPO_ROOT/packaging/mac/sign-stable.sh" \
      "com.neev.agent.$(basename "$lib" .dylib)" "$lib"
  done
  bash "$REPO_ROOT/packaging/mac/sign-stable.sh" com.neev.agent "$DAEMON_DST/neev-agent"
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
# Relocation MUST be off, and only a component plist turns it off.
#
# By default pkgbuild marks every app bundle it finds as relocatable, which
# emits <relocate><bundle id="com.neev.neevRemote"/></relocate> in PackageInfo.
# At install time the installer then asks LaunchServices where that bundle id
# already lives and writes the payload THERE, ignoring --install-location. On a
# machine that had ever held an older copy — including one sitting in the Trash —
# the app was written to that stale path, so /Applications stayed empty and the
# postinstall could not find what it had just installed. A Mac that had never
# seen the app before installed fine, which is exactly what made this look
# machine-specific for several releases.
#
# --analyze produces the component plist; forcing BundleIsRelocatable=false in
# it drops the <relocate> element, so --install-location is honoured
# unconditionally. Note that --root vs --component was never the deciding
# factor: both emit <upgrade-bundle>, and that element alone is harmless.
PKG_ROOT="$(mktemp -d)"
cp -R "$APP_PATH" "$PKG_ROOT/"
COMPONENT_PLIST="$(mktemp -t neev-component).plist"
pkgbuild --analyze --root "$PKG_ROOT" "$COMPONENT_PLIST"
/usr/bin/python3 - "$COMPONENT_PLIST" <<'PY'
import plistlib
import sys

path = sys.argv[1]
with open(path, "rb") as fh:
    comps = plistlib.load(fh)
for comp in comps:
    comp["BundleIsRelocatable"] = False
with open(path, "wb") as fh:
    plistlib.dump(comps, fh)
PY
pkgbuild --root "$PKG_ROOT" \
  --component-plist "$COMPONENT_PLIST" \
  --install-location /Applications \
  --identifier com.neev.neev_remote \
  --version "$PKG_VERSION" \
  --scripts "$SCRIPTS_DIR" \
  "$OUT/NeevRemote-macos.pkg"
rm -rf "$PKG_ROOT" "$COMPONENT_PLIST"

echo "==> done:"
ls -lh "$OUT"/NeevRemote-macos.*
