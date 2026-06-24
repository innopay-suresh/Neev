#!/bin/bash
# Install.command — Double-click this file to install RemoteAgent to /Applications
#
# This script:
#   1. Removes the macOS quarantine attribute (so Gatekeeper doesn't block it)
#   2. Copies RemoteAgent.app to /Applications
#   3. Re-signs the app ad-hoc
#   4. Launches it

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_NAME="RemoteAgent.app"
SOURCE_APP="$SCRIPT_DIR/$APP_NAME"
DEST_APP="/Applications/$APP_NAME"

if [ ! -d "$SOURCE_APP" ]; then
  echo "❌ Error: Cannot find $APP_NAME next to this script."
  echo "   Make sure RemoteAgent.app is in the same folder as Install.command."
  echo ""
  read -n 1 -s -r -p "Press any key to close..."
  exit 1
fi

echo "╔══════════════════════════════════════════════╗"
echo "║   RemoteAgent Desktop — macOS Installer      ║"
echo "╚══════════════════════════════════════════════╝"
echo ""

# Step 1: Remove quarantine attribute
echo "▶ Removing quarantine attribute…"
xattr -cr "$SOURCE_APP" 2>/dev/null || true

# Step 2: Copy to /Applications
echo "▶ Installing to /Applications…"
if [ -d "$DEST_APP" ]; then
  echo "  → Removing existing installation…"
  rm -rf "$DEST_APP"
fi
cp -R "$SOURCE_APP" "$DEST_APP"

# Step 3: Remove quarantine from installed copy too
xattr -cr "$DEST_APP" 2>/dev/null || true

# Step 4: Ad-hoc code sign
if command -v codesign >/dev/null 2>&1; then
  echo "▶ Signing application…"
  codesign --force --deep --sign - --timestamp=none "$DEST_APP" >/dev/null 2>&1 || true
fi

# Step 5: Launch the app
echo "▶ Launching RemoteAgent…"
open "$DEST_APP"

echo ""
echo "✅ RemoteAgent has been installed to /Applications and launched."
echo "   You can find it in Launchpad and Spotlight."
echo ""
read -n 1 -s -r -p "Press any key to close this window..."
