#!/usr/bin/env bash
# build-dmg.sh — Create a polished macOS .dmg installer with drag-to-Applications layout
#
# Usage:
#   ./build-dmg.sh /path/to/RemoteAgent.app [output.dmg]
#
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"

APP_BUNDLE="${1:-$repo_root/client/build/bin/RemoteAgent.app}"
VERSION="${VERSION:-1.0.0}"
OUTPUT_DMG="${2:-$script_dir/RemoteAgent-Desktop-${VERSION}.dmg}"
VOLUME_NAME="RemoteAgent Desktop"
DMG_SIZE="30m"

if [[ ! -d "$APP_BUNDLE" ]]; then
  echo "Error: App bundle not found: $APP_BUNDLE" >&2
  exit 1
fi

echo "▶ Building DMG installer for $(basename "$APP_BUNDLE")…"

# ── Staging ──────────────────────────────────────────────────────────────────
staging_dir="$(mktemp -d /tmp/remote-agent-dmg.XXXXXX)"
trap 'rm -rf "$staging_dir"' EXIT

dmg_source="$staging_dir/dmg_root"
tmp_dmg="$staging_dir/tmp.dmg"
mkdir -p "$dmg_source"

# Copy app bundle - use ditto to strip resource forks and xattrs
bundle_name=$(basename "$APP_BUNDLE")
ditto --norsrc "$APP_BUNDLE" "$dmg_source/$bundle_name"

# Remove ALL extended attributes to prevent code signing failures
# and Gatekeeper "damaged app" errors
bundle_path="$dmg_source/$bundle_name"
xattr -cr "$bundle_path" 2>/dev/null || true
find "$bundle_path" -xattr -exec xattr -d com.apple.FinderInfo {} 2>/dev/null \; || true
find "$bundle_path" -xattr -exec xattr -d com.apple.quarantine {} 2>/dev/null \; || true
find "$bundle_path" -xattr -exec xattr -d com.apple.provenance {} 2>/dev/null \; || true
find "$bundle_path" -xattr -exec xattr -d com.apple.fileprovider.fpfs {} 2>/dev/null \; || true

# Ad-hoc code sign the copy
if command -v codesign >/dev/null 2>&1; then
  echo "  → Ad-hoc signing…"
  codesign --force --deep --sign - --timestamp=none "$bundle_path" 2>&1 || echo "  ⚠ Signing failed, continuing anyway"
fi

# Create Applications symlink for drag-and-drop install
ln -s /Applications "$dmg_source/Applications"

# ── Create temporary DMG ─────────────────────────────────────────────────────
echo "  → Creating temporary disk image…"
hdiutil create \
  -srcfolder "$dmg_source" \
  -volname "$VOLUME_NAME" \
  -fs HFS+ \
  -fsargs "-c c=64,a=16,e=16" \
  -format UDRW \
  -size "$DMG_SIZE" \
  "$tmp_dmg" >/dev/null

# ── Mount and customize ──────────────────────────────────────────────────────
echo "  → Mounting and configuring layout…"
device=$(hdiutil attach -readwrite -noverify -noautoopen "$tmp_dmg" | \
  egrep '^/dev/' | sed 1q | awk '{print $1}')

# Wait for volume to mount
sleep 1

# Set Finder window appearance via AppleScript
osascript <<EOF || true
tell application "Finder"
  tell disk "$VOLUME_NAME"
    open
    set current view of container window to icon view
    set toolbar visible of container window to false
    set statusbar visible of container window to false
    set bounds of container window to {100, 100, 640, 400}
    set theViewOptions to the icon view options of container window
    set arrangement of theViewOptions to not arranged
    set icon size of theViewOptions to 80
    -- Position the app icon and Applications symlink
    set position of item "$(basename "$APP_BUNDLE")" of container window to {140, 150}
    set position of item "Applications" of container window to {400, 150}
    update without registering applications
    delay 1
    close
  end tell
end tell
EOF

# Ensure changes are flushed
sync

# ── Detach and convert to compressed DMG ──────────────────────────────────────
echo "  → Finalizing compressed DMG…"
hdiutil detach "$device" >/dev/null 2>&1 || true
sleep 1

rm -f "$OUTPUT_DMG"
hdiutil convert "$tmp_dmg" \
  -format UDZO \
  -imagekey zlib-level=9 \
  -o "$OUTPUT_DMG" >/dev/null

echo "✅ DMG created: $OUTPUT_DMG"
echo "   Size: $(du -h "$OUTPUT_DMG" | cut -f1)"
