#!/bin/bash
# Builds NeevVoice.app — the host's macOS session controls (menu bar).
#
# Plain swiftc rather than an Xcode project: this is one source file with no
# resources, and a .xcodeproj would be another thing to keep in sync with CI.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
OUT="${1:-$HERE/build}"
APP="$OUT/NeevVoice.app"

rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS"
cp "$HERE/Info.plist" "$APP/Contents/Info.plist"

# arm64 + x86_64 so one bundle serves both Apple Silicon and Intel hosts.
swiftc -O \
  -target arm64-apple-macos11.0 \
  -o "$OUT/NeevVoice-arm64" "$HERE/main.swift" "$HERE/sysaudio.swift"
swiftc -O \
  -target x86_64-apple-macos11.0 \
  -o "$OUT/NeevVoice-x86_64" "$HERE/main.swift" "$HERE/sysaudio.swift"
lipo -create -output "$APP/Contents/MacOS/NeevVoice" \
  "$OUT/NeevVoice-arm64" "$OUT/NeevVoice-x86_64"
rm -f "$OUT/NeevVoice-arm64" "$OUT/NeevVoice-x86_64"

# Strip extended attributes first: codesign refuses a bundle carrying resource
# forks or Finder metadata, which files picked up from a copy often have.
xattr -cr "$APP" 2>/dev/null || true

# Ad-hoc sign so the bundle has a stable identity for TCC. Without a signature
# macOS treats each rebuild as a different app and re-asks for the microphone.
# --identifier pins the bundle ID; otherwise the signature inherits the lipo
# input's name and the identity changes shape between builds.
codesign --force --sign - --timestamp=none \
  --identifier com.neev.remote.voice "$APP" || \
  echo "warning: codesign failed — TCC will re-prompt on every update"

echo "built $APP"
