# Deploying Neev Remote on a Linux server

This deploys the **signaling server + download portal + TURN relay** on a Linux
host using Docker Compose. The desktop/web clients then connect to it.

The stack (`deploy/docker-compose.prod.yml`):

| Service | Purpose | Image |
|---|---|---|
| `server` | Signaling (WebSocket) + REST API + serves the web app & installers | built from `deploy/Dockerfile.server` |
| `redis` | Session / agent registry | `redis:7-alpine` |
| `postgres` | Audit log, users (only used when `auth.enabled: true`) | `postgres:16-alpine` |
| `turn` | TURN/STUN relay for clients behind strict NAT | `coturn/coturn` |

---

## 1. Prerequisites

- A Linux server (Ubuntu 22.04 LTS recommended) with a **public IP** and ideally a **domain** (e.g. `remote.yourcompany.com`).
- Docker Engine + Compose plugin:
  ```bash
  curl -fsSL https://get.docker.com | sh
  sudo usermod -aG docker $USER   # re-login after this
  ```
- The repo on the server:
  ```bash
  git clone <your-repo-url> neev && cd neev
  ```

---

## 2. Configure `deploy/config.prod.yaml`

Edit these to match your server and set **strong secrets** (don't ship the defaults):

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  public_download_dir: "/app/downloads"
  debug: false
  allowed_origins:
    - "https://remote.yourcompany.com"   # your portal origin(s)

turn:
  public_ip: "YOUR.PUBLIC.IP"            # the server's public IP
  port: 3478
  realm: "remote.yourcompany.com"
  auth_user: "agent"
  auth_pass: "<STRONG-RANDOM>"

jwt:
  secret: "<STRONG-RANDOM>"

auth:
  enabled: true
  bootstrap_email: "admin@yourcompany.com"
  bootstrap_password: "<STRONG-RANDOM>"   # first dashboard login

network:
  stun_servers:
    - "stun:stun.l.google.com:19302"
  turn_server: "turn:YOUR.PUBLIC.IP:3478"
  relay_url: "wss://remote.yourcompany.com/ws"   # what clients dial (see TLS below)
  enrollment_code: "<STRONG-RANDOM>"
```

Also set the DB password via env (compose reads `${DB_PASSWORD}`):
```bash
echo "DB_PASSWORD=$(openssl rand -hex 16)" >> deploy/.env
```

> Match `turn.auth_user`/`auth_pass` to `deploy/turnserver.conf`.

---

## 3. Serve the installers (downloads)

The portal lists files from `public_download_dir` (`/app/downloads` in the
container). Mount a host folder so you can drop installers in without rebuilding.
Create `deploy/docker-compose.override.yml`:

```yaml
services:
  server:
    volumes:
      - ./config.prod.yaml:/app/config.yaml:ro
      - ../downloads:/app/downloads:ro          # /api/v1/public/installers
      - ../flutter-downloads:/app/flutter-downloads:ro
```

Put your built installers (`.exe`, `.dmg`, `.pkg`, `.tar.gz`) into `downloads/`
on the host — they appear on the portal immediately.

---

## 4. TLS / HTTPS (required for production)

Browsers need **HTTPS** for the web client's screen capture, and clients should
use **`wss://`**. Easiest: put **Caddy** in front for automatic Let's Encrypt.

`deploy/Caddyfile`:
```
remote.yourcompany.com {
    reverse_proxy localhost:8080
}
```
Run Caddy (host-installed or as a container) — it auto-provisions the cert and
proxies `443 → 8080`, including the `/ws` WebSocket. Then `relay_url` is
`wss://remote.yourcompany.com/ws`.

> Alternative: set `server.tls_cert` / `server.tls_key` in config to your PEM
> paths and mount them — the Go server then terminates TLS itself on 8443.

---

## 5. Open the firewall

```bash
sudo ufw allow 443/tcp        # (or 8080/tcp if not using a TLS proxy)
sudo ufw allow 3478/tcp
sudo ufw allow 3478/udp
sudo ufw allow 49160:49200/udp   # TURN relay range
```

---

## 6. Launch

```bash
cd deploy
docker compose -f docker-compose.prod.yml up -d --build
docker compose -f docker-compose.prod.yml ps
docker compose -f docker-compose.prod.yml logs -f server
```

Verify:
```bash
curl -fsS https://remote.yourcompany.com/v1/health      # -> 200
```
Open `https://remote.yourcompany.com` → dashboard → **Downloads**.

---

## 7. Point the client installers at this server

The installer bakes in the server URL at build time. Build with your domain so
clients auto-connect (users don't have to touch Settings):

- **CI:** set `RELAY_URL: 'wss://remote.yourcompany.com/ws'` in
  `.github/workflows/flutter.yml` (the `env:` block), push, download the
  artifacts.
- **Local:** `flutter build windows --release --dart-define=RELAY_URL=wss://remote.yourcompany.com/ws`
  (and the same for macos/linux), or run `packaging/build_*.sh` with
  `RELAY_URL` exported.

Then drop the resulting installers into `downloads/` (step 3).

---

## 8. Updating

```bash
git pull
cd deploy
docker compose -f docker-compose.prod.yml up -d --build server
```

## Operations

- **Logs:** `docker compose -f docker-compose.prod.yml logs -f server`
- **Restart:** `docker compose -f docker-compose.prod.yml restart server`
- **Backups:** the `redis_data` and `postgres_data` volumes hold state.
- **No Postgres?** Set `auth.enabled: false` in config and you can remove the
  `postgres` service + `depends_on` (signaling only needs Redis).
