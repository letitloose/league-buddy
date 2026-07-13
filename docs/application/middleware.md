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
| `userName` | string | Always set (player name or email) |

This middleware never redirects. It only populates context.

## Route-Specific Middleware

### `requireAuthentication`

Used for `/user/logout`. Redirects to `/user/login` if `isAuthenticated` is false.

### `requireActive`

Used for player-facing routes. Redirects to `/` if `isActive` is false. Sets `Cache-Control: no-store`.

### `requireAdmin`

Used for administrative routes. Redirects to `/` if `isAdmin` is false. Sets `Cache-Control: no-store`.

## Static Files

Static files (`/static/*filepath`) bypass the dynamic chain entirely — no session or CSRF middleware.
