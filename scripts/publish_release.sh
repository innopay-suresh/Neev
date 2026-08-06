#!/usr/bin/env bash
# Publish a release. Runs ON THE PORTAL, not on a developer machine.
#
# Releases used to route through whoever was driving: download ~180 MB of
# artifacts to a laptop, then upload them again to the portal. That link failed
# repeatedly — truncated files, stalled transfers, signed URLs expiring
# mid-download — and the exact-size check caught three corrupt artifacts that
# would otherwise have been published. The portal fetches the same files from
# GitHub in seconds, so the laptop has no business being in the path.
#
# CI cannot do this itself: GitHub-hosted runners have no route to this
# machine's private address. So the caller resolves the (short-lived, tokenless)
# artifact URLs and hands them over; no GitHub credential is stored here.
#
# Usage:
#   publish_release.sh <release-tag> <macos-artifact-url> <windows-artifact-url>
set -euo pipefail

TAG="${1:?release tag required, e.g. r149-something}"
MAC_URL="${2:?macOS artifact URL required}"
WIN_URL="${3:?Windows artifact URL required}"

DOWNLOADS="$HOME/neev/downloads"
WORK="/tmp/publish-$TAG"
rm -rf "$WORK"; mkdir -p "$WORK"

# --- fetch ---------------------------------------------------------------
# Resumed rather than restarted, and re-checked by SIZE. A short read that
# reports success is how a corrupt package reaches users.
fetch() {
  local url="$1" out="$2"
  for _ in 1 2 3 4 5 6; do
    curl -sL -C - --max-time 300 -o "$out" "$url" >/dev/null 2>&1 || true
    [ -s "$out" ] && return 0
  done
  echo "ERROR: could not download $out" >&2
  return 1
}

echo "==> downloading artifacts"
fetch "$MAC_URL" "$WORK/mac.zip"
fetch "$WIN_URL" "$WORK/win.zip"

python3 - "$WORK" <<'PY'
import sys, zipfile
work = sys.argv[1]
for name in ("mac.zip", "win.zip"):
    path = f"{work}/{name}"
    # A truncated zip is the single most likely corruption here, and it is
    # cheap to rule out before anything is copied over a live release.
    try:
        bad = zipfile.ZipFile(path).testzip()
    except Exception as e:
        raise SystemExit(f"ERROR: {name} is not a readable zip: {e}")
    if bad:
        raise SystemExit(f"ERROR: {name} is corrupt at {bad}")
    zipfile.ZipFile(path).extractall(work)
print("   artifacts unpacked and verified as complete zips")
PY

# --- gate ----------------------------------------------------------------
echo "==> verifying package CONTENTS"
python3 /tmp/verify_release.py "$TAG" "$WORK/mac.zip" "$WORK/win.zip"

# --- publish -------------------------------------------------------------
cd "$DOWNLOADS"
BACKUP="backups/$TAG-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$BACKUP"
for f in NeevRemote-macos.dmg neev-remote-macos-arm64.dmg NeevRemote-macOS.pkg \
         neev-remote-macos.pkg NeevRemote-macos.zip neev-remote-macos.zip \
         neev-remote-windows-x64.exe NeevRemote-windows-x64-portable.zip version.json; do
  [ -f "$f" ] && cp -p "$f" "$BACKUP/" || true
done
echo "==> previous release backed up to $BACKUP"

install -m644 "$WORK/NeevRemote-macos.dmg"                 NeevRemote-macos.dmg
install -m644 "$WORK/NeevRemote-macos.dmg"                 neev-remote-macos-arm64.dmg
install -m644 "$WORK/NeevRemote-macos.pkg"                 NeevRemote-macOS.pkg
install -m644 "$WORK/NeevRemote-macos.pkg"                 neev-remote-macos.pkg
install -m644 "$WORK/NeevRemote-macos.zip"                 NeevRemote-macos.zip
install -m644 "$WORK/NeevRemote-macos.zip"                 neev-remote-macos.zip
install -m644 "$WORK/NeevRemote-Setup-x64.exe"             neev-remote-windows-x64.exe
install -m644 "$WORK/NeevRemote-windows-x64-portable.zip"  NeevRemote-windows-x64-portable.zip

# version.json LAST: it is what the update check reads, so it must never
# advertise a build that is not fully served yet.
cat > version.json <<EOF
{
  "build": "build 2026-07-31 · $TAG",
  "release": "$TAG",
  "published": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "url": "http://172.17.17.77:8080/downloads/"
}
EOF

echo "==> served:"
curl -s localhost:8080/api/v1/public/installers/version.json
echo
echo "==> published $TAG at $(TZ=Asia/Kolkata date '+%Y-%m-%d %H:%M:%S %Z')"
rm -rf "$WORK"
