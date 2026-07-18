# Domain Summary

The application is organized into four domains. Each has its own handler file, service, and model (Leagues and Teams share the roster/invite/join-request concerns closely, so they're documented together below).

---

## Players

**Files:** `handlers_players.go`, `services/players.go`, `models/players.go`

A **player** is a person on a team roster: firstname, lastname, date of birth, address, email, and phone number.

**Key concepts:**
- `players.teamID` is nullable. A player is assigned to a team by an admin/captain adding them directly to a roster, by signing up via a captain's invite link, or by an admin/captain approving a join request. A self-registered player with no invite starts **unaffiliated** (`teamID` null).
- Players can exist without a linked user account (an admin/captain can add a roster entry before the player signs up) and user accounts can exist without a player (an admin/coach account).
- A player's address is stored in a separate `address` table, linked by `addressID`.
- Deleting a player defensively clears them as captain first (`TeamModel.ClearCaptainByPlayer`) so the `fk_teams_captain` foreign key doesn't block the delete.

**Routes are team-scoped** — roster management hangs off the owning team, not a flat `/player` tree:

| Method | Path | Access | Purpose |
|---|---|---|---|
| GET | `/team/:teamID/player` | Active | List that team's roster |
| GET | `/team/:teamID/player/search` | Active | Filter by name/email |
| GET | `/player/view/:id` | Active | Player profile (flat — addressed by player id alone) |
| GET/POST | `/player/update/:id` | Active | Edit — own profile, admin, or the captain of the player's own team |
| GET/POST | `/team/:teamID/player/create` | Team manager | Add a player directly to that team's roster |
| DELETE | `/team/:teamID/player/delete/:id` | Team manager | Remove a player from the roster |

"Team manager" = admin, or the captain of `:teamID` — see [`requireTeamManager`](./middleware.md#requireteammanager).

---

## Teams (`handlers_teams.go`, `services/teams.go` + `services/invites.go` + `services/joinRequests.go`, `models/teams.go` + `models/invites.go` + `models/joinRequests.go`)

A **team** belongs to exactly one **league** and has zero or one **captain** (a player on its own roster, via `teams.captainPlayerID`, admin-assigned). A team's captain gets the same roster CRUD an admin has, scoped to just that team, plus the ability to invite players and approve/reject join requests.

**Key concepts:**
- **Invites** (`invites` table): a captain or admin enters one or a list of email addresses; each gets a single-use signup link (`/user/signup?invite=<token>`). The invited team is granted regardless of what email address the person actually registers with — the token, not the email, carries the team assignment. See [Authentication Flow](../security/authentication.md) for how the token survives from signup to the separately-clicked activation link. Only `usedAt` prevents reuse — there's no time-based expiration.
- **Join requests** (`teamJoinRequests` table): an unaffiliated active player can browse to a team and request to join. This notifies the team's captain (by email, or logs the link in dev if no email is configured); if the team has no captain assigned, the request is still visible cross-team to any admin at `/admin/joinRequests`. Approving a request assigns the team (`PlayerModel.SetTeam`) and auto-rejects any other pending request by that player; rejecting leaves the player free to submit again immediately (no cooldown).
- A team can have at most one captain, and a player can't captain two teams at once (`teams.captainPlayerID` is unique).

**Routes (active users — team view, roster browsing, requesting to join):**

| Method | Path | Purpose |
|---|---|---|
| GET | `/league` | Browse all leagues |
| GET | `/league/:id` | One league's teams |
| GET | `/team/:teamID` | Team detail — captain, roster link, Request-to-Join form if eligible |
| POST | `/team/:teamID/joinRequest` | Unaffiliated active player requests to join |

**Routes (team manager — admin, or the captain of `:teamID`):**

| Method | Path | Purpose |
|---|---|---|
| GET/POST | `/team/:teamID/invite` | Send invite emails |
| GET | `/team/:teamID/joinRequests` | Pending requests for this team |
| POST | `/team/:teamID/joinRequests/:requestID/approve` | Approve (re-verifies the request belongs to `:teamID`) |
| POST | `/team/:teamID/joinRequests/:requestID/reject` | Reject (same re-verification) |

**Routes (admin-only):** league and team CRUD live under `/admin/...` rather than `/league/...`/`/team/...`, to avoid a routing conflict — see [Architecture Overview: Routing Constraints](./architecture-overview.md#routing-constraints).

| Method | Path | Purpose |
|---|---|---|
| GET/POST | `/admin/league/create` | Create a league |
| GET/POST | `/admin/league/update/:id` | Edit a league |
| DELETE | `/admin/league/delete/:id` | Delete (blocked with `ErrHasDependents` if it has teams) |
| GET/POST | `/admin/team/create` | Create a team (picks a league) |
| GET/POST | `/admin/team/update/:id` | Edit a team |
| DELETE | `/admin/team/delete/:id` | Delete (blocked with `ErrHasDependents` if it has players) |
| POST | `/admin/team/setCaptain` | Set or clear (`playerID=0`) a team's captain — target must already be on that team's roster |
| GET | `/admin/joinRequests` | Pending join requests across all teams |

---

## Users (`handlers_users.go`, `services/users.go`, `models/users.go`)

A **user** is an account with login credentials. Users are separate from players but linked by `users.playerID` (a nullable foreign key, not the reference project's join-by-email).

**Key concepts:**
- Users have an `active` flag. An inactive user can log in but cannot access protected routes.
- Only one role exists: `ADMIN`, stored in a `userRole` junction table (kept as real tables so adding e.g. `COACH` later is a data change, not a schema change).
- The full registration flow is: signup → email verification → account activation, optionally carrying an invite token. See [Authentication Flow](../security/authentication.md).
- When a user is activated, they're linked to an existing unlinked player row with a matching email if one exists (an admin/captain pre-added roster entry), otherwise a placeholder player record is created — joined to the invite's team if the signup carried one.
- When an admin activates a user (via toggle), the same link-or-create happens if the user has no linked player.

**Public routes:**
| Method | Path | Purpose |
|---|---|---|
| GET/POST | `/user/signup` | Register a new account (reads/threads `?invite=<token>`) |
| GET/POST | `/user/login` | Log in |
| GET | `/user/activate` | Activate account from email link |
| GET/POST | `/user/forgotPassword` | Request a password reset |
| GET/POST | `/user/resetPassword` | Complete a password reset |

**Protected (authenticated):**
| Method | Path | Purpose |
|---|---|---|
| POST | `/user/logout` | Log out |

**Admin-only:**
| Method | Path | Purpose |
|---|---|---|
| GET | `/user/search` | List/search user accounts |
| GET | `/user/view/:id` | View user details |
| POST | `/user/toggleActive` | Activate/deactivate a user |
| POST | `/user/toggleAdmin` | Grant/revoke admin role |
| DELETE | `/user/delete/:id` | Delete a user |

---

## Site (`handlers_site.go`)

| Route | Access | Purpose |
|---|---|---|
| `/` | Public | Home page |
