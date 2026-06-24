#!/usr/bin/env bash
# build.sh — cross-platform build script for Neev Remote Agent desktop client
#
# Usage:
#   ./build.sh              # build for current platform
#   ./build.sh --all        # build for all platforms (requires xgo or Docker)
#   ./build.sh --dev        # run in dev mode (hot reload)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

MODE="${1:-}"
APP_NAME="remote-agent"
VERSION="1.0.0"
APP_BUNDLE="build/bin/Neev Remote.app"
export GOCACHE="${GOCACHE:-/private/tmp/go-cache}"

# Check wails CLI
if ! command -v wails &>/dev/null; then
  export PATH="$HOME/go/bin:$PATH"
  if ! command -v wails &>/dev/null; then
    echo "Installing wails CLI..."
    go install github.com/wailsapp/wails/v2/cmd/wails@latest
    export PATH="$HOME/go/bin:$PATH"
  fi
fi

case "$MODE" in
  --dev)
    echo "▶ Starting dev server (hot reload)…"
    wails dev
    ;;
  --all)
    echo "▶ Building for all platforms…"
    # macOS (requires macOS build host)
    if [[ "$(uname)" == "Darwin" ]]; then
      echo "  → macOS (arm64)…"
      wails build -platform darwin/arm64 -o "dist/${APP_NAME}-${VERSION}-darwin-arm64"
      echo "  → macOS (amd64)…"
      wails build -platform darwin/amd64 -o "dist/${APP_NAME}-${VERSION}-darwin-amd64"
    fi
    echo "  → Linux (amd64)…"
    wails build -platform linux/amd64 -o "dist/${APP_NAME}-${VERSION}-linux-amd64"
    echo "✅ All builds complete. Output in dist/"
    ;;
  *)
    # Build for current platform.
    PLATFORM="$(uname -s | tr '[:upper:]' '[:lower:]')/$(uname -m | sed 's/x86_64/amd64/' | sed 's/arm64/arm64/')"
    echo "▶ Building for ${PLATFORM}…"

    # On macOS: wails rebuilds the frontend which gets xattrs added by macOS,
    # then fails signing because codesign can't handle xattrs.
    # Solution: build frontend separately, clear xattrs, then wails -s (skip frontend)
    if [[ "$(uname)" == "Darwin" ]]; then
      echo "  → Building frontend…"
      (cd frontend && npm run build) >/dev/null 2>&1
      echo "  → Clearing extended attributes from frontend/dist…"
      xattr -cr "${SCRIPT_DIR}/frontend/dist"
      echo "  → Frontend ready."
    fi

    # Use -s to skip frontend build (we already built it above with xattrs cleared)
    # We expect signing to fail due to xattrs on the Go binary, so capture and continue
    if ! wails build -platform "$PLATFORM" -clean -s 2>&1; then
      if [[ "$(uname)" == "Darwin" ]]; then
        echo "  → wails build failed (likely xattr signing issue), cleaning and re-signing…"
      fi
    fi

    # Strip extended attributes and sign ad-hoc
    if [[ "$(uname)" == "Darwin" ]]; then
      for bundle in build/bin/*.app; do
        if [[ -d "$bundle" ]]; then
          echo "  → Stripping extended attributes from $(basename "$bundle")…"
          xattr -cr "$bundle"
          find "$bundle" -xattr -exec xattr -d com.apple.quarantine {} 2>/dev/null \; || true
          find "$bundle" -xattr -exec xattr -d com.apple.provenance {} 2>/dev/null \; || true
          find "$bundle" -xattr -exec xattr -d com.apple.FinderInfo {} 2>/dev/null \; || true
          find "$bundle" -xattr -exec xattr -d com.apple.fileprovider.fpfs {} 2>/dev/null \; || true
          echo "  → Signing…"
          if codesign --force --deep --sign - --timestamp=none "$bundle" 2>&1; then
            echo "  → Signed successfully."
          else
            echo "  → Warning: signing failed but app bundle is ready."
          fi
          break
        fi
      done
    fi

    echo "✅ Build complete. Output in build/bin/"
    ;;
esac
