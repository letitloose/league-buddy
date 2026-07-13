# League Buddy

A web app for administering a sports team's roster — self-registration, an admin-managed player roster, and the infrastructure to grow into a multi-team league later.

Built as a sibling project to `toller-club-docker`, reusing its Docker setup, CI/CD pipeline, and Go architecture (users/auth, email, middleware, CRUD patterns). See [docs/](docs/) for architecture, security, and integration details.

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
| `RESETDB` | `true` tears down/reseeds the schema + creates the default admin on boot. Never set `true` against real data. |
| `LEAGUEBUDDYUSER`, `LEAGUEBUDDYPASSWORD` | Default admin account (created only when `RESETDB=true`) |
| `TEAM_NAME` | Name for the single seeded team |
| `EMAIL_USER`, `EMAIL_PASSWORD`, `EMAIL_SENDER` | Mailjet credentials — leave `EMAIL_USER` unset to skip email sending (not fatal) |
| `SITE_HOST`, `SITE_PORT` | Bind address for the Go HTTP server |
| `VIRTUAL_HOST` | Public hostname used in activation/reset email links, and as the nginx-proxy routing label in prod |
| `LETSENCRYPT_HOST` | TLS cert target (prod only) |
| `MIGRATION_PATH` | Directory of `.sql` files applied on boot |
| `INFO_LOG`, `ERROR_LOG` | Optional log file paths (default stdout/stderr) |
| `SOFTWARE_LAST_UPDATE` | Footer timestamp, stamped by the (currently disabled) deploy job |

See `.env.example` for a fully-annotated placeholder copy. Real `.env`/`.env.dev`/`.env.testdb` files are gitignored — never commit real credentials.

## Production deploy

```bash
docker network create shared-net   # one-time, shared with any other app on the host
docker compose -f nginx-compose.yml up -d
docker compose up -d
```

CI builds and pushes `ghcr.io/letitloose/league-buddy:latest` on every push to `main` (see `.github/workflows/main.yml`). The automated deploy job is written but commented out until a production server exists — see `docs/infrastructure/github-actions.md` for activation steps.

## What's here vs. what's deferred

Single-team roster administration with self-registration is fully built. PayPal, multi-team/League UI, and a few other things are intentionally deferred — see the "Explicitly deferred" section of the architecture doc: [docs/application/architecture-overview.md](docs/application/architecture-overview.md#future-work).
