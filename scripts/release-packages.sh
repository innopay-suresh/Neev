#!/usr/bin/env bash
set -euo pipefail

target_os="${1:-$(uname -s | tr '[:upper:]' '[:lower:]')}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"

version="${VERSION:-1.0.0}"
relay_url="${RELAY_URL:-ws://localhost:8080/ws}"
enrollment_code="${ENROLLMENT_CODE:-}"
org_id="${ORG_ID:-}"
device_group="${DEVICE_GROUP:-}"
turn_url="${TURN_URL:-}"
turn_user="${TURN_USER:-agent}"
turn_pass="${TURN_PASS:-changeme}"
output_dir="${OUTPUT_DIR:-$repo_root/dist/packages}"

mkdir -p "$output_dir"

case "$target_os" in
  linux)
    agent_binary="${AGENT_BINARY:-$repo_root/agent/remote-agent}"
    architecture="${ARCH:-$(dpkg --print-architecture 2>/dev/null || uname -m)}"
    output_file="$output_dir/remote-agent_${version}_${architecture}.deb"
    RELAY_URL="$relay_url" \
      ENROLLMENT_CODE="$enrollment_code" \
      ORG_ID="$org_id" \
      DEVICE_GROUP="$device_group" \
      TURN_URL="$turn_url" \
      TURN_USER="$turn_user" \
      TURN_PASS="$turn_pass" \
      VERSION="$version" \
      OUTPUT_DEB="$output_file" \
      "$repo_root/packaging/linux/build-deb.sh" "$agent_binary"
    ;;
  darwin|mac|macos)
    agent_binary="${AGENT_BINARY:-$repo_root/agent/remote-agent-mac}"
    output_file="$output_dir/RemoteAgent-${version}.pkg"
    RELAY_URL="$relay_url" \
      ENROLLMENT_CODE="$enrollment_code" \
      ORG_ID="$org_id" \
      DEVICE_GROUP="$device_group" \
      TURN_URL="$turn_url" \
      TURN_USER="$turn_user" \
      TURN_PASS="$turn_pass" \
      VERSION="$version" \
      OUTPUT_PKG="$output_file" \
      "$repo_root/packaging/mac/build-pkg.sh" "$agent_binary"
    ;;
  *)
    echo "Unsupported target OS: $target_os" >&2
    exit 1
    ;;
esac
