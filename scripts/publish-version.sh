#!/usr/bin/env bash
# Publishes the build stamp the app checks against.
#
# The app can now answer "am I current?", but only if the portal states what
# current IS. This writes version.json into the downloads directory, which the
# portal already serves by filename — so no server change was needed.
#
# Run this as part of every publish, AFTER the installers are in place. A stale
# version.json is worse than none: it would tell users on the newest build to
# downgrade, or hide a real update.
set -euo pipefail

TAG="$(grep -o "r[0-9]\+-[a-z-]*" neev_remote/lib/core/constants/app_constants.dart | head -1)"
[ -n "$TAG" ] || { echo "could not read the build tag from app_constants.dart" >&2; exit 1; }

BUILD="$(grep -o "build [0-9-]* · r[0-9]\+-[a-z-]*" neev_remote/lib/core/constants/app_constants.dart | head -1)"

cat > /tmp/version.json <<JSON
{
  "build": "${BUILD}",
  "release": "${TAG}",
  "published": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "url": "http://172.17.17.77:8080/downloads/"
}
JSON

echo "publishing:"; cat /tmp/version.json
sshpass -e scp -o StrictHostKeyChecking=no /tmp/version.json \
  neev@172.17.17.77:~/neev/downloads/version.json
echo "verifying via the public API:"
sshpass -e ssh -o StrictHostKeyChecking=no -o NumberOfPasswordPrompts=1 \
  neev@172.17.17.77 'curl -s localhost:8080/api/v1/public/installers/version.json'
