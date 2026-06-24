#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"

agent_binary="${1:-$repo_root/dist/linux/remote-agent}"
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
architecture="${ARCH:-$(dpkg --print-architecture 2>/dev/null || echo amd64)}"
output_deb="${OUTPUT_DEB:-$script_dir/remote-agent_${version}_${architecture}.deb}"

if [[ ! -f "$agent_binary" ]]; then
  echo "Agent binary not found: $agent_binary" >&2
  exit 1
fi

staging_root="$(mktemp -d /tmp/remote-agent-deb.XXXXXX)"
trap 'rm -rf "$staging_root"' EXIT

pkg_root="$staging_root/pkg"
control_root="$pkg_root/DEBIAN"
mkdir -p "$pkg_root/opt/remote-agent"
mkdir -p "$pkg_root/etc/systemd/system"
mkdir -p "$pkg_root/etc/remote-agent"
mkdir -p "$pkg_root/usr/bin"
mkdir -p "$pkg_root/usr/share/applications"
mkdir -p "$control_root"

cp "$agent_binary" "$pkg_root/opt/remote-agent/remote-agent"
cp "$repo_root/packaging/linux/remote-agent.service" "$pkg_root/etc/systemd/system/remote-agent.service"
chmod 755 "$pkg_root/opt/remote-agent/remote-agent"

cat > "$pkg_root/usr/bin/remote-agent-status" <<'EOF'
#!/bin/sh
xdg-open "http://127.0.0.1:7891/" >/dev/null 2>&1 || true
EOF
chmod 755 "$pkg_root/usr/bin/remote-agent-status"

cat > "$pkg_root/usr/share/applications/remote-agent-status.desktop" <<'EOF'
[Desktop Entry]
Version=1.0
Type=Application
Name=RemoteAgent Status
Comment=Open the local RemoteAgent status page
Exec=remote-agent-status
Terminal=false
Categories=Network;Utility;
EOF

cat > "$pkg_root/etc/remote-agent/agent.env" <<EOF
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

cat > "$control_root/control" <<EOF
Package: remote-agent
Version: ${version}
Section: utils
Priority: optional
Architecture: ${architecture}
Maintainer: RemoteAgent <support@example.com>
Depends: systemd
Description: Cross-platform remote desktop host agent
 RemoteAgent installs a host service and enrolls it against a central relay.
EOF

cp "$repo_root/packaging/linux/scripts/postinst" "$control_root/postinst"
cp "$repo_root/packaging/linux/scripts/prerm" "$control_root/prerm"
cp "$repo_root/packaging/linux/scripts/postrm" "$control_root/postrm"
chmod 755 "$control_root/postinst" "$control_root/prerm" "$control_root/postrm"
chmod 644 "$pkg_root/etc/systemd/system/remote-agent.service" "$pkg_root/etc/remote-agent/agent.env"
chmod 644 "$pkg_root/usr/share/applications/remote-agent-status.desktop"

dpkg-deb --build "$pkg_root" "$output_deb"
echo "Built Debian package: $output_deb"
