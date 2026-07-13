# Docker Compose

Four compose files cover different scenarios, plus a fifth (`nginx-compose.yml`) for the always-on reverse proxy stack. Ported structurally from `toller-club-docker`, with distinct host ports so both projects can run on the same machine concurrently.

| File | Purpose | Host ports |
|---|---|---|
| `docker-compose.yml` | Production. Pulls the prebuilt `ghcr.io/letitloose/league-buddy:latest` image. | DB `3308`, app `127.0.0.1:8081` |
| `docker-compose-dev.yml` | Local dev. Runs the Go source through `cosmtrek/air` for hot reload. | DB `3308`, app `8081` |
| `docker-compose-debug.yml` | Local dev with the `dlv` debugger attached (`Dockerfile.debug`). | DB `3308`, app `8081`, debugger `2346` |
| `docker-compose-test-db.yml` | A throwaway MariaDB instance for local `go test` runs. | DB `3308` |
| `nginx-compose.yml` | `nginx-proxy` + `acme-companion` — always-on reverse proxy and Let's Encrypt automation, shared across every app on the host via the external `shared-net` network. | `80`, `443` |

## `shared-net`

`docker-compose.yml` and `nginx-compose.yml` both join an **external** Docker network called `shared-net`, which must exist before either stack starts:

```bash
docker network create shared-net
```

This lets one nginx-proxy instance route to multiple independent app stacks (e.g. league-buddy alongside toller-club-docker) purely by Docker label (`VIRTUAL_HOST` / `LETSENCRYPT_HOST` in each app's `.env`), with no nginx config changes required per app.

## Env files

Each compose file's `env_file:` points at a different, gitignored env file (`.env`, `.env.dev`, `.env.testdb`). See the root `README.md` for the full variable list. `.env.example` is the only committed env file — a placeholder-only reference copy.
