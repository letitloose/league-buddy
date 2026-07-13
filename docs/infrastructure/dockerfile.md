# Dockerfile

Multi-stage build, ported from `toller-club-docker`:

1. **Build stage** (`golang:1.25`): copies `application/` in, `go mod download`, then `CGO_ENABLED=0 GOOS=linux go build -o /league-buddy ./cmd/web`. `CGO_ENABLED=0` produces a fully static binary with no libc dependency, which is what makes the tiny Alpine runtime stage possible.
2. **Runtime stage** (`alpine:latest`): installs only `ca-certificates` and `tzdata`, copies the compiled binary and the `sql/` directory (needed at runtime for `RESETDB=true` and the migration runner), and sets `ENTRYPOINT ["/root/league-buddy"]`.

Nothing else is copied into the final image — templates and static assets are already embedded in the binary via `ui/efs.go`, so the runtime image has no external file dependencies beyond `sql/`.

`EXPOSE 8080` matches the port the app actually listens on (`SITE_PORT`), which corrects a stale `EXPOSE 80` in the reference project's Dockerfile that didn't match its runtime behavior.

`Dockerfile.debug` is a separate, much simpler image (`golang:latest` + `dlv` installed) used only by `docker-compose-debug.yml`, which bind-mounts source and builds/runs it live under the Delve debugger rather than using a prebuilt binary.
