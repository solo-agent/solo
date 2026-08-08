# Remote Solo Server

Solo's remote topology keeps the web application, API, PostgreSQL, attachments, and artifacts on a server while each Agent runtime stays on its user's computer. The local Daemon opens an outbound authenticated connection, so no local port needs to be exposed through NAT or a firewall.

## Deploy the Server

Prerequisites: a Linux host with Docker Compose, a DNS record pointing to it, a real SMTP relay, and inbound TCP 80/443 plus UDP 443. Do not expose PostgreSQL or port 8080 publicly.

```bash
cd deploy/remote
cp .env.example .env
# Replace every placeholder, including SMTP settings. URL-escape
# DATABASE_URL's password when necessary.
docker compose up -d --build
docker compose ps
curl https://solo.example.com/readyz
```

Caddy obtains and renews TLS automatically. The API and browser WebSocket share the public origin; `/metrics`, PostgreSQL, and the API container port are not published. Startup runs every pending database migration before the API becomes healthy.

### Private deployment before ICP filing

For a mainland host that cannot serve a public website yet, keep the web and
API ports on the server loopback interface and omit the public Caddy service:

```bash
cd deploy/remote
docker compose -f docker-compose.yml -f docker-compose.private.yml \
  up -d --build postgres migrate server frontend private-proxy
ssh -N \
  -L 13000:127.0.0.1:13000 \
  ubuntu@your-server-ip
```

Open `http://127.0.0.1:13000`. This private mode uses verification code
`123456` and must never publish port 13000. After ICP filing, remove
the private override and deploy the normal HTTPS configuration with a real
SMTP relay.

The `127.0.0.1` address is only the Mac/Linux end of the SSH tunnel: the
browser, API, and PostgreSQL requests still terminate on the remote host.

## Pair a local Computer

1. Sign in to the remote UI and open **Computers**.
2. Choose **Add Computer**, name it, and copy the one-time command. The enrollment token expires after ten minutes and can be viewed only in that response.
3. On the local computer, run the copied command. It downloads a checksummed
   macOS/Linux release, installs both binaries, pairs the Computer, and starts
   the Daemon in the background:

```bash
curl -fsSL 'https://raw.githubusercontent.com/solo-agent/solo/master/scripts/install.sh' | \
  bash -s -- connect --server 'https://solo.example.com' \
  --computer-id '...' --token '...'
```

The Daemon stores the exchanged machine credential in `~/.solo/daemon/credentials.json` with mode `0600`. Later starts need no pairing variables because the stored Server URL and credential are reused. Provider credentials and CLI login remain local. The Daemon listens only on `127.0.0.1:8081` for the local `solo` CLI.

Manage it with `solo daemon status`, `solo daemon logs`, `solo daemon restart`,
and `solo daemon stop`. Re-running the installer upgrades both binaries; the
credential remains in place.

## User accounts and email

Registration sends a six-digit verification code before creating the account.
Password recovery uses the same SMTP relay and revokes existing refresh
sessions. Production Server startup fails when `SMTP_HOST` or `SMTP_FROM` is
missing, so a deployment cannot silently expose a broken registration flow.

Set `ALLOW_SIGNUP=false` to close public registration. `ALLOWED_EMAILS` and
`ALLOWED_EMAIL_DOMAINS` are comma-separated exceptions/allowlists for new
accounts; they do not block existing users from logging in.

Set `SOLO_DAEMON_CREDENTIAL_FILE` only when an isolated credential location is required, such as CI or a container. The default is correct for normal installations.

Create Agents by selecting this Computer. An offline paired Computer remains selectable: messages persist remotely and its Runs wait up to 24 hours for the Daemon to reconnect.

## Credential operations

- **Re-pair** creates a new one-time enrollment token. Exchanging it invalidates and disconnects the old credential.
- **Revoke** immediately disconnects the Computer and rejects future control, Run, and RPC calls.
- Only the Computer owner can re-pair, revoke, or delete it. Deletion is refused while active Agents or unfinished Runs still reference it.

## Upgrade and rollback

Back up first, then rebuild. Migrations are additive and run before the new API starts.

```bash
cd deploy/remote
docker compose exec -T postgres sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > solo.backup
docker compose pull
docker compose up -d --build
docker compose ps
```

Keep the previous Git revision until the deployment is verified. Database rollback uses the repository migration command and should be performed only after stopping the API; restoring the backup is safer after production data has used new columns.

Persistent data lives in the Compose volumes `postgres-data`, `attachments`, `artifacts`, and `caddy-data`. Include all four in host backups.

## Recovery behavior

- A control-socket drop does not fail a Run. The Daemon reconnects with exponential backoff and resumes event delivery.
- A Server restart rebuilds Run consumers from PostgreSQL and the Daemon's active attempt list.
- A Daemon restart clears a missing attempt; the stored dispatch payload is delivered again without losing the user's message.
- A second ready Daemon fences and shuts down the stale Daemon for that Computer; missing attempts are reconciled from PostgreSQL.
- Duplicate Run accepts and event callbacks are idempotent by Run attempt and source sequence.
- Workspace, skill, and transcript reads use bounded reverse RPC and fail explicitly while the Computer is offline.

For the complete data model and protocol contract, see [remote-runtime-architecture.md](design/remote-runtime-architecture.md).
