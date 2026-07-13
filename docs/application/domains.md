# Domain Summary

The application is organized into two domains. Each has its own handler file, service, and model.

---

## Players (`handlers_players.go`, `services/players.go`, `models/players.go`)

The core domain. A **player** is a person on the team roster: firstname, lastname, date of birth, address, email, and phone number.

**Key concepts:**
- Every player belongs to a `team` via `teamID`. v1 has exactly one team (`TeamModel.GetDefault()`); the schema is ready for more without a migration.
- Players can exist without a linked user account (an admin can add a roster entry before the player signs up) and user accounts can exist without a player (an admin/coach account).
- A player's address is stored in a separate `address` table, linked by `addressID`.

**Routes (active users):**
| Method | Path | Purpose |
|---|---|---|
| GET | `/player` | List the roster |
| GET | `/player/search` | Filter by name/email |
| GET | `/player/view/:id` | Player profile |
| GET/POST | `/player/update/:id` | Edit — own profile, or any player if admin |

**Admin-only:**
| Method | Path | Purpose |
|---|---|---|
| GET/POST | `/player/create` | Add a player directly to the roster |
| DELETE | `/player/delete/:id` | Remove a player |

---

## Users (`handlers_users.go`, `services/users.go`, `models/users.go`)

A **user** is an account with login credentials. Users are separate from players but linked by `users.playerID` (a nullable foreign key, not the reference project's join-by-email).

**Key concepts:**
- Users have an `active` flag. An inactive user can log in but cannot access protected routes.
- Only one role exists in v1: `ADMIN`, stored in a `userRole` junction table (kept as real tables so adding e.g. `COACH` later is a data change, not a schema change).
- The full registration flow is: signup → email verification → account activation. See [Authentication Flow](../security/authentication.md).
- When a user is activated, they're linked to an existing unlinked player row with a matching email if one exists (an admin-pre-added roster entry), otherwise a placeholder player record is created.
- When an admin activates a user (via toggle), the same link-or-create happens if the user has no linked player.

**Public routes:**
| Method | Path | Purpose |
|---|---|---|
| GET/POST | `/user/signup` | Register a new account |
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
