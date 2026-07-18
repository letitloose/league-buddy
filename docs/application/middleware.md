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

Reads `authenticatedUserID` from the session. If present, calls `userService.GetAuthContext(id)` — a single SQL query that fetches the user's active flag, admin flag, and linked player (if any).

| Context key | Type | Set when |
|---|---|---|
| `isAuthenticated` | bool | Any logged-in user |
| `isActive` | bool | `users.active = true` |
| `isAdmin` | bool | Has ADMIN role |
| `playerID` | int | Linked player record exists |
| `teamID` | int | Linked player has a team (`players.teamID` is set) |
| `isCaptain` | bool | Linked player is some team's `captainPlayerID` |
| `userName` | string | Always set (player name or email) |

This middleware never redirects. It only populates context.

## Route-Specific Middleware

### `requireAuthentication`

Used for `/user/logout`. Redirects to `/user/login` if `isAuthenticated` is false.

### `requireActive`

Used for player-facing routes. Redirects to `/` if `isActive` is false. Sets `Cache-Control: no-store`.

### `requireAdmin`

Used for administrative routes. Redirects to `/` if `isAdmin` is false. Sets `Cache-Control: no-store`.

### `requireTeamManager`

Used for team-scoped roster/invite/join-request management routes (`/team/:teamID/player/create`, `/team/:teamID/invite`, `/team/:teamID/joinRequests`, etc.), chained after `requireActive`. Reads `:teamID` from the route params (via `httprouter.ParamsFromContext` — params are visible to middleware earlier in the alice chain, not just the final handler, since the whole chain is registered as "the handler" with httprouter). 404s if `:teamID` doesn't parse to a positive integer. Otherwise allows the request through if `isAdmin` is true, **or** `isCaptain` is true **and** the request's own `teamID` context value matches the route's `:teamID` — i.e. an admin can manage any team, a captain can only manage their own. Everyone else is redirected to `/`. Sets `Cache-Control: no-store`.

## Static Files

Static files (`/static/*filepath`) bypass the dynamic chain entirely — no session or CSRF middleware.
