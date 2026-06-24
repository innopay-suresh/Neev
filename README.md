# RemoteAgent

**Cross-platform remote desktop system** — works over public internet and private LANs. Similar to AnyDesk/TeamViewer, built with Go + WebRTC.

---

## Architecture

```
Controller (Viewer) ←──WebRTC P2P──→ Host Agent
        │                                  │
        └────── Signaling Relay ───────────┘
                (WebSocket + TURN)
```

- **P2P first**: WebRTC with STUN NAT hole-punching (no relay needed)
- **TURN fallback**: coturn relay when symmetric NAT blocks direct connection
- **LAN mode**: mDNS discovery — no relay needed on private networks
- **E2E encrypted**: DTLS-SRTP — relay server cannot see screen data

---

## Quick Start (Docker)

```bash
# Clone and start the full stack
git clone https://github.com/yourorg/remote-agent
cd remote-agent
docker compose up -d

# Web viewer at:
open http://localhost:3000
```

## Run the Host Agent

```bash
# macOS / Linux
RELAY_URL=ws://your-server:8080/ws go run ./agent

# Windows
set RELAY_URL=ws://your-server:8080/ws
go run ./agent

# Output:
# ╔══════════════════════════════════════╗
# ║        REMOTE AGENT READY            ║
# ╠══════════════════════════════════════╣
# ║  Agent ID  :  123-456-789            ║
# ║  Password  :  aB3xR7mK               ║
# ╚══════════════════════════════════════╝
```

Open the **web viewer** → enter the ID and password → connect!

### Bootstrap install flow

For company rollouts, the agent now reads a bootstrap env file from the standard platform path:

- Linux: `/etc/remote-agent/agent.env`
- macOS: `/Library/Application Support/RemoteAgent/agent.env`
- Windows: `%ProgramData%\RemoteAgent\agent.env`

The dashboard exposes copyable install commands under **Fleet Enrollment** so you can drop the env file, start the service, and enroll the laptop in one step.

### Windows installer

Build the packaged Windows installer with:

```powershell
cd packaging/windows
.\build-installer.ps1 -AgentBinary ..\..\dist\windows\remote-agent.exe -RelayUrl wss://your-domain/ws -EnrollmentCode your-code -OrgId acme -DeviceGroup laptops
```

The installer writes the bootstrap env file to `%ProgramData%\RemoteAgent\agent.env`, installs the Windows service, and starts it automatically.

### macOS package

Build a signed or unsigned `.pkg` on macOS:

```bash
cd packaging/mac
./build-pkg.sh /path/to/remote-agent-mac
```

You can also embed rollout values during build:

```bash
RELAY_URL=wss://your-domain/ws ENROLLMENT_CODE=your-code ORG_ID=acme DEVICE_GROUP=laptops ./build-pkg.sh /path/to/remote-agent-mac
```

The package installs the binary to `/usr/local/bin/remote-agent-mac`, writes `/Library/Application Support/RemoteAgent/agent.env`, and boots the launchd service.

### Linux `.deb`

Build a Debian package:

```bash
cd packaging/linux
./build-deb.sh /path/to/remote-agent
```

Or bake enrollment values into the package:

```bash
RELAY_URL=wss://your-domain/ws ENROLLMENT_CODE=your-code ORG_ID=acme DEVICE_GROUP=laptops ./build-deb.sh /path/to/remote-agent
```

The package installs the binary to `/opt/remote-agent/remote-agent`, writes `/etc/remote-agent/agent.env`, and enables the `remote-agent.service` systemd unit.

### One-command release build

The GitHub Actions **Build and Release** workflow now builds the Linux `.deb`, macOS `.pkg`, and Windows installer together on tag push or manual dispatch. It uses the same release wrappers in `scripts/release-packages.sh` and `scripts/release-packages.ps1`.

## Public Internet Deployment

To use RemoteAgent without VPN, the signaling relay and TURN server must be reachable from the public internet:

- Expose the signaling server on `8080/tcp` and the TURN server on `3478/udp`, `3478/tcp`, plus the relay port range used by coturn.
- Set `RELAY_URL` on agents and desktop clients to the public WebSocket URL, for example `wss://your-domain/ws`.
- Keep `TURN_URL` pointed at a public TURN endpoint so browsers can fall back when direct P2P cannot be established.
- The controller and agent now fetch ICE servers from `/api/v1/session/ice-servers`, so the server config controls the network path automatically.

## Public Installer Portal

If you want users to download installers without logging in:

- Run `scripts/publish-installers.sh` on macOS or Linux to publish the current platform installer into `dist/packages`.
- Build packages with `scripts/release-packages.sh` or `scripts/release-packages.ps1`.
- Copy the generated files into the server’s `PUBLIC_DOWNLOAD_DIR` folder.
- Open `#/downloads` in the browser and share that link with users.
- For Docker deployments, mount the package folder into the server container at `/app/downloads`.

The portal reads installers from the configured download directory and serves them from `/api/v1/public/installers/...`.

---

## Repository Structure

```
remote-agent/
├── server/               # Signaling server (Go)
│   ├── signaling/        # WebSocket SDP exchange hub
│   ├── session/          # Redis-backed agent registry
│   ├── api/              # REST API (Fiber)
│   ├── config/           # YAML configuration
│   └── db/migrations/    # PostgreSQL schema
├── agent/                # Host daemon (Go, cross-compiled)
│   ├── capture/          # Platform screen capture
│   │   ├── capture_windows.go  (DXGI)
│   │   ├── capture_darwin.go   (CoreGraphics)
│   │   └── capture_linux.go    (X11 XShm)
│   ├── input/            # Mouse/keyboard injection
│   │   ├── input_windows.go    (SendInput)
│   │   ├── input_darwin.go     (CGEvent)
│   │   └── input_linux.go      (XTest)
│   ├── auth/             # Argon2id password hashing
│   └── network/          # WebRTC peer + mDNS + signaling client
├── web/                  # Browser-based controller (React + Vite)
├── deploy/               # Docker, config, coturn
└── packaging/            # Platform installers (Phase 6)
```

---

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `RELAY_URL` | `ws://localhost:8080/ws` | Signaling server WebSocket URL |
| `ACCESS_PASSWORD` | *(auto-generated)* | One-time session password |
| `TURN_URL` | *(none)* | TURN server URL e.g. `turn:myserver:3478` |
| `TURN_USER` | `agent` | TURN username |
| `TURN_PASS` | `changeme` | TURN credential |
| `ENROLLMENT_CODE` | *(none)* | Shared enrollment code for new agents |
| `ORG_ID` | *(none)* | Optional organization label for enrolled agents |
| `DEVICE_GROUP` | *(none)* | Optional group label for enrolled agents |
| `JWT_SECRET` | `CHANGE_ME_IN_PRODUCTION` | Dashboard token signing secret |
| `AUTH_ENABLED` | `false` | Enable dashboard login and RBAC |
| `AUTH_BOOTSTRAP_EMAIL` | *(none)* | First admin/support account email |
| `AUTH_BOOTSTRAP_PASSWORD` | *(none)* | First admin/support account password |
| `AUTH_BOOTSTRAP_PASSWORD_HASH` | *(none)* | Pre-hashed first account password |
| `AUTH_BOOTSTRAP_ROLE` | `admin` | Bootstrap account role |
| `TLS_CLIENT_CA` | *(none)* | CA bundle used to verify agent client certificates |
| `TLS_CLIENT_CA_KEY` | *(none)* | Private key used to issue agent client certificates |
| `PUBLIC_DOWNLOAD_DIR` | `./downloads` | Folder that the public installer portal serves from |
| `AGENT_CERT_FILE` | *(none)* | Agent client certificate PEM path |
| `AGENT_KEY_FILE` | *(none)* | Agent client key PEM path |
| `AGENT_CA_FILE` | *(none)* | Optional CA bundle for agent-to-server TLS |
| `REMOTE_AGENT_CONFIG` | *(platform default)* | Explicit bootstrap env file path |
| `CONFIG_PATH` | *(none)* | Path to server config YAML |

---

## Security

- **End-to-end encrypted** via DTLS-SRTP (relay blind to content)
- **Argon2id** password hashing (memory-hard, GPU-resistant)
- **Access password** displayed only on host — required for connection
- **Security middleware**: origin allowlist, request IDs, secure headers
- **Audit trail** in Redis: sessions and connection attempts recorded
- **Dashboard auth + RBAC**: JWT-backed login with admin/support roles
- **MFA**: TOTP-based two-factor login for dashboard users
- **Admin user management**: create, update, and delete dashboard accounts
- **Agent mTLS**: client certificates on `/agent/ws` for endpoint identity
- **Managed device trust**: server issues, rotates, and revokes per-device client certificates
- **Public installer portal**: anonymous visitors can download published installers
- **Remote sessions still use ID + password**; dashboard access uses login when enabled

---

## Platforms

| Platform | Host Agent | Web Viewer | Desktop Client (Phase 3) |
|---|---|---|---|
| Windows 10/11 | ✅ (DXGI) | ✅ | Planned |
| macOS 12+ | ✅ (CoreGraphics) | ✅ | Planned |
| Linux (X11) | ✅ (XShm) | ✅ | Planned |
| Linux (Wayland) | 🔄 Planned | ✅ | Planned |
| iOS / Android | — | ✅ (browser) | Planned |

---

## Roadmap

- [x] Phase 1: Signaling server + agent registration + P2P WebRTC
- [ ] Phase 2: Screen capture + VP8 streaming
- [ ] Phase 3: Desktop client (Wails)
- [ ] Phase 4: Web viewer + admin dashboard *(web viewer skeleton done)*
- [ ] Phase 5: Security hardening (MFA + audit done; mTLS in place)
- [x] Phase 6: Packaging (NSIS, .pkg, .deb) + self-hosted deploy
- [ ] Phase 7: Managed device trust (CA issuance, rotation, revocation)

---

## License

MIT — see [LICENSE](LICENSE)
