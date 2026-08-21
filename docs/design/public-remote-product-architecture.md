# Solo Public Remote Product Architecture

> Status: implemented and verified against local real components; public
> activation still requires ICP approval, Tencent SES activation/configuration, and a
> published GitHub Release
> Baseline: `codex/remote-server` after the Remote Runtime V1 work
> Goal: a new user can open a public Solo URL, create and recover an account,
> install and pair a local Computer without source code, and receive a real
> Agent result from the remote Server.

## 1. Product boundary

The public Server owns the web UI, user identity, routing, durable Runs,
messages, tasks, attachments, and deployment state. The local Computer owns the
Daemon process, provider CLIs and credentials, workspaces, skills, transcripts,
and persistent provider sessions.

This work completes three product modules:

1. verified email registration and password recovery;
2. downloadable Solo CLI/Daemon releases and one-command Computer pairing;
3. production defaults, operator documentation, and an end-to-end first-user
   acceptance path.

It does not introduce passwordless login, OAuth, invitations, CAPTCHA, Redis,
an update service, or a second Daemon implementation. These are independent
features and are not required for the first complete remote product.

## 2. Domain model

### 2.1 User

`users.email_verified_at` records that the current email address was proved by
the user. Existing users are backfilled with `created_at`, so upgrading never
locks out an existing account. Login continues to use email and password and is
not dependent on the mail provider after registration.

### 2.2 Email challenge

`auth_email_challenges` is a short-lived, single-use authorization record:

| Field | Meaning |
|---|---|
| `email` | normalized target email |
| `purpose` | `register` or `password_reset` |
| `code_hash` | HMAC-SHA-256 bound to Server secret, email, purpose, and code; raw codes are never persisted |
| `display_name` | pending registration name; null for reset |
| `password_hash` | pending bcrypt hash; null for reset |
| `attempts` | failed verification attempts, capped at five |
| `expires_at` | ten-minute expiry |
| `used_at` | single-use marker |
| `created_at` | resend/cooldown ordering |

Only the most recent unused challenge for an email and purpose is accepted.
Creating a new challenge invalidates previous unused challenges of that
purpose. A one-minute per-email resend cooldown supplements the public route IP
rate limit.

### 2.3 Local installation

No installation row or package registry is added to PostgreSQL. GitHub Release
artifacts are the release source of truth. The existing Computer credential
file remains the local identity source of truth:

`~/.solo/daemon/credentials.json` (directory `0700`, file `0600`).

Daemon process state uses `~/.solo/daemon/daemon.pid` and
`~/.solo/daemon/daemon.log`. The PID is validated before signals are sent.

## 3. Ownership and lifecycle

### 3.1 Registration

```text
form submit -> validate signup policy -> create challenge -> send code
           -> verify latest challenge in transaction
           -> mark challenge used + create verified user
           -> issue existing access/refresh tokens -> bootstrap onboarding
```

An email address is not a usable account until verification succeeds. Duplicate
registration for an existing account returns a conflict without issuing mail.
`ALLOW_SIGNUP`, `ALLOWED_EMAILS`, and `ALLOWED_EMAIL_DOMAINS` apply only to new
accounts; they never prevent an existing active user from logging in.

### 3.2 Password recovery

```text
request(email) -> always return the same public response
               -> if active verified user exists, send reset code
verify(email, code, new password)
               -> consume challenge + replace bcrypt hash + revoke sessions
               -> user signs in with the new password
```

The request response does not reveal whether an account exists. Reset consumes
all existing refresh sessions. Access tokens remain bounded by their existing
short lifetime.

### 3.3 Daemon installation and pairing

```text
Computers page creates Computer and one-time enrollment token
 -> user runs generated install command
 -> installer downloads archive + checksum from GitHub Releases
 -> installs `solo` and `solo-daemon`
 -> the Computer ID is also used as the managed Daemon profile
 -> `solo daemon connect` starts that profile with the one-time token
 -> Daemon exchanges and persists the machine credential
 -> `solo daemon status` observes the managed background process
```

The Computers page and installer derive the profile from the existing Computer
ID; users do not choose another name. Each Computer therefore owns an independent
local state directory, process, port, credential, and log. `connect` replaces only
that Computer's local pairing and restarts only its managed Daemon. This remains
compatible with the released CLI that already supports explicit profiles, while a
newer CLI can also infer the same profile when it is omitted. No database or API
migration is required. The Server's existing one-current-connection fence remains
the remote authority. `start`, `stop`, `restart`, `status`, and `logs` operate on
the selected local managed process. Direct `solo-daemon` execution remains
supported for containers and advanced operators.

## 4. Data flow and trust boundaries

- The browser sends registration and reset codes only over HTTPS.
- The Server stores hashes, never raw email or machine secrets beyond the email
  address that is already the account identifier.
- Mail-provider credentials remain Server environment secrets and are never
  exposed in an API response or frontend build argument.
- The shared mail sender supports the existing SMTP transport and Tencent SES
  API. Tencent SES uses one approved transactional template with `intro` and
  `code` variables; registration and password reset continue to share the same
  challenge lifecycle. Development without a configured transport logs the
  one-time code. Production validates the selected transport and fails closed
  when its sender, template, or credentials are absent.
- The pairing command contains a ten-minute one-time enrollment token. The UI
  marks it as secret, displays it once, and never stores it after exchange.
- The installer never receives a user JWT, provider key, database URL, or Server
  JWT secret.
- Downloaded archives are checked against the release checksum before install.

## 5. Persistence and transactions

- Registration verification locks the selected challenge, validates attempts
  and expiry, consumes it, and inserts the user in one transaction.
- Password reset locks and consumes the challenge, updates the password, and
  deletes refresh sessions in one transaction.
- Sending email occurs after a challenge is committed because external mail
  cannot participate in a PostgreSQL transaction. If delivery fails, the new
  challenge is invalidated and the API returns service unavailable.
- Expired challenges are removed opportunistically and by the existing Server
  cleanup loop. No scheduler or queue is added.
- Release artifacts and checksums are immutable per tag. Re-running the
  installer is the V1 upgrade mechanism.

## 6. HTTP APIs

Public, JSON-only, IP-rate-limited routes:

| Route | Result |
|---|---|
| `POST /api/v1/auth/register` | validate details and send registration code; `202` |
| `POST /api/v1/auth/register/verify` | create verified user and return the existing auth response |
| `POST /api/v1/auth/password/forgot` | send reset code when eligible; always generic `202` |
| `POST /api/v1/auth/password/reset` | consume code and set password; `204` |
| `POST /api/v1/auth/login` | unchanged email/password login |
| `POST /api/v1/auth/refresh` | unchanged rotating refresh token |

Bodies use strict size limits and reject unknown oversized input. Verification
errors are deliberately generic. The existing protected routes remain
unchanged.

## 7. Frontend state and UX

### Registration

The existing registration form remains the first step. A successful submission
switches the same page to a six-digit verification step showing the normalized
email, resend action, back action, expiry guidance, and explicit mail-delivery
errors. Only verification stores auth tokens and redirects to onboarding.

### Login and recovery

Login remains email/password and gains a visible “Forgot password?” link. The
recovery page has two states: request code, then code plus new password. Success
returns to login with confirmation. Loading, cooldown, validation, and API
errors are visible and keyboard accessible.

### Computer setup

The Computers dialog displays a clean-machine command based on the public
Server origin:

```sh
curl -fsSL https://raw.githubusercontent.com/solo-agent/solo/master/scripts/install.sh | \
  bash -s -- connect --server 'https://solo.example.com' \
  --computer-id '...' --token '...'
```

It also displays the shorter command for an existing installation. Copy success
and the one-time/expiry warning are visible. Online state remains the final
proof that setup worked.

## 8. Release and version compatibility

GoReleaser builds `solo` and `solo-daemon` for Darwin/Linux on amd64/arm64 and
places both in one archive with SHA-256 checksums. A tag-triggered workflow
publishes those artifacts to GitHub Releases. The installer supports an
explicit `SOLO_VERSION` for deterministic installs and otherwise resolves the
latest release.

Both binaries receive the release version through Go linker flags. The Daemon
reports that version during handshake and in `/health`. Protocol version remains
independent. An unsupported protocol continues to fail explicitly; a different
application version remains observable but does not block compatible clients.

## 9. Failure recovery

- Mail unavailable: production startup fails when the selected transport is
  unconfigured; runtime provider, template-review, quota, and network failures
  return the existing retryable service-unavailable response and invalidate the
  challenge. A user retry creates a fresh challenge after the cooldown.
- Code lost/expired: the user resubmits registration or requests another reset
  code after cooldown. Old codes remain unusable.
- Installer network failure: no existing installed binary is overwritten until
  download and checksum verification succeed.
- Pairing failure: the command exits non-zero, preserves diagnostic output, and
  does not invent an online state. The user can generate a new enrollment token.
- Daemon crash: the managed command can restart it; remote Run persistence and
  redelivery continue to follow Remote Runtime V1.
- Server/Daemon version mismatch: the Computers page exposes reported versions;
  protocol incompatibility remains an explicit handshake error.

## 10. Configuration and deployment

Production adds:

- `PUBLIC_URL` for operator-visible links and generated setup commands;
- `AUTH_MAIL_TRANSPORT` (`smtp` or `tencent_ses`);
- SMTP deployments keep `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`,
  `SMTP_PASSWORD`, `SMTP_FROM`;
- Tencent SES deployments use `TENCENTCLOUD_SECRET_ID`,
  `TENCENTCLOUD_SECRET_KEY`, `TENCENT_SES_REGION`, `TENCENT_SES_FROM`, and
  `TENCENT_SES_TEMPLATE_ID`;
- `ALLOW_SIGNUP` (default true);
- `ALLOWED_EMAILS` and `ALLOWED_EMAIL_DOMAINS` (comma-separated).

The Compose stack passes these only to the Server. `deploy/remote/.env.example`
documents every required value. The Server continues to require a strong JWT
secret, explicit CORS origins, persistent storage, migrations, TLS termination,
and private PostgreSQL networking.

## 11. Migration and compatibility

1. Add `email_verified_at` nullable, then backfill existing users from
   `created_at`, then require it for newly created accounts in application code.
2. Add `auth_email_challenges` and its lookup/cleanup indexes.
3. Existing passwords, sessions, user IDs, onboarding Channels, Computers,
   machine credentials, Agents, Runs, and messages are unchanged.
4. Switching mail transports needs no database migration and does not change
   existing users, challenges, sessions, or frontend state.
5. Existing source-built Daemons continue working. Packaged Daemons use the same
   environment and credential format.
6. Existing local development registration remains testable through the logged
   development code; production never returns a code in an HTTP response.

## 12. Acceptance matrix

Validation uses the real frontend, API Server, PostgreSQL, SMTP receiver, local
Daemon, and a real local Agent runtime. No HTTP or database behavior is mocked.

1. A new allowed email receives a code, verifies, logs in, and has persisted
   `email_verified_at` plus onboarding data.
2. A wrong, expired, reused, or over-attempted code cannot create/reset an
   account.
3. Registration policy blocks only new disallowed accounts; an existing user can
   still log in.
4. Password recovery changes the real bcrypt hash and revokes refresh sessions.
5. A generated release archive installs into a clean temporary home, checksum
   verification succeeds, and both binaries report the same version.
6. The Computers-page command pairs the real Daemon; the UI and database show
   the same Computer online and credential state.
7. A message sent through the real UI creates a durable remote Run, executes in
   the paired local Agent runtime, returns a visible message, and persists the
   terminal Run/event state.
8. Stopping and restarting the managed Daemon preserves the credential and
   restores the Computer connection.

## 13. Localhost registration convenience

### 13.1 Domain model and ownership

Local registration convenience is an ephemeral development capability, not a
second User type or a persisted account flag. `SOLO_DEV_AUTO_VERIFY_LOCAL`
belongs to the make-managed Server process. The Server remains the only owner
of registration challenges and the existing verified-User transaction remains
the only path that creates a User, personal Workspace, `general` and Lucy
Channels, Public membership, and an authenticated session.

### 13.2 Lifecycle and data flow

`make rebuild` enables the capability for its development Server. For a request
whose TCP peer, API Host, browser Origin, and forwarded client/host headers (if
present) all resolve to loopback, `POST /auth/register` creates the normal
hashed challenge but does not call the mail transport. Its `202` response
includes the short-lived development code and declares that manual email
verification is not required. The registration page immediately posts that
code to the unchanged `/auth/register/verify` route, stores the returned tokens,
selects the personal Workspace, and enters onboarding without rendering the
verification form.

Any non-loopback signal, a disabled capability, or `APP_ENV=production` uses
the existing email-delivery lifecycle. The frontend does not infer trust from
its own URL and cannot request the bypass with a body field.

### 13.3 Persistence and recovery

The development flow persists the same HMAC challenge and consumes it in the
same PostgreSQL transaction as remote registration. There is no schema change,
no unverified User, and no development-only value stored on the User. If the
automatic verify request is interrupted, resubmitting registration after the
existing cooldown creates a fresh challenge; an already completed account
continues through normal login.

### 13.4 HTTP API and frontend state

`GET /auth/config` reports `email_verification=false` only for an eligible
localhost request. The registration `202` response may additionally contain
`verification_code` only under that same Server-side predicate. Remote
responses retain their existing shape apart from an explicit
`email_verification=true` field and never expose a raw code.

`AuthContext.register` returns the optional local code to the registration
page. When present, the page keeps its submitting state, invokes the existing
verification action, and redirects exactly as the manual form does. When
absent, the current pending-email, resend, edit-address, and error states are
unchanged.

### 13.5 Failure boundaries and compatibility

- A public hostname routed to the same make-managed Server does not qualify;
  it sends mail and requires the code.
- Reverse-proxy requests qualify only when forwarded client and host values are
  also loopback; production mode disables the feature unconditionally.
- A malformed or expired returned code fails in the existing generic verify
  state and creates no User.
- Password reset still requires email verification, including on localhost;
  this convenience applies only to first-time registration.
- Existing remote deployments, users, challenges, sessions, Daemons, and
  database migrations are compatible without changes.

### 13.6 Acceptance

Validation uses the make-managed frontend, API Server, and PostgreSQL without
mocks. A browser opened at `http://localhost:3000` must submit the registration
form once, never render the verification-code UI, arrive at the dashboard, and
leave a verified User, session, personal Workspace, Public membership, and
ordinary-Channel memberships in PostgreSQL. Unit coverage separately proves
that production, public Host/Origin, and public forwarded headers cannot enable
the shortcut.

## 14. Verification results

Completed against the real repository stack on 2026-08-07. Mail delivery below
uses a real SMTP exchange with Mailpit; it does not claim delivery through the
production provider:

- `go test ./...`: all Go packages passed.
- `npm run build`: production frontend build passed, including registration,
  password recovery, Computers, and dashboard routes.
- `npm run lint`: zero errors; existing repository warnings remain.
- `make test-release-install`: a real Darwin/arm64 archive was built, checksummed,
  installed into a clean temporary home, and both binaries reported the linked
  release version.
- the official GoReleaser v2 image accepted the release configuration and built
  snapshot archives for Darwin/Linux on amd64/arm64, each containing `solo` and
  `solo-daemon`.
- `make test-e2e-public-remote`: the real browser, API, PostgreSQL, and Mailpit
  path verified delivered registration mail, email verification, persisted
  onboarding data, login, password reset, session revocation, and both Computer
  setup commands.
- `make test-e2e-remote-server`: the real browser, API, PostgreSQL, outbound
  Daemon control connection, and an installed local Agent runtime verified
  pairing, online execution, protected attachments and runtime RPC, an offline
  queued Run, reconnect redelivery, visible result, and persisted terminal state.

Project lifecycle rules require E2E Daemon restarts to use `make rebuild`, so
the clean installer and managed CLI process lifecycle are validated separately
from the real remote execution test. They share the same binaries, enrollment
API, credential file, and control protocol; `solo daemon connect` additionally
waits for `/health` to report a ready control channel before reporting success.
