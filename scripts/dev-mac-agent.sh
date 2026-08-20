#!/usr/bin/env bash
# Build, sign and install the macOS host agent WITHOUT a release.
#
# Why this exists: every agent-side bug in the r168-r174 run — capture, input,
# consent, modifiers, idle CPU — was a Go change that took ~3 seconds to
# compile, and each one still cost ~20 minutes because the only path to a
# testable binary went through CI, a signed .pkg and a full reinstall. That is
# the wrong loop for a fix you want to try twice in a row.
#
# This does the same thing the installer does, for the agent only:
#   build (~3s) -> sign with the SAME identity -> install -> restart -> tail log
#
# Signing with the real identity is not optional. TCC keys the Screen Recording
# and Accessibility grants to the code requirement, so an ad-hoc dev binary is a
# different identity and lands you re-granting permissions by hand every
# iteration — which is most of what made the loop painful in the first place.
#
# NOT a substitute for a release: it touches one machine and skips the app, the
# packaging and the release gate. Ship the real thing when the fix is settled.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"
SUPPORT="/Library/Application Support/NeevRemote"
# sign-stable.sh wants the certificate's SHA-1 (it serves as both the signing
# selector and the leaf hash in the designated requirement), not its name.
CERT_NAME="${NEEV_DEV_CERT_NAME:-Neev Remote Dev Signing}"
CERT_SHA1="$(security find-identity -v -p codesigning 2>/dev/null \
  | grep "$CERT_NAME" | head -1 | awk '{print $2}')"

cd "$REPO"

echo "==> building"
go build -o /tmp/neev-agent-dev ./agent

if [ -z "$CERT_SHA1" ]; then
  echo "WARNING: no '$CERT_NAME' certificate in the keychain — signing ad-hoc." >&2
  echo "         Every build will then be a NEW identity to TCC, so you will" >&2
  echo "         re-grant Screen Recording and Accessibility every iteration." >&2
  codesign --force --sign - --identifier com.neev.agent --timestamp=none /tmp/neev-agent-dev
else
  echo "==> signing with $CERT_NAME ($CERT_SHA1)"
  NEEV_SIGN_IDENTITY="$CERT_SHA1" bash packaging/mac/sign-stable.sh com.neev.agent /tmp/neev-agent-dev
  # NOTE: this is very likely NOT the certificate CI signs releases with, so the
  # dev binary has its own TCC identity. Grant Screen Recording + Accessibility
  # to it ONCE; the dev cert does not change, so every later dev build inherits
  # the grant and the loop stays permission-free.
fi

echo "==> installing (sudo)"
sudo install -m 0755 /tmp/neev-agent-dev "$SUPPORT/neev-agent"

echo "==> restarting transport + worker"
sudo launchctl kickstart -k system/com.neev.transport
launchctl kickstart -k "gui/$(id -u)/com.neev.worker"

# The FIRST worker after the binary changes is the one macOS denies while it
# records the grant (LD-MAC-TCC-4), so a single restart is not enough on the
# first run after an identity change. Restart again if capture did not start.
WORKER_LOG="${TMPDIR}NeevRemote-worker.log"
sleep 4
if ! tail -20 "$WORKER_LOG" 2>/dev/null | grep -q "capture bounds"; then
  echo "==> capture not up yet; restarting the worker once more"
  launchctl kickstart -k "gui/$(id -u)/com.neev.worker"
  sleep 4
fi

echo "==> worker log:"
tail -6 "$WORKER_LOG" 2>/dev/null | sed 's/^/    /'
echo
echo "done. Host id: $(cat "$SUPPORT/transport.id" 2>/dev/null || echo '(not registered yet)')"
