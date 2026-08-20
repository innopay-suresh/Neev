#!/usr/bin/env bash
# Create the LOCAL dev signing identity. Run once, then never again.
#
# Why not reuse the release certificate: it is a production credential (whoever
# holds it can sign a binary this machine already trusts for Screen Recording),
# it lives in GitHub secrets, and dev builds have no business carrying it. A
# separate local certificate keeps the fast loop working without spreading the
# real key onto workstations.
#
# Why not ad-hoc: TCC keys Screen Recording and Accessibility to the code
# requirement. Ad-hoc has no certificate, so the requirement pins the binary's
# HASH and changes on every build — you would re-grant permissions every single
# iteration, which is precisely the friction the fast loop exists to remove.
# With a fixed certificate you grant once and every later dev build inherits it.
set -euo pipefail

KC_NAME="neev-dev.keychain"
KC_PASS="${NEEV_DEV_KEYCHAIN_PASS:-neev-dev}"
CN="Neev Remote Dev Signing"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

if security find-identity -v -p codesigning 2>/dev/null | grep -q "$CN"; then
  echo "already set up: '$CN' is present."
  exit 0
fi

echo "==> creating $KC_NAME"
security delete-keychain "$KC_NAME" 2>/dev/null || true
security create-keychain -p "$KC_PASS" "$KC_NAME"
# No auto-lock: a locked keychain makes codesign block on a GUI prompt, which is
# exactly the dialog this script exists to stop you seeing.
security set-keychain-settings -lut 36000 "$KC_NAME"
security unlock-keychain -p "$KC_PASS" "$KC_NAME"

echo "==> generating a 25-year code-signing certificate"
openssl req -x509 -newkey rsa:2048 -nodes -days 9125 \
  -keyout "$WORK/key.pem" -out "$WORK/cert.pem" \
  -subj "/CN=$CN" \
  -addext "keyUsage=critical,digitalSignature" \
  -addext "extendedKeyUsage=critical,codeSigning" >/dev/null 2>&1
openssl pkcs12 -export -legacy -inkey "$WORK/key.pem" -in "$WORK/cert.pem" \
  -out "$WORK/dev.p12" -passout "pass:$KC_PASS" >/dev/null 2>&1

security import "$WORK/dev.p12" -k "$KC_NAME" -P "$KC_PASS" -T /usr/bin/codesign -A
# Without this, codesign stops on "wants to use the keychain" every build.
security set-key-partition-list -S apple-tool:,apple:,codesign: \
  -s -k "$KC_PASS" "$KC_NAME" >/dev/null 2>&1
security list-keychains -d user -s "$KC_NAME" login.keychain-db

echo "==> trusting the certificate (admin password required once)"
# find-identity -v hides an untrusted self-signed certificate, so signing would
# silently fall back to ad-hoc without this step.
sudo security add-trusted-cert -d -r trustRoot \
  -p codeSign -k /Library/Keychains/System.keychain "$WORK/cert.pem"

echo
security find-identity -v -p codesigning | grep "$CN" || {
  echo "ERROR: the identity is still not usable — signing would fall back to ad-hoc." >&2
  exit 1
}
echo
echo "Done. Next:"
echo "  1. bash scripts/dev-mac-agent.sh"
echo "  2. grant Screen Recording + Accessibility to"
echo "     /Library/Application Support/NeevRemote/neev-agent  (ONCE)"
echo "Every later dev build reuses this identity, so permissions stay granted."
