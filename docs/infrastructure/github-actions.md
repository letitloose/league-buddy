# GitHub Actions (`.github/workflows/main.yml`)

Triggers on every `push`. Two active jobs, one commented-out job.

## `test`

Spins up a `mariadb:latest` service container (fixed throwaway credentials, port `3308`, health-checked via `healthcheck.sh`), then runs:

```bash
cd application && go test -p 1 ./...
```

`-p 1` disables parallel package execution — the model and handler tests all share the one service-container database, so parallel packages would race on schema setup/teardown.

## `build-and-push`

Runs only after `test` passes, and only `if: github.ref == 'refs/heads/main'`. Logs into GHCR with the auto-provided `secrets.GITHUB_TOKEN` (no manual PAT needed — the `packages: write` permission at the workflow level is what authorizes the push), then builds and pushes `ghcr.io/letitloose/league-buddy:latest` via `docker/build-push-action@v5`.

## `deploy` (commented out)

Written but not active — there's no production server yet. The commented block SSHes into a host via `appleboy/ssh-action`, `git pull`s, stamps a `SOFTWARE_LAST_UPDATE` timestamp into `.env`, and runs `docker compose pull && docker compose up -d`. To activate it:

1. Uncomment the `deploy:` block.
2. Add repo secrets `SERVER_IP`, `SERVER_USERNAME`, `SERVER_KEY` (SSH private key for a user that can `docker compose` on the target host).
3. Ensure the target host already has this repo cloned at `~/league-buddy` (or adjust the `cd` in the script) and `nginx-compose.yml` running for TLS/routing.
