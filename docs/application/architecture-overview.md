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
| SMS | Twilio Messages API (raw REST, no SDK) |

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
| `helpers.go` | `render`, `serverError`, template data builder, and most in-handler authorization helpers (`canManageTeam`, `canManageLeague`, `isMemberOfTeam`, `smsFeatureEnabled`, etc. — see [Middleware Chain](./middleware.md)) |
| `templates.go` | Template cache construction, `templateData` struct, date/time formatting helpers |
| `handlers_site.go` | Home dashboard, privacy/terms/contact pages, captain guide, SMS opt-in proof page |
| `handlers_users.go` | Signup (including invite-token threading), login (including Remember Me), logout, password reset, admin user management |
| `handlers_players.go` | Player profile CRUD, career stats, `canManagePlayer`/`isTeammateOfPlayer` authorization |
| `handlers_playerNotifications.go` | A player's own Notification Preferences page: phone verification, SMS program opt-in/opt-out, per-category email/text delivery preferences, calendar token regeneration |
| `handlers_leagues.go` | League CRUD, browse, and the league detail page (standings, leaders, season picker) |
| `handlers_seasons.go` | Season CRUD and CSV schedule import |
| `handlers_matches.go` | Match CRUD, the match detail page (RSVPs, goals, cards, captain notes, attendance), and reminder test-send endpoints — the largest handler file |
| `handlers_teams.go` | Team CRUD, roster PDF export/CSV import, captain/scorekeeper/Legend-status management, invites, join requests — the second-largest handler file |
| `handlers_locations.go` | Admin CRUD for home-field locations |
| `handlers_calendar.go` | The token-authenticated, cookie-less per-player calendar feed (`GET /calendar/:token/schedule.ics`) |

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

The base schema is defined in `sql/setup.sql` (18 tables — `sessions`, `migration`, `leagues`, `teams`, `address`, `locations`, `players`, `teamMembers`, `leagueAdmins`, `seasons`, `matches`, `playerMatchStats`, `users`, `roles`, `userRole`, `auditLog`, `invites`, `teamJoinRequests`). `sql/migrations/` is a genuine, actively-growing history from there — 20 files as of this writing, layering on RSVPs, goals/cards, team notes, reminder settings and their delivery-tracking tables, per-player attendance overrides, notification preferences and phone verification, Legend roster status, calendar tokens, and SMS program opt-in, among others. Migrations are applied automatically on startup by `MigrationModel.PerformMigrations()`, which tracks applied files in a `migration` table — safe to run repeatedly, since already-applied files are skipped.

The schema supports real multi-team leagues: a `leagues` table (top-level scope) and a `teams` table (belongs to one league, has zero or one captain via `captainPlayerID`). Team membership is **not** a column on `players` — it's the `teamMembers` junction table (`playerID`, `teamID`, `joinedAt`), so a player can belong to more than one team at once (only "at most one team per league" is enforced, and only in the service layer, not by a DB constraint). A self-registered player starts on no team at all until an invite or an approved join request assigns one. `leagueAdmins` is a similar many-to-many junction — a player can administer more than one league, and a league can have more than one admin. Two more supporting tables round out registration: `invites` (single-use signup tokens a captain/admin emails to a prospective player — the invited team is granted regardless of what email address the person actually registers with) and `teamJoinRequests` (an unaffiliated player's request to join a team, approved/rejected by that team's captain or any admin). `setup.sql` seeds exactly one league and one team on a fresh install; the dev-only `RESETDB=true` path layers on realistic seed data (a full roster, historical seasons/results, test logins for every role) on top of that — see `cmd/web/seed_roster.go`/`seed_historical.go`, never run outside local dev.

## Configuration

All configuration is passed via environment variables (never compiled in). Key variables:

| Variable | Purpose |
|---|---|
| `DBHOST`, `DBPORT`, `MYSQL_*` | Database connection |
| `EMAIL_USER`, `EMAIL_PASSWORD`, `EMAIL_SENDER` | Mailjet credentials (unset = email sending skipped, not fatal) |
| `SMS_FEATURE_ENABLED`, `SMS_ACCOUNT_SID`, `SMS_AUTH_TOKEN`, `SMS_FROM_NUMBER` | Twilio credentials and a separate site-wide feature flag (unset SID = SMS sending skipped, not fatal — see [Integrations](../integrations/integrations.md)) |
| `PUBLIC_HOST` | Hostname the app itself uses to build links (email, calendar feed) |
| `VIRTUAL_HOST` | nginx-proxy/Let's Encrypt routing label only, not read by the app |
| `SITE_HOST`, `SITE_PORT` | Bind address |
| `MIGRATION_PATH` | Directory of `.sql` files applied on boot |
| `RESETDB=true` | Tears down and re-seeds the database on startup |

## Routing Constraints

`httprouter` v1.3.0 refuses to register a static route (e.g. `/league/create`) and a wildcard route (e.g. `/league/:id`) at the same path depth for the same HTTP method — it panics at startup with a wildcard conflict. This is why the admin-only league/team create, update, delete, and set-captain routes live under `/admin/league/...` and `/admin/team/...` rather than directly under `/league/...`/`/team/...`: the view routes (`/league/:id`, `/team/:teamID`, and everything nested under it) need `:id`/`:teamID` to be the *sole* child of that path segment. When adding new routes, either nest a wildcard behind a static disambiguator (`/player/view/:id` is the established pattern) or give the new static route its own top-level prefix — never register it as a sibling of an existing bare wildcard.

## Future Work

Explicitly deferred, not forgotten:

- **PayPal integration** — not ported. The reference project (`toller-club-docker`) has a working `internal/services/paypal.go` to pull over when team payments are needed.
- **Generic role management UI** — the `roles`/`userRole` tables support more than one role, but only `ADMIN` is ever seeded and only a plain activate/deactivate + admin toggle exist; add a role picker if a role beyond Admin/Player is needed.
- **Invite token expiration** — only `usedAt` prevents reuse; there's no time-based expiry.
- **One captain per team** — `teams.captainPlayerID` is still unique per team, so a team has at most one captain (a player isn't otherwise restricted from captaining more than one). Scorekeepers (`teamScorekeepers`) now provide a second, non-exclusive tier of match-editing delegation below the captain, for teams that want to split that work up.
- **A global cross-team player directory for Admins** — rosters are browsed per-team like everyone else; only `/admin/joinRequests` is genuinely cross-team.
- **League/team deletion cascades** — blocked with `ErrHasDependents`; an admin must clear dependents (teams, then players) first.
- **Resubmission cooldowns** after a rejected join request, and **revoking/rate-limiting invites**.
- **An inbound-SMS webhook** — STOP/HELP replies are currently handled entirely by Twilio's own Advanced Opt-Out feature at the messaging-service level; the app has no code path that sees or records an inbound text itself.

## Further Reading

- [Middleware Chain](./middleware.md) — how requests flow through the middleware stack
- [Security Model](../security/security.md) — CSRF, sessions, headers, and bcrypt
- [Authentication Flow](../security/authentication.md) — signup, activation, login, and per-request context
- [Domain Summary](./domains.md) — what each domain covers
- [Integrations](../integrations/integrations.md) — Mailjet (and what's deferred)
