# Security Model

Ported near-verbatim from `toller-club-docker` — none of this is domain-specific.

## CSRF Protection

All state-changing requests (POST, DELETE) are protected against Cross-Site Request Forgery by [nosurf](https://github.com/justinas/nosurf).

**How it works:**

1. On first page load, nosurf sets a `Secure`, `HttpOnly` session cookie containing a random base64 token.
2. The `authenticate` middleware (via `nosurf.Token(r)`) makes the current request's masked token available as `{{.CSRFToken}}` in every template's `templateData`.
3. Every HTML form includes a hidden field: `<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">`.
4. On each POST, nosurf validates that the submitted token matches the cookie.
5. For DELETE requests (which don't carry a form body), the token is sent as an `X-CSRF-Token` request header — see `ui/static/js/main.js`, which reads a `<meta name="csrf-token">` tag in `base.html`'s `<head>`.

**Token encoding gotcha:** Go's `html/template` HTML-escapes attribute values, turning the base64 `+` character into `&#43;`. When extracting the CSRF token from rendered HTML in tests, `html.UnescapeString()` must be called before the token is usable. This is handled in `testutils_test.go`.

**The CSRF cookie is `Secure`-only**, which means production requires TLS (enforced by nginx) and tests use `httptest.NewTLSServer` rather than plain HTTP.

**Version difference from the reference project:** `go get` pulled `nosurf` v1.2.0 (the reference project pins v1.1.1). v1.2.0 added a same-origin check — via `Sec-Fetch-Site`, falling back to `Origin`, falling back to `Referer` — that runs *before* the token check on every non-safe-method request. Real browsers send one of these automatically on a same-page form submit, so this is invisible in normal use and is a strict security improvement (an extra layer against CSRF beyond the token alone). It does mean any HTTP client that doesn't set `Origin`/`Referer` — including Go's own `http.Client.PostForm` — gets a 400 before ever reaching the token check. `testutils_test.go`'s `postForm`/`delete` helpers set a `Referer` header matching the test server's own URL to satisfy this.

## Sessions

Sessions are managed by [SCS v2](https://github.com/alexedwards/scs) with a **MySQL-backed store** (`scs/mysqlstore`).

- Session tokens are stored in a `sessions` table in MariaDB.
- Sessions expire after **12 hours** (set in `main.go`).
- The session stores only the authenticated user ID: `authenticatedUserID = int`.
- `sessionManager.RenewToken()` is called on both login (before writing the user ID) and logout (before clearing it), preventing session fixation attacks.

## Password Hashing

Passwords are hashed with **bcrypt at cost 12** using `golang.org/x/crypto/bcrypt`.

- The `verification_hash` (used for email activation and password reset links) is also bcrypt-hashed — derived from `email + password`.
- Raw passwords are never logged or stored.
- Bcrypt is deliberately slow at cost 12; the test suite pre-computes hashes at `bcrypt.MinCost` for test users to keep the suite fast.

## Secure HTTP Headers

Every response carries the following headers (set by the `secureHeaders` middleware):

| Header | Value |
|---|---|
| `Content-Security-Policy` | `default-src 'self'` plus inline styles — no third-party script/style origins in v1 |
| `Referrer-Policy` | `origin-when-cross-origin` |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `deny` |
| `X-XSS-Protection` | `0` — disabled; CSP is the correct modern protection |

## Authorization Model

Authorization is role-based and enforced at the middleware level, not in handlers.

| Tier | Middleware | Condition |
|---|---|---|
| Authenticated | `requireAuthentication` | Session contains `authenticatedUserID` |
| Active | `requireActive` | User record has `active = true` |
| Admin | `requireAdmin` | User has the `ADMIN` role in `userRole` |

`GetAuthContext` consolidates the active flag, admin flag, and linked player into **one SQL query per request**.

One additional check exists outside the middleware layer: `playerUpdate`/`playerUpdatePost` compare the logged-in user's `playerID` against the record being edited, so a non-admin can only edit their own profile. This is handler-level, not middleware, because it depends on the specific `:id` in the URL — the Dog CRUD pattern this was modeled on has no equivalent, since a dog has no concept of "the logged-in user's own record."

## TLS

TLS termination happens at nginx, not in the Go application. The Go server listens on HTTP on `127.0.0.1:8081` (host) and is not reachable from outside the Docker network.

## Audit Logging

Security-sensitive actions are written to the `auditLog` table: account created, account activated, password reset requested/completed, user activated/deactivated by admin, admin role toggled, user deleted, player record created/updated/deleted. Each entry records the actor's email, a timestamp, and a description.

## Input Validation

All user input is validated in the **service layer** (not handlers) using the `internal/validator` package before being passed to models. Validation errors are returned as `models.ErrBadData`, causing the handler to re-render the form with error messages. Database-level uniqueness constraints (`UNIQUE` on `users.email`, `players.email`) provide a second line of defense, surfaced as `models.ErrDuplicateEmail`.

## SQL Injection

All SQL uses parameterized queries (`?` placeholders via `database/sql`). The one exception is `playerSearch`/`userSearch`, where the sort column and direction are constructed from user-supplied query parameters — these are validated against an allowlist map (`allowedSorts`) in `buildPlayerSearchStatement`/`buildUserSearchStatement`, not interpolated directly, so this should be reviewed if new sort options are ever added directly into the SQL string rather than through that allowlist.
