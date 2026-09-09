# League Buddy ("Blame the Ball")

A web app for running a multi-team recreational sports league: leagues, seasons, matches, standings and leaders, self-registration with team invites/join-requests, RSVPs and automated email/text reminders, captain/scorekeeper delegation, roster PDF export and CSV import, and a per-player calendar subscription feed.

Built as a sibling project to `toller-club-docker`, reusing its Docker setup, CI/CD pipeline, and Go architecture (users/auth, email, middleware, CRUD patterns) as a starting point — the domain has since grown well past that scaffold. See [docs/](docs/) for architecture, security, and integration details.

## Stack

Go (no framework — `httprouter` + `alice` middleware chaining), server-rendered `html/template`, MariaDB, Tailwind CSS (compiled, embedded into the binary), Docker Compose.

## Local development

### 1. One-time setup

Fetch the standalone Tailwind CLI binary (used by Air's hot-reload build step; not needed for Docker/CI, since compiled CSS is committed):

```bash
mkdir -p tools
# pick the line matching your platform:
curl -sL https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-x64   -o tools/tailwindcss   # Linux x64
curl -sL https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-linux-arm64 -o tools/tailwindcss   # Linux arm64
curl -sL https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-x64   -o tools/tailwindcss   # macOS Intel
curl -sL https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-arm64 -o tools/tailwindcss   # macOS Apple Silicon
chmod +x tools/tailwindcss
```

Create `.env.dev` (gitignored) — see [Environment variables](#environment-variables) below, or copy `.env.example` as a starting point.

### 2. Run it

```bash
docker compose -f docker-compose-dev.yml up
```

This starts MariaDB and the app under [Air](https://github.com/cosmtrek/air) for hot reload — editing any `.go`, `.html`, `.css`, or `.js` file triggers an automatic rebuild (Tailwind recompiles first) and browser refresh via Air's built-in proxy. With `RESETDB=true` (the default in the env vars below), the schema is rebuilt and a default admin account is created on every boot.

Visit `http://localhost:8081`. Log in with `LEAGUEBUDDYUSER`/`LEAGUEBUDDYPASSWORD` from your `.env.dev`.

### 3. Run the tests

```bash
docker compose -f docker-compose-test-db.yml up -d
cd application && go test -p 1 ./...
```

`-p 1` is required — the model, service, and route tests all share one database, so parallel packages would race on schema setup/teardown.

### Debugging

`docker compose -f docker-compose-debug.yml up` runs the app under [Delve](https://github.com/go-delve/delve) instead of Air, listening on port `2346` for a remote debugger attach.

## Environment variables

| Variable | Purpose |
|---|---|
| `DBHOST`, `DBPORT`, `MYSQL_DATABASE`, `MYSQL_USER`, `MYSQL_PASSWORD`, `MYSQL_ROOT_PASSWORD` | DB connection + container bootstrap |
| `RESETDB` | `true` tears down/reseeds the schema + creates the default admin (and dev seed data) on boot. Never set `true` against real data. |
| `LEAGUEBUDDYUSER`, `LEAGUEBUDDYPASSWORD` | Default admin account (created only when `RESETDB=true`) |
| `LEAGUEBUDDY_LEAGUEADMIN_EMAIL`/`_PASSWORD`, `LEAGUEBUDDY_CAPTAIN_EMAIL`/`_PASSWORD`, `LEAGUEBUDDY_PLAYER_EMAIL`/`_PASSWORD` | Optional test logins, one per role below system admin (each pair independently optional), created only when `RESETDB=true` |
| `EMAIL_USER`, `EMAIL_PASSWORD`, `EMAIL_SENDER` | Mailjet credentials — leave `EMAIL_USER` unset to skip email sending (not fatal) |
| `SMS_FEATURE_ENABLED` | Whether the phone-verification/notification-preferences UI is shown at all — kept independent of the credentials below so real Twilio creds can sit configured while a toll-free number is still pending carrier approval |
| `SMS_ACCOUNT_SID`, `SMS_AUTH_TOKEN`, `SMS_FROM_NUMBER` | Twilio credentials for text reminders/phone verification — leave `SMS_ACCOUNT_SID` unset to skip SMS sending (codes/reminders are logged instead, not fatal) |
| `SITE_HOST`, `SITE_PORT` | Bind address for the Go HTTP server |
| `PUBLIC_HOST` | The single canonical hostname the app itself uses to build links in emails and the calendar feed — kept separate from `VIRTUAL_HOST` since that one can be a comma-separated list |
| `VIRTUAL_HOST` | Read by nginx-proxy for routing and by its Let's Encrypt companion for cert issuance — not read by the app itself; may be a comma-separated list |
| `LETSENCRYPT_HOST`, `LETSENCRYPT_EMAIL` | TLS cert target (prod only) |
| `MIGRATION_PATH` | Directory of `.sql` files applied on boot |
| `INFO_LOG`, `ERROR_LOG` | Optional log file paths (default stdout/stderr) |
| `SOFTWARE_LAST_UPDATE` | Footer timestamp, stamped automatically by the deploy job on every push to `main` |

See `.env.example` for a fully-annotated placeholder copy. Real `.env`/`.env.dev`/`.env.testdb` files are gitignored — never commit real credentials.

## Production deploy

```bash
docker network create shared-net   # one-time, shared with any other app on the host
docker compose -f nginx-compose.yml up -d
docker compose up -d
```

CI builds and pushes `ghcr.io/letitloose/league-buddy:latest` on every push to `main`, then deploys it automatically: the workflow SSHes into the production server, pulls the repo, stamps `SOFTWARE_LAST_UPDATE`, and runs `docker compose pull && docker compose up -d` (see `.github/workflows/main.yml` and `docs/infrastructure/github-actions.md`).

## What's here vs. what's deferred

Leagues, seasons, matches, standings/leaders, RSVPs with automated email/text reminders, captain and scorekeeper delegation, and self-registration via invites/join-requests are all fully built. PayPal and a generic (beyond Admin) role picker are the main things intentionally deferred — see the "Future Work" section of the architecture doc: [docs/application/architecture-overview.md](docs/application/architecture-overview.md#future-work).
