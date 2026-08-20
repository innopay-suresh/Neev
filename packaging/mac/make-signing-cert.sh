#!/usr/bin/env bash
# Generate the self-signed code-signing certificate ONCE.
#
# Why this exists: macOS ties a Screen Recording (TCC) grant to the code
# identity of the binary. An ad-hoc signature has no certificate, so codesign
# falls back to a designated requirement containing the binary's HASH — which
# changes on every build. The grant then silently stops applying while still
# showing as enabled in System Settings, and the host captures nothing.
#
# Signing with a fixed certificate AND an explicit designated requirement makes
# the identity stable across rebuilds, so a grant given once survives every
# future update. Verified: two different binaries signed this way produce a
# byte-identical requirement.
#
# THIS KEY IS A PRODUCTION CREDENTIAL. Whoever holds it can sign a binary that
# inherits your TCC grants on machines that already trust it. Never commit it —
# this repository is public. Store it in GitHub Actions secrets and keep a
# backup in a password manager; CI secrets are write-only and cannot be read
# back, and losing the key means re-granting on every Mac.
set -euo pipefail

CN="${1:-Neev Remote Code Signing}"
OUT="${2:-$(pwd)}"
# 25 years. The requirement match is a hash comparison and ignores expiry, but
# codesign refuses to SIGN with an expired certificate — so this must outlive
# the product rather than merely the current year.
DAYS=9125

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

cat > "$WORK/req.cnf" <<EOF
[req]
distinguished_name = dn
prompt = no
x509_extensions = v3
[dn]
CN = $CN
[v3]
basicConstraints = critical,CA:false
keyUsage = critical,digitalSignature
extendedKeyUsage = critical,codeSigning
EOF

openssl req -x509 -newkey rsa:2048 -keyout "$WORK/key.pem" -out "$WORK/cert.pem" \
  -days "$DAYS" -nodes -config "$WORK/req.cnf" >/dev/null 2>&1

PASS="$(openssl rand -base64 24)"
# -legacy: macOS `security import` cannot read OpenSSL 3's default PKCS#12
# encryption, and fails with a misleading "wrong password" error.
openssl pkcs12 -export -legacy \
  -inkey "$WORK/key.pem" -in "$WORK/cert.pem" \
  -out "$OUT/neev-signing.p12" -passout "pass:$PASS" -name "$CN" >/dev/null 2>&1

FINGERPRINT="$(openssl x509 -in "$WORK/cert.pem" -outform DER | shasum -a 1 | awk '{print toupper($1)}')"

cat <<EOF

Certificate created: $OUT/neev-signing.p12
Common name:         $CN
Valid for:           $DAYS days (~25 years)
SHA-1 fingerprint:   $FINGERPRINT

This fingerprint IS the code identity. Every build signed with this
certificate satisfies the same designated requirement, so a Screen Recording
grant given once keeps working across updates.

Set these two repository secrets
(GitHub → Settings → Secrets and variables → Actions):

  MACOS_SIGNING_P12
$(base64 < "$OUT/neev-signing.p12" | tr -d '\n' | fold -w 76 | sed 's/^/    /')

  MACOS_SIGNING_PASSWORD
    $PASS

Then BACK UP $OUT/neev-signing.p12 and the password somewhere you would keep a
production credential. GitHub secrets cannot be read back, and losing this key
means re-granting Screen Recording on every Mac.

Do NOT commit the .p12 — this repository is public.
EOF
