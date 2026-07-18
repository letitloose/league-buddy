# Architecture Overview

This document describes how League Buddy is structured end-to-end — from the browser to the database.

## The Big Picture

```
Internet
    │
    ▼
┌───────────────────────────────────────────────────────┐
│  Docker Network: shared-net                           │
│                                                       │
│  ┌──────────────┐     ┌──────────────────────────┐   │
│  │ nginx proxy  │────▶│ go-app (league-buddy)    │   │
│  │ :443 / :80   │     │ :8080                    │   │
│  │ TLS / LetsE  │     └───────────┬──────────────┘   │
│  └──────────────┘                 │                   │
│                      ┌────────────▼──────────┐        │
│                      │      MariaDB          │        │
│                      │      :3306 (backend)  │        │
│                      └───────────────────────┘        │
└───────────────────────────────────────────────────────┘
```

nginx is the public entry point. It terminates TLS and reverse-proxies all traffic to the Go application on `127.0.0.1:8081` (host) / `:8080` (container). nginx lives on a separate `shared-net` Docker network so it can proxy multiple apps from a single host — this project is designed to run alongside other apps (e.g. `toller-club-docker`) on the same machine.

## Technology Stack

| Layer | Technology |
|---|---|
| Language | Go |
| HTTP router | [httprouter](https://github.com/julienschmidt/httprouter) |
| Middleware chaining | [alice](https://github.com/justinas/alice) |
| Session management | [SCS v2](https://github.com/alexedwards/scs) (MySQL store) |
| CSRF protection | [nosurf](https://github.com/justinas/nosurf) |
| HTML templates | `html/template` (Go standard library) |
| Password hashing | bcrypt (via `golang.org/x/crypto`) |
| Database | MariaDB (via `go-sql-driver/mysql`) |
| CSS | Tailwind CSS (compiled, embedded) |
| Email | Mailjet API v3 |

## Application Layers

The Go application follows a three-layer architecture inside `application/`, ported from the sibling `toller-club-docker` project:

```
cmd/web/                    ← HTTP layer (handlers, routes, middleware)
internal/services/          ← Business logic layer
internal/models/            ← Database access layer
```

### `cmd/web/` — HTTP Layer

| File | Responsibility |
|---|---|
| `routes.go` | Registers all URL patterns with their middleware chains |
| `middleware.go` | CSRF, sessions, authentication, panic recovery, logging |
| `context.go` | Typed context keys for passing auth state through the request |
| `helpers.go` | `render`, `serverError`, `isAuthenticated`, template data builder |
| `templates.go` | Template cache construction, `templateData` struct |
| `handlers_site.go` | Home page |
| `handlers_users.go` | Signup (including invite-token threading), login, logout, password reset, user admin |
| `handlers_players.go` | Player (roster) CRUD, search — team-scoped |
| `handlers_leagues.go` | League CRUD, browse |
| `handlers_teams.go` | Team CRUD, captain assignment, invites, join requests |

### `internal/services/` — Business Logic Layer

Each service wraps one or more models and adds validation, business rules, and side-effects (sending emails, writing audit logs). Services are the only layer that knows about cross-domain behavior — for example, activating a user also links or creates a placeholder player record.

### `internal/models/` — Database Access Layer

Each model corresponds to a database table. Models execute SQL directly against `*sql.DB`. They know nothing about HTTP or business rules — they only read and write data.

## Static Assets and Templates

HTML templates and static files (`/static/`) are embedded into the binary at compile time using Go's `embed` package (via `ui/efs.go`). The deployed Docker image has no external file dependencies at runtime.

Templates use a base/partial/page structure:
- `html/base.html` — outer shell with `<head>`, nav, flash messages
- `html/partials/` — reusable fragments (nav, form field sets)
- `html/pages/` — one file per page, rendered into the base

## Database Schema

The schema is defined in full in `sql/setup.sql` (this is a fresh scaffold, not an accreted history — `sql/migrations/` starts empty and is reserved for genuine future changes). Migrations, if any, are applied automatically on startup by `MigrationModel.PerformMigrations()`, which tracks applied files in a `migration` table.

The schema supports real multi-team leagues: a `leagues` table (top-level scope), a `teams` table (belongs to one league, has zero or one captain via `captainPlayerID`), and a nullable `players.teamID` — a self-registered player starts unaffiliated until an invite or an approved join request assigns a team. Two supporting tables round this out: `invites` (single-use signup tokens a captain/admin emails to a prospective player — the invited team is granted regardless of what email address the person actually registers with) and `teamJoinRequests` (an unaffiliated player's request to join a team, approved/rejected by that team's captain or any admin). `setup.sql` seeds exactly one league and one team on a fresh install.

## Configuration

All configuration is passed via environment variables (never compiled in). Key variables:

| Variable | Purpose |
|---|---|
| `DBHOST`, `DBPORT`, `MYSQL_*` | Database connection |
| `EMAIL_USER`, `EMAIL_PASSWORD`, `EMAIL_SENDER` | Mailjet credentials (unset = email sending skipped, not fatal) |
| `VIRTUAL_HOST` | Public hostname (used in email links) |
| `SITE_HOST`, `SITE_PORT` | Bind address |
| `RESETDB=true` | Tears down and re-seeds the database on startup |

## Routing Constraints

`httprouter` v1.3.0 refuses to register a static route (e.g. `/league/create`) and a wildcard route (e.g. `/league/:id`) at the same path depth for the same HTTP method — it panics at startup with a wildcard conflict. This is why the admin-only league/team create, update, delete, and set-captain routes live under `/admin/league/...` and `/admin/team/...` rather than directly under `/league/...`/`/team/...`: the view routes (`/league/:id`, `/team/:teamID`, and everything nested under it) need `:id`/`:teamID` to be the *sole* child of that path segment. When adding new routes, either nest a wildcard behind a static disambiguator (`/player/view/:id` is the established pattern) or give the new static route its own top-level prefix — never register it as a sibling of an existing bare wildcard.

## Future Work

Explicitly deferred, not forgotten:

- **PayPal integration** — not ported. The reference project (`toller-club-docker`) has a working `internal/services/paypal.go` to pull over when team payments are needed.
- **Generic role management UI** — only an ADMIN toggle exists; add a role picker if a role beyond Admin/Player is needed.
- **Invite token expiration** — only `usedAt` prevents reuse; there's no time-based expiry.
- **Multiple captains per team, or one captain for multiple teams** — `teams.captainPlayerID` is unique and a team has at most one captain.
- **A global cross-team player directory for Admins** — rosters are browsed per-team like everyone else; only `/admin/joinRequests` is genuinely cross-team.
- **Direct admin reassignment of a player between teams** outside the invite/join-request flows — `PlayerModel.SetTeam` exists and could be exposed later.
- **League/team deletion cascades** — blocked with `ErrHasDependents`; an admin must clear dependents (teams, then players) first.
- **Resubmission cooldowns** after a rejected join request, and **revoking/rate-limiting invites**.

## Further Reading

- [Middleware Chain](./middleware.md) — how requests flow through the middleware stack
- [Security Model](../security/security.md) — CSRF, sessions, headers, and bcrypt
- [Authentication Flow](../security/authentication.md) — signup, activation, login, and per-request context
- [Domain Summary](./domains.md) — what each domain covers
- [Integrations](../integrations/integrations.md) — Mailjet (and what's deferred)
