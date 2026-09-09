# GitHub Actions (`.github/workflows/main.yml`)

Triggers on every `push`. Three jobs, all active: `test` → `build-and-push` → `deploy`, each gated on the previous one succeeding.

## `test`

Spins up a `mariadb:latest` service container (fixed throwaway credentials, port `3308`, health-checked via `healthcheck.sh`), then runs:

```bash
cd application && go test -p 1 ./...
```

`-p 1` disables parallel package execution — the model and handler tests all share the one service-container database, so parallel packages would race on schema setup/teardown.

## `build-and-push`

Runs only after `test` passes, and only `if: github.ref == 'refs/heads/main'`. Logs into GHCR with the auto-provided `secrets.GITHUB_TOKEN` (no manual PAT needed — the `packages: write` permission at the workflow level is what authorizes the push), then builds and pushes `ghcr.io/letitloose/league-buddy:latest` via `docker/build-push-action@v5`.

## `deploy`

Runs only after `build-and-push` succeeds, and only `if: github.ref == 'refs/heads/main'` — so every push to `main` that passes tests goes live automatically, with no manual deploy step. SSHes into the production host via `appleboy/ssh-action`, then:

```bash
echo "$GHCR_TOKEN" | docker login ghcr.io -u <actor> --password-stdin
cd league-buddy
git pull
sed -i~ '/^SOFTWARE_LAST_UPDATE=/s/=.*/=<current date/time>/' .env
docker compose pull
docker compose up -d
```

Requires three repo secrets: `SERVER_IP`, `SERVER_USERNAME`, `SERVER_KEY` (an SSH private key for a user that can run `docker compose` on the target host). The target host is expected to already have this repo cloned at `~/league-buddy` and `nginx-compose.yml` running for TLS/routing (see [Docker Compose](./docker-compose.md)).

Because this runs unattended on every push to `main`, treat a merge to `main` as a production deploy — there's no separate "promote to prod" step to catch a bad change before it goes live beyond the `test` job.
