# Middleware Chain

Every HTTP request passes through two layers of middleware before reaching a handler. The outer layer runs unconditionally; the inner layer is composed per-route based on what that route requires. Ported directly from `toller-club-docker` — this chain is domain-agnostic.

## The Two Layers

```
Request
   │
   ▼
┌─────────────────────────────────────────┐  ← standard (every request)
│  recoverPanic → logRequest → secureHeaders
└───────────────────────┬─────────────────┘
                        │
                        ▼
              ┌─────────────────────────────┐  ← dynamic (every non-static request)
              │  sessionManager.LoadAndSave  │
              │  noSurf (CSRF)               │
              │  authenticate                │
              └──────────────┬──────────────┘
                             │
                    ┌────────┴──────────┐
                    │  route-specific   │
                    │  requireActive    │
                    │  requireAdmin     │
                    │  requireAuthentication
                    │  requireTeamManager
                    │  requireLeagueManager
                    └───────────────────┘
                             │
                             ▼
                          Handler
```

## Standard Chain (Every Request)

Wired with `alice.New(app.recoverPanic, app.logRequest, secureHeaders).Then(router)`. Runs before routing.

### `recoverPanic`

Wraps the entire request in a deferred `recover()`. If any handler panics, the connection is closed with a `Connection: close` header and a 500 response is sent.

### `logRequest`

Logs every non-static request to the info log: `<remote-addr> - <proto> <method> <uri>`. Requests to `/static/...` are filtered out.

### `secureHeaders`

Sets security-related response headers on every response. The CSP here is intentionally tighter than the reference project's — no PayPal, FontAwesome, or Tailwind-CDN origins are allowlisted, since none of those are used in v1.

| Header | Value | Purpose |
|---|---|---|
| `Content-Security-Policy` | `default-src 'self'` (plus inline styles) | Blocks XSS and injection |
| `Referrer-Policy` | `origin-when-cross-origin` | Limits referrer data leakage |
| `X-Content-Type-Options` | `nosniff` | Prevents MIME sniffing |
| `X-Frame-Options` | `deny` | Prevents clickjacking |
| `X-XSS-Protection` | `0` | Disabled — CSP is the correct modern approach |

## Dynamic Chain (All Page Routes)

Built as `alice.New(sessionManager.LoadAndSave, noSurf, app.authenticate)`.

### `sessionManager.LoadAndSave`

Loads the session from the MySQL-backed SCS session store at the start of the request and commits it at the end.

### `noSurf`

Generates and validates CSRF tokens for all state-changing requests. The token is stored in a `Secure`, `HttpOnly` cookie and must match the value submitted in the form (hidden `{{.CSRFToken}}` field) or the `X-CSRF-Token` header (for DELETE/JSON requests).

The CSRF cookie is `Secure`-only, so the application requires TLS in production. Tests use `httptest.NewTLSServer`.

### `authenticate`

Reads `authenticatedUserID` from the session. If present, calls `userService.GetAuthContext(id)` — a single SQL query that fetches the user's active flag, admin flag, and linked player (if any) — and populates the request context with everything the authorization helpers below read:

| Context key | Type | Set when |
|---|---|---|
| `isAuthenticated` | bool | Any logged-in user |
| `isActive` | bool | `users.active = true` |
| `isAdmin` | bool | Has ADMIN role (suppressed while viewing as player — see below) |
| `realIsAdmin` | bool | True admin status, unaffected by "view as player" |
| `viewingAsPlayer` | bool | Admin has toggled "view as player" |
| `playerID` | int | Linked player record exists |
| `teamIDs` | []int | Every team the linked player belongs to (`teamMembers` — a player can be on more than one) |
| `captainTeamIDs` | []int | Teams where the linked player is `captainPlayerID` |
| `scorekeeperTeamIDs` | []int | Teams where the linked player is a scorekeeper (`teamScorekeepers`) |
| `leagueAdminLeagueIDs` | []int | Leagues the linked player administers (`leagueAdmins`) |
| `leagueAdminTeamIDs` | []int | Teams belonging to any league in `leagueAdminLeagueIDs` (precomputed for `canManageTeam`) |
| `userName` | string | Always set (player name or email) |

**View as player**: an admin can toggle `POST /user/toggleViewAsPlayer` to browse the site as their own linked player would see it — every elevated field above except `realIsAdmin` is suppressed while this is on, so the UI and every in-handler authorization check behave exactly as they would for that player. `realIsAdmin` stays true so the "switch back to admin" control can still render.

This middleware never redirects. It only populates context.

## Route-Specific Middleware

### `requireAuthentication`

Used for `/user/logout`. Redirects to `/user/login` if `isAuthenticated` is false.

### `requireActive`

Used for player-facing routes. Redirects to `/` if `isActive` is false. Sets `Cache-Control: no-store`.

### `requireAdmin`

Used for administrative routes. Redirects to `/` if `isAdmin` is false. Sets `Cache-Control: no-store`.

### `requireTeamManager`

Used for team-scoped roster/invite/join-request management routes (`/team/:teamID/player/create`, `/team/:teamID/invite`, `/team/:teamID/joinRequests`, etc.), chained after `requireActive`. Reads `:teamID` from the route params (via `httprouter.ParamsFromContext` — params are visible to middleware earlier in the alice chain, not just the final handler, since the whole chain is registered as "the handler" with httprouter). 404s if `:teamID` doesn't parse to a positive integer. Otherwise allows the request through if `canManageTeam` (below) is true for that team — i.e. an admin can manage any team, a captain or league admin only their own. Everyone else is redirected to `/`. Sets `Cache-Control: no-store`.

### `requireLeagueManager`

Used only for team deletion (`DELETE /admin/team/delete/:teamID`) — deliberately excludes plain captains, since deleting a team is more destructive than editing or managing its roster. Same `:teamID`-resolution shape as `requireTeamManager`, but gates on `canDeleteTeam` (admin or that team's league admin) instead of `canManageTeam`.

## In-Handler Authorization Helpers

Several routes carry their scoping ID (a `leagueID`/`teamID`) in the POST body rather than the URL, so there's no route param for the middleware tiers above to key off of — these routes sit on the plain `active` tier and check authorization themselves, in `cmd/web/helpers.go` unless noted. All of them are boolean predicates over the context keys `authenticate` set above, not additional database queries.

| Function | Checks | Typical use |
|---|---|---|
| `canManageTeam` | admin, or captain/league-admin of that team | `requireTeamManager`, and team-scoped POST routes (`/admin/team/update`, `/admin/team/setCaptain`, scorekeeper/Legend toggles) |
| `canManageLeague` | admin, or league admin of that league | league-scoped POST routes (season/match create, team create) |
| `canDeleteTeam` | admin, or league admin of that team (excludes plain captain) | `requireLeagueManager` |
| `canInviteAsCaptain` | admin, or league admin of that team (excludes plain captain) | the invite form's "invite as captain" checkbox |
| `isMemberOfTeam` | player belongs to that team | RSVP eligibility and similar own-team checks |
| `canRSVPAsTeam` | member of the team, and not a Legend | `matchRSVPSubmit` |
| `canManageMatchSide` | same as `canManageTeam`, for one side of a match | Player-of-the-Match/captain-notes editing |
| `canManageAttendanceSide` | `canManageMatchSide`, or a scorekeeper of that side | match attendance overrides |
| `canManagePlayer` (`handlers_players.go`) | admin, the player's own account, or a manager of any team the player is on | player profile view/edit |
| `isTeammateOfPlayer` (`handlers_players.go`) | viewer shares any active team roster with the player | gates a player's private bio fields (email/phone/DOB/address) |
| `canManageMatch` / `canDeleteMatch` (`handlers_matches.go`) | admin, league admin, captain, or (manage only) scorekeeper of either side | match score/goals/cards editing vs. deletion |
| `requireOwnPlayer` (`handlers_playerNotifications.go`) | strictly the logged-in user's own linked player — no admin/captain override | every Notification Preferences route (consent isn't something anyone else can grant on a player's behalf) |
| `smsFeatureEnabled` | `SMS_FEATURE_ENABLED` env var | whether the phone-verification/notification-preferences UI is shown at all |

## Static Files

Static files (`/static/*filepath`) bypass the dynamic chain entirely — no session or CSRF middleware.
