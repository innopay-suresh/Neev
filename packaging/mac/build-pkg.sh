#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"

agent_binary="${1:-$repo_root/dist/macos/remote-agent-mac}"
relay_url="${RELAY_URL:-ws://localhost:8080/ws}"
enrollment_code="${ENROLLMENT_CODE:-}"
org_id="${ORG_ID:-}"
device_group="${DEVICE_GROUP:-}"
turn_url="${TURN_URL:-}"
turn_user="${TURN_USER:-agent}"
turn_pass="${TURN_PASS:-changeme}"
agent_cert_file="${AGENT_CERT_FILE:-}"
agent_key_file="${AGENT_KEY_FILE:-}"
agent_ca_file="${AGENT_CA_FILE:-}"
version="${VERSION:-1.0.0}"
output_pkg="${OUTPUT_PKG:-$script_dir/RemoteAgent-${version}.pkg}"

if [[ ! -f "$agent_binary" ]]; then
  echo "Agent binary not found: $agent_binary" >&2
  exit 1
fi

staging_root="$(mktemp -d /tmp/remote-agent-mac.XXXXXX)"
trap 'rm -rf "$staging_root"' EXIT

payload_root="$staging_root/root"
scripts_root="$staging_root/scripts"
mkdir -p "$payload_root/usr/local/bin"
mkdir -p "$payload_root/Library/LaunchAgents"
mkdir -p "$payload_root/Library/Application Support/RemoteAgent"
mkdir -p "$payload_root/Applications/RemoteAgent.app/Contents/MacOS"
mkdir -p "$payload_root/Applications/RemoteAgent.app/Contents/Resources"
mkdir -p "$payload_root/Applications/RemoteAgent Status.app/Contents/MacOS"
mkdir -p "$payload_root/Applications/RemoteAgent Status.app/Contents/Resources"
mkdir -p "$scripts_root"

cp "$agent_binary" "$payload_root/Applications/RemoteAgent.app/Contents/MacOS/RemoteAgent"
ln -sf "/Applications/RemoteAgent.app/Contents/MacOS/RemoteAgent" "$payload_root/usr/local/bin/remote-agent-mac"
cp "$repo_root/packaging/mac/com.neev.remoteagent.plist" "$payload_root/Library/LaunchAgents/com.neev.remoteagent.plist"
cp "$repo_root/packaging/mac/scripts/postinstall" "$scripts_root/postinstall"
chmod 755 "$scripts_root/postinstall"

cat > "$payload_root/Applications/RemoteAgent.app/Contents/Info.plist" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key>
  <string>RemoteAgent</string>
  <key>CFBundleIdentifier</key>
  <string>com.neev.remoteagent.agent</string>
  <key>CFBundleName</key>
  <string>RemoteAgent</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>1.0.0</string>
  <key>CFBundleVersion</key>
  <string>1.0.0</string>
  <key>LSMinimumSystemVersion</key>
  <string>10.13.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
EOF

cat > "$payload_root/Applications/RemoteAgent Status.app/Contents/Info.plist" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleExecutable</key>
  <string>remote-agent-status</string>
  <key>CFBundleIdentifier</key>
  <string>com.neev.remoteagent.status</string>
  <key>CFBundleName</key>
  <string>RemoteAgent Status</string>
  <key>CFBundlePackageType</key>
  <string>APPL</string>
  <key>CFBundleShortVersionString</key>
  <string>1.0.0</string>
  <key>CFBundleVersion</key>
  <string>1.0.0</string>
  <key>LSMinimumSystemVersion</key>
  <string>10.13.0</string>
  <key>NSHighResolutionCapable</key>
  <true/>
</dict>
</plist>
EOF

cat > "$payload_root/Applications/RemoteAgent Status.app/Contents/MacOS/remote-agent-status" <<'EOF'
#!/bin/sh
open "http://127.0.0.1:7891/"
EOF

cat > "$payload_root/Library/Application Support/RemoteAgent/agent.env" <<EOF
RELAY_URL=${relay_url}
ENROLLMENT_CODE=${enrollment_code}
ORG_ID=${org_id}
DEVICE_GROUP=${device_group}
TURN_URL=${turn_url}
TURN_USER=${turn_user}
TURN_PASS=${turn_pass}
AGENT_CERT_FILE=${agent_cert_file}
AGENT_KEY_FILE=${agent_key_file}
AGENT_CA_FILE=${agent_ca_file}
NO_BROWSER=1
EOF

chmod 644 "$payload_root/Library/LaunchAgents/com.neev.remoteagent.plist"
chmod 755 "$payload_root/Applications/RemoteAgent.app/Contents/MacOS/RemoteAgent"
chmod 755 "$payload_root/Applications/RemoteAgent Status.app/Contents/MacOS/remote-agent-status"
chmod 600 "$payload_root/Library/Application Support/RemoteAgent/agent.env"

if command -v codesign >/dev/null 2>&1; then
  codesign --force --sign - --timestamp=none "$payload_root/Applications/RemoteAgent.app" >/dev/null 2>&1 || true
  codesign --force --sign - --timestamp=none "$payload_root/Applications/RemoteAgent Status.app" >/dev/null 2>&1 || true
fi

pkgbuild \
  --root "$payload_root" \
  --scripts "$scripts_root" \
  --identifier com.neev.remoteagent \
  --version "$version" \
  --install-location / \
  "$output_pkg"

echo "Built macOS package: $output_pkg"
