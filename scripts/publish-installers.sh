#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"

output_dir="${OUTPUT_DIR:-$repo_root/dist/packages}"
version="${VERSION:-1.0.0}"
target_os="${TARGET_OS:-$(uname -s | tr '[:upper:]' '[:lower:]')}"

mkdir -p "$output_dir"

case "$target_os" in
  darwin|mac|macos)
    agent_binary="${AGENT_BINARY:-$repo_root/bin_agent/agent}"
    if [[ ! -f "$agent_binary" ]]; then
      agent_binary="${AGENT_BINARY:-$repo_root/agent/remote-agent-mac}"
    fi
    if [[ ! -f "$agent_binary" ]]; then
      echo "No macOS agent binary found. Set AGENT_BINARY to a built agent executable." >&2
      exit 1
    fi
    OUTPUT_DIR="$output_dir" \
      VERSION="$version" \
      AGENT_BINARY="$agent_binary" \
      "$repo_root/scripts/release-packages.sh" darwin
    if bash "$repo_root/client/build.sh"; then
      desktop_bundle="$repo_root/client/build/bin/neev-remote.app"
    else
      echo "⚠ Desktop controller build failed; continuing with agent package only." >&2
      desktop_bundle=""
    fi
    desktop_dmg="$output_dir/NeevRemote-Desktop-macOS.dmg"
    desktop_zip="$output_dir/NeevRemote-Desktop-macOS.zip"
    if [[ -n "$desktop_bundle" && -d "$desktop_bundle" ]]; then
      # Primary: Build a polished DMG with drag-to-Applications layout
      echo "▶ Building macOS DMG installer…"
      VERSION="$version" "$repo_root/packaging/mac/build-dmg.sh" "$desktop_bundle" "$desktop_dmg"

      # Secondary: Also build a zip fallback with Install.command
      echo "▶ Building macOS zip fallback…"
      staging_dir="$(mktemp -d /tmp/remote-agent-desktop.XXXXXX)"
      staging_inner="$staging_dir/NeevRemote-Desktop-macOS"
      mkdir -p "$staging_inner"
      cp -R "$desktop_bundle" "$staging_inner/"
      cp "$repo_root/packaging/mac/install-mac-app.sh" "$staging_inner/Install.command"
      chmod +x "$staging_inner/Install.command"
      xattr -cr "$staging_inner" 2>/dev/null || true
      if command -v ditto >/dev/null 2>&1; then
        ditto -c -k --sequesterRsrc --keepParent "$staging_inner" "$desktop_zip"
      else
        rm -f "$desktop_zip"
        (cd "$staging_dir" && zip -qry "$desktop_zip" "NeevRemote-Desktop-macOS")
      fi
      rm -rf "$staging_dir"
    else
      echo "⚠ Skipping desktop controller artifacts; bundle not available." >&2
    fi
    ;;
  linux)
    agent_binary="${AGENT_BINARY:-$repo_root/dist/linux/remote-agent}"
    if [[ ! -f "$agent_binary" ]]; then
      agent_binary="${AGENT_BINARY:-$repo_root/agent/remote-agent}"
    fi
    if [[ ! -f "$agent_binary" ]]; then
      echo "No Linux agent binary found. Set AGENT_BINARY to a built agent executable." >&2
      exit 1
    fi
    OUTPUT_DIR="$output_dir" \
      VERSION="$version" \
      AGENT_BINARY="$agent_binary" \
      "$repo_root/scripts/release-packages.sh" linux
    ;;
  *)
    echo "Unsupported host OS: $target_os" >&2
    exit 1
    ;;
esac

echo "Published installers to $output_dir"
