#!/usr/bin/env bash
# Sign a binary or bundle with a STABLE code identity.
#
# The whole point is the designated requirement. codesign only derives a
# certificate-based requirement for Apple-chained certs; for ad-hoc AND for
# self-signed it falls back to the binary's cdhash, which changes every build
# and silently invalidates the machine's Screen Recording grant. Passing the
# requirement explicitly pins the identity to the certificate instead, so a
# grant given once survives every update.
#
# Verified: two different binaries signed this way produce a byte-identical
# designated requirement.
#
# Falls back to ad-hoc when no signing identity is configured, so local builds
# and forks (which cannot read repository secrets) still produce a working app —
# they just inherit the per-update re-grant.
#
# Usage: sign-stable.sh <identifier> <path> [<path>...]
set -euo pipefail

IDENTIFIER="${1:?bundle identifier required}"
shift

if [ -z "${NEEV_SIGN_IDENTITY:-}" ]; then
  for target in "$@"; do
    codesign --force --sign - --identifier "$IDENTIFIER" --timestamp=none "$target" \
      >/dev/null 2>&1 || echo "   (ad-hoc sign skipped for $target)"
  done
  exit 0
fi

# find-identity reports the certificate's SHA-1, which is exactly what the
# requirement needs — one value serves as both the signing selector and the
# leaf hash.
REQ_FILE="$(mktemp)"
trap 'rm -f "$REQ_FILE"' EXIT
printf 'designated => identifier "%s" and certificate leaf = H"%s"\n' \
  "$IDENTIFIER" "$NEEV_SIGN_IDENTITY" > "$REQ_FILE"

for target in "$@"; do
  codesign --force --sign "$NEEV_SIGN_IDENTITY" --identifier "$IDENTIFIER" \
    ${NEEV_SIGN_KEYCHAIN:+--keychain "$NEEV_SIGN_KEYCHAIN"} \
    -r "$REQ_FILE" --timestamp=none "$target"
  # Prove the requirement actually took. A silent fallback to cdhash would
  # reintroduce the exact bug this exists to prevent, and would only surface
  # weeks later as "permissions stopped working after an update".
  if codesign -dr - "$target" 2>&1 | grep -q 'cdhash'; then
    echo "ERROR: $target still has a cdhash-pinned requirement — identity is NOT stable" >&2
    exit 1
  fi
done
