# RemoteAgent Roadmap

This roadmap follows the same operating model as AnyDesk/TeamViewer:

- one central control plane
- one agent installed on every endpoint
- web and desktop controllers for support staff
- public and LAN connectivity using the same agent

## Phase 1 — Central Control Plane

- Keep one relay/server stack for the whole company
- Expose signaling, dashboard, and TURN from a single deployment
- Store devices, groups, sessions, and logs centrally

## Phase 2 — Endpoint Agent

- Ship a single agent for Windows, macOS, and Linux
- Auto-register each device to the central server
- Support unattended access and device identity
- Keep the agent always available after reboot/login

## Phase 3 — Enrollment and Fleet Management

- Add organization and device-group metadata
- Support enrollment keys or deployment tokens
- Allow IT to assign devices to groups during install
- Surface enrolled devices in the admin dashboard

## Phase 4 — Controllers

- Keep the browser viewer for support from any laptop
- Keep the desktop viewer for power users and admins
- Use the same session broker for both

## Phase 5 — Packaging and Rollout

- Windows: MSI / silent install / Intune / GPO
- macOS: PKG / MDM / Jamf
- Linux: DEB / RPM / systemd service
- Provide a branded custom client for company rollout

## Phase 6 — Policy and Security

- Add device allowlists and role-based access
- Add approval flows for sensitive systems
- Add session audit logs and retention
- Add stronger auth, MFA, and admin controls

## Phase 7 — Managed Device Trust

- Issue per-device certificates for agent identity
- Distribute trust bundles through installers and config
- Add certificate rotation and revocation
- Tie cert lifecycle to enrollment and device ownership

Status:

- Dashboard login + JWT roles are now implemented
- MFA + admin user management are now implemented
- Agent mTLS is in place
- Device certificate issuance, rotation, and revocation are now implemented
- Managed CA rotation and lifecycle tooling are next
- approval workflows and device allowlists are still pending
