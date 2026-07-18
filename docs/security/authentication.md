# Authentication Flow

This document describes every step of the authentication lifecycle — from initial signup through per-request identity resolution. Ported from `toller-club-docker`, rewritten around the `playerID` foreign key instead of the reference project's join-by-email.

## Overview

```
Signup → Email Verification → Account Activation → Login → Session → Per-Request Context
```

Users and players are separate records linked by `users.playerID` (nullable). A user account holds credentials; a player record holds roster information. An admin/coach account can exist with no linked player; a player can exist on the roster with no user account yet.

---

## Signup (`POST /user/signup`)

**Service:** `UserService.InsertUser`
**Model:** `UserModel.Insert`

1. The handler validates the form server-side (email format, non-blank password, 8+ chars, password confirmation match).
2. `bcrypt.GenerateFromPassword(password, 12)` produces `hashed_password`.
3. `bcrypt.GenerateFromPassword(email+password, 12)` produces `verification_hash` — used as a single-use token in the activation link and password reset flow.
4. Both hashes are inserted into `users` along with `active = false`.
5. **Invite token** (optional): if the signup form carried an `inviteToken` (threaded through from `?invite=<token>` on the signup link — see [Invites](../application/domains.md#teams)), `InviteModel.GetByToken` looks it up; if found and not already used, `UserModel.SetPendingInvite` stores the invite's ID on the new user row (`users.pendingInviteID`). A stale, unknown, or already-used token is silently treated as "no invite" — signup is never blocked over a bad URL param. This durable DB column, not session or URL state, is what lets the invite survive from this request through to the separate activation-link click below.
6. **Activation email** (if `EMAIL_USER` is configured): fetches the raw `verification_hash` and emails the user a link: `https://<VIRTUAL_HOST>/user/activate?hash=<verification_hash>`.
7. An audit log entry is written: `"user record created: <email>"`.

At this point the user exists but `active = false`, so `requireActive` blocks all protected routes.

---

## Email Verification / Account Activation (`GET /user/activate?hash=<hash>`)

**Service:** `UserService.ActivateUser`

1. `GetByVerificationHash(hash)` looks up the user by their stored `verification_hash`.
2. `Activate(userID)` sets `users.active = 1`.
3. Audit log: `"user record activated: <email>"`.
4. `UserService.linkOrCreatePlayer(userID, email)` runs unconditionally:
   - If the user has a `pendingInviteID` (set at signup, see above), the invite is loaded via `InviteModel.Get`.
   - If an unlinked player row already exists with this email (an admin pre-added them to the roster), the user is linked to it via `SetPlayerID`; if a pending invite was found and that player has no team yet, `PlayerModel.SetTeam` assigns the invite's team.
   - Otherwise, a placeholder player record is created with empty name fields — assigned directly to the invite's team if one was found, otherwise left unaffiliated (`teamID` null) until an invite or an approved join request assigns one — and the user is linked to it.
   - If a pending invite was consumed, `InviteModel.MarkUsed` and `UserModel.ClearPendingInvite` finalize it — safe to call more than once for the same user, since re-linking to the same player or re-consuming an already-used invite is a no-op.

Signing up via a captain's invite link therefore joins that team directly at activation time, with no separate approval step. A plain (non-invited) signup ends up unaffiliated and must browse to a team and submit a join request — see [Teams](../application/domains.md#teams).

The verification hash is **not rotated** after activation — reuse of the same link has no additional effect since `active` is already true.

---

## Password Reset

### Request (`POST /user/forgotPassword`)

**Service:** `UserService.ForgotPassword`

1. Validates that the email field is non-blank.
2. `GetVerificationHashByEmail(email)` fetches the existing hash. If no user exists with that email, the function silently returns (no enumeration leak).
3. Emails the user a link (if `EMAIL_USER` is configured): `https://<VIRTUAL_HOST>/user/resetPassword?hash=<verification_hash>`.
4. Audit log: `"password reset email sent: <email>"`.

### Complete (`POST /user/resetPassword`)

**Service:** `UserService.ResetPassword`
**Model:** `UserModel.ResetPassword`

1. The reset form arrives pre-populated with the user's email (fetched in the GET handler via the hash URL parameter).
2. Validates password (8+ chars, matches confirmation).
3. `bcrypt.GenerateFromPassword(newPassword, 12)` generates a new hash and updates `users.hashed_password`.
4. Audit log: `"user password reset: <email>"`.

Note: the `verification_hash` is **not updated** after a password reset (same known limitation as the reference project) — the old reset link remains valid until the password is changed again.

---

## Login (`POST /user/login`)

**Service:** `UserService.AuthenticateUser`
**Model:** `UserModel.Authenticate`

1. Validates that the email field is non-blank.
2. `SELECT id, hashed_password FROM users WHERE email = ?` — if no row, return `ErrInvalidCredentials`.
3. `bcrypt.CompareHashAndPassword(storedHash, submittedPassword)` — if mismatch, return `ErrInvalidCredentials`.
4. Updates `lastlogin` timestamp.
5. Back in the handler: `sessionManager.RenewToken(ctx)` — rotates the session token to prevent session fixation.
6. `sessionManager.Put(ctx, "authenticatedUserID", id)`.
7. Redirects to `/`.

Login sets **only the user ID** in the session. All role and player data is re-fetched on every request by the `authenticate` middleware.

---

## Logout (`POST /user/logout`)

1. `sessionManager.RenewToken(ctx)` — rotates the token.
2. `sessionManager.Remove(ctx, "authenticatedUserID")`.
3. Redirects to `/`.

---

## Per-Request Identity: The `authenticate` Middleware

On every non-static request, after the session is loaded:

1. `sessionManager.GetInt(ctx, "authenticatedUserID")` — if 0, skip and call next handler with no auth context set.
2. `userService.GetAuthContext(id)` — one SQL query returning `users.active`, an `EXISTS` subquery for the ADMIN role, and a `LEFT JOIN` to `players` on `playerID` for the linked player's ID and display name.
3. Each flag is written to the request context with a typed key (`isAuthenticated`, `isActive`, `isAdmin`, `playerID`, `userName`).

The context key type is `type contextKey string` (defined in `context.go`) — a distinct Go type that prevents collisions with context keys set by third-party middleware.

**This middleware never redirects.** It only populates context. Authorization decisions happen in the route-specific middleware (`requireAuthentication`, `requireActive`, `requireAdmin`).

---

## Admin: Activate / Deactivate User (`POST /user/toggleActive`)

**Service:** `UserService.ToggleActive`

1. Fetches the current `active` state of the target user.
2. Toggles `users.active`.
3. Audit log: `"<adminEmail> activated/deactivated user record: <targetEmail>"`.
4. **Side effect**: if the target user has no linked player (`PlayerID` is null), `linkOrCreatePlayer` runs — same as the email activation flow.

---

## Session Fixation Prevention

Session token is rotated at two points: login (before writing `authenticatedUserID`) and logout (before clearing it). A token observed before login cannot be used after login, and a token observed during a logged-in session cannot be reused after logout.
