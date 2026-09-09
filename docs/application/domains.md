# Domain Summary

The application is organized into seven domains. Leagues, Teams, Seasons/Matches, and Notifications are closely related and share concepts (season/team-scoped authorization, RSVP/reminder plumbing), so cross-references between them are called out inline rather than duplicating the same explanation.

---

## Players

**Files:** `handlers_players.go`, `services/players.go`, `models/players.go`

A **player** is a person on a team roster: firstname, lastname, date of birth, address, email, phone number, plus phone-verification and SMS-opt-in state (see [Notifications](#notifications)).

**Key concepts:**
- Team membership is **not** a column on `players` — it's the `teamMembers` junction table, so a player can belong to more than one team at once (see [Teams](#teams)). A self-registered player with no invite starts on no team at all.
- Players can exist without a linked user account (an admin/captain can add a roster entry before the player signs up) and user accounts can exist without a player (an admin/coach account).
- A player's address is stored in a separate `address` table, linked by `addressID`.
- Deleting a player defensively clears them as captain first (`TeamModel.ClearCaptainByPlayer`) so the `fk_teams_captain` foreign key doesn't block the delete.
- **Privacy**: a player's name and stats are visible to any active user (team pages, standings, leader tables). The private bio fields (email, phone, date of birth, address) are only shown to an admin, the player themself, a manager of any team the player is on, or a plain teammate (`isTeammateOfPlayer`) — everyone else sees the profile with that card omitted entirely, not shown with placeholders.
- **Career stats**: a player's profile shows all-time totals (goals/assists/cards/MP) and a season-by-season breakdown (`buildPlayerCareerStats`), each season linking into that league's page with the season selected.

**Routes:**

| Method | Path | Access | Purpose |
|---|---|---|---|
| GET | `/player/view/:id` | Active | Player profile — always reachable, private fields conditionally shown |
| GET/POST | `/player/update/:id` | Active | Edit — own profile, admin, or a manager of the player's team(s) |
| GET/POST | `/team/:teamID/player/create` | Team manager | Add a player directly to that team's roster |
| DELETE | `/team/:teamID/player/:id/remove` | Team manager | Remove a player from that roster (lightweight — leaves the player record and their stats intact) |
| DELETE | `/player/delete/:id` | Admin | Full destructive delete: bio, address, every team membership, orphans their login |

The roster itself is shown inline on the team page (`GET /team/:teamID`), not a separate list route — see [Teams](#teams).

---

## Leagues

**Files:** `handlers_leagues.go`, `services/leagues.go`, `models/leagues.go`

A **league** is the top-level scope everything else (teams, seasons, matches) belongs to. The league page (`/league/:id`) is the hub for browsing a league: it merges what used to be two separate pages (a league overview and a per-season standings page) into one, with a season picker driving three tabs.

**Key concepts:**
- **League Admins** (`leagueAdmins` table): a many-to-many junction — a league can have more than one admin, and a player can administer more than one league. Assigning/revoking a league admin is deliberately system-admin-only (never delegable, including to other league admins), to avoid a privilege-escalation loop.
- A league admin can manage everything about their league (teams, seasons, matches) that a plain captain can't, but cannot edit the league record itself (name/motto/established date) or delete it — only a system admin can, and league deletion is blocked (`ErrHasDependents`) until its teams are removed.
- **The season picker** (`?season=<id>` on `/league/:id`): defaults to `SeasonModel.GetCurrentOrNext` (prefers an upcoming season over a recently-ended one, so a brand-new season shows immediately rather than the page lingering on the last one with results) when no season is specified or the one given doesn't belong to this league. Three tabs — Standings, Matches, Leaders — all key off whichever season is selected.
- **Standings** are computed fresh per request (`buildStandings`) by merging every team currently in the league with that season's match results, so a winless team still shows up at all zeros rather than being absent.
- **Leaders**: top-5 goal and assist scorers for the selected season, league-wide.

**Routes:**

| Method | Path | Access | Purpose |
|---|---|---|---|
| GET | `/league` | Active | Browse all leagues |
| GET | `/league/:id` | Active | League detail: season picker, Standings/Matches/Leaders tabs |
| GET/POST | `/admin/league/create` | Admin | Create a league |
| GET/POST | `/admin/league/update/:id` | Admin | Edit a league |
| DELETE | `/admin/league/delete/:id` | Admin | Delete (blocked with `ErrHasDependents` if it has teams) |
| POST | `/admin/league/admins/add` | Admin | Grant a player league-admin status |
| POST | `/admin/league/admins/remove` | Admin | Revoke league-admin status |

League/team CRUD lives under `/admin/...` rather than directly under `/league/...`/`/team/...` for a routing-conflict reason — see [Architecture Overview: Routing Constraints](./architecture-overview.md#routing-constraints).

---

## Teams

**Files:** `handlers_teams.go`, `services/teams.go` + `services/invites.go` + `services/joinRequests.go`, `models/teams.go` + `models/teamMembers.go` + `models/invites.go` + `models/joinRequests.go`

A **team** belongs to exactly one **league** and has zero or one **captain** (a player on its own roster, via `teams.captainPlayerID`, admin-assigned). The team page (`/team/:teamID`) is the roster hub: a season picker plus Roster/Matches/Leaders tabs, mirroring the league page's structure — "Matches" here is that team's own schedule for the selected season, and "Roster" is that team's current membership with per-season stat columns.

**Key concepts:**
- **Team membership** (`teamMembers` table): a player can be on more than one team, though the service layer enforces at most one team per league per player. Membership carries an `isLegend` flag (see below).
- **Captain**: one per team (`teams.captainPlayerID` is unique), gets the same roster CRUD an admin has, scoped to that team, plus inviting players and approving/rejecting join requests.
- **Scorekeepers** (`teamScorekeepers` table): a non-exclusive second tier of delegation below the captain — a scorekeeper can edit that team's match scores/goals/cards but not manage the roster, invites, or join requests.
- **Legend status** (`teamMembers.isLegend`): a roster-organizing status, not a functional restriction — a Legend still shows up, can RSVP is intentionally blocked (a Legend is assumed to be a former active player kept around for career-stats history, not someone actively scheduling to play), and their career stats still count. Roster pages split into Active/Legends sub-tabs.
- **Invites** (`invites` table): a captain/league-admin/admin enters one or a list of email addresses; each gets a single-use signup link. The invited team is granted regardless of what email address the person actually registers with — the token, not the email, carries the assignment. Only `usedAt` prevents reuse — no time-based expiration. An invite can optionally carry `asCaptain` (gated to admin/league-admin, excludes a plain captain — see `canInviteAsCaptain`), making the invited player that team's captain on signup.
- **Join requests** (`teamJoinRequests` table): an active player with no team in that league can browse to a team and request to join. Notifies the team's captain (email, or logged in dev); if the team has no captain, it's visible cross-team to any admin at `/admin/joinRequests`. Approving assigns the team and auto-rejects any other pending request by that player; rejecting leaves them free to resubmit immediately.
- **Roster export/import**: a PDF roster-registration form export (`teamRosterExport`, via `fpdf`) and a CSV roster import (`teamRosterImportForm`/`Submit`) for bulk-adding players.

**Routes (active users):**

| Method | Path | Purpose |
|---|---|---|
| GET | `/team/:teamID` | Team detail — roster, season picker, Request-to-Join form if eligible |
| POST | `/team/:teamID/joinRequest` | An eligible active player requests to join |

**Routes (team manager — admin, that team's captain, or its league's admin):**

| Method | Path | Purpose |
|---|---|---|
| GET/POST | `/team/:teamID/player/create` | Add a player to the roster |
| DELETE | `/team/:teamID/player/:id/remove` | Remove from the roster |
| GET/POST | `/team/:teamID/invite` | Send invite emails |
| POST | `/team/:teamID/invite/roster` | Invite existing roster members who have no account yet |
| DELETE | `/team/:teamID/invite/:inviteID/cancel` | Cancel a pending invite |
| GET | `/team/:teamID/joinRequests` | Pending requests for this team |
| POST | `/team/:teamID/joinRequests/:requestID/approve` \| `/reject` | Approve/reject (re-verifies the request belongs to `:teamID`) |
| GET | `/team/:teamID/rosterExport` | Download the PDF roster form |
| GET/POST | `/team/:teamID/rosterImport` | Bulk-add players via CSV |
| GET | `/admin/team/update/:teamID` | Edit the team's own info (name/motto/home field/reminder settings) |

**Routes (in-handler `canManageTeam` check — POST body carries the team, not the URL):**

| Method | Path | Purpose |
|---|---|---|
| POST | `/admin/team/update` | Save team edits |
| POST | `/admin/team/setCaptain` | Set or clear (`playerID=0`) the captain — target must already be on the roster |
| POST | `/admin/team/scorekeepers/add` \| `/remove` | Grant/revoke scorekeeper status |
| POST | `/admin/team/legends/add` \| `/remove` | Move a roster member to/from Legend status |

**Routes (league manager — admin or that team's league admin, excludes plain captain):**

| Method | Path | Purpose |
|---|---|---|
| DELETE | `/admin/team/delete/:teamID` | Delete (blocked with `ErrHasDependents` if it has players) |

**Routes (admin-only):**

| Method | Path | Purpose |
|---|---|---|
| GET/POST | `/admin/team/create` | Create a team (picks a league) |
| GET | `/admin/joinRequests` | Pending join requests across all teams |

---

## Seasons & Matches

**Files:** `handlers_seasons.go`, `handlers_matches.go`, `services/seasons.go` + `services/matches.go` + `services/scheduleImport.go`, `models/seasons.go` + `models/matches.go` + `models/playerMatchStats.go` + `models/rsvps.go` + `models/matchAttendance.go`

A **season** belongs to one league and holds a set of **matches**, each between two of that league's teams. This is the domain the league/team pages' Standings/Matches/Leaders tabs are built from.

**Key concepts:**
- **CSV schedule import** (`seasonScheduleImportForm`/`Submit`): bulk-creates a season's matches from a spreadsheet — team and location names are matched against existing records (case/punctuation-insensitive) rather than auto-creating duplicates; re-uploading is safe, since rows are matched by home team/away team/date and updated in place rather than duplicated.
- **Match detail page** (`/match/:id`) is the richest single page in the app: score/goals/cards (editable by a manager or scorekeeper), a per-team captain's note and Player-of-the-Match pick, per-player RSVPs (with a message), and — once the match date has passed — an attendance reconciliation (defaults to "yes" RSVPs, correctable by a manager/scorekeeper for actual no-shows or walk-ons).
- **RSVPs** (`rsvps` table): any active roster member (not a Legend — see `canRSVPAsTeam`) can respond yes/no with an optional message, changeable any time before the match.
- **Player stats** (`playerMatchStats` table): goals, assists, own goals, yellow/red cards, entered per match by a manager/scorekeeper — the source for career stats, standings, and leader tables.
- **Reminders**: `MatchReminderService` sends RSVP reminders (to anyone who hasn't responded) and captain's-message reminders, once a day, counting down from each team's own configured `ReminderDaysOut` to the match, gated on that team's `ReminderTime` having passed for the day and `RemindersEnabled` being on. Delivery channel (email/SMS/both) is per-player, per-category — see [Notifications](#notifications). A captain/admin can also send an ad-hoc test reminder (email or SMS) to specific teammates without affecting the real schedule or delivery-tracking tables (`matchRSVPReminders`, `matchCaptainMessageReminders`).

**Routes:**

| Method | Path | Access | Purpose |
|---|---|---|---|
| GET | `/match/:id` | Active | Match detail page |
| POST | `/match/:id/rsvp` | Active (own team) | Submit/update an RSVP |
| POST | `/match/:id/notes` | `canManageMatchSide` | Save Player-of-the-Match/captain's notes for one side |
| POST | `/match/:id/attendance` | `canManageAttendanceSide` | Save attendance corrections for one side |
| POST | `/match/:id/testReminder` \| `/testReminderSMS` | `canManageMatchSide` | Send an ad-hoc test reminder |
| GET | `/season/:id/scheduleImport` | Active + `canManageLeague` | CSV schedule import form |
| POST | `/season/:id/scheduleImport` | Active + `canManageLeague` | Submit the CSV |
| GET | `/scheduleImport/sample.csv`, `/rosterImport/sample.csv` | Active | Downloadable CSV templates |
| GET/POST | `/admin/season/create`, `/admin/season/update/:id` | `canManageLeague` (in-handler) | Season CRUD |
| DELETE | `/admin/season/delete/:id` | `canManageLeague` (in-handler) | Delete a season (bulk-deletes its matches too) |
| GET/POST | `/admin/match/create`, `/admin/match/update/:id` | `canManageLeague` / `canManageMatch` | Match CRUD |
| DELETE | `/admin/match/delete/:id` | `canDeleteMatch` | Delete a match |

---

## Notifications

**Files:** `handlers_playerNotifications.go`, `handlers_calendar.go`, `services/notificationPreferences.go`, `models/notificationPreferences.go`, `internal/services/sms.go`, `internal/services/calendar.go`

A player's own Notification Preferences page (`/player/notifications/:id`, strictly self-service — see `requireOwnPlayer`) covers phone verification, SMS program consent, per-notification-category delivery choice, and a personal calendar feed. See [Integrations: Twilio](../integrations/integrations.md#twilio-sms) for the full SMS consent model and compliance rationale.

**Key concepts:**
- **Phone verification** (`Player.PhoneVerifiedAt`/`PhoneVerificationCode`/`PhoneVerificationExpiresAt`): a 6-digit code, valid 10 minutes, with a 60-second resend cooldown. Changing the phone number clears verification, requiring the new number to be re-verified.
- **SMS program opt-in** (`Player.SMSOptInAt`): a separate, explicit, persisted consent event — distinct from verification — checked alongside it wherever a text is actually sent. Revocable independently via a self-service "Opt Out of SMS Program" action; "Remove Phone Number" is a separate, independent action that clears the number/verification without touching this consent.
- **Per-category delivery preference** (`playerNotificationPreferences` table): `rsvp_reminder` and `captain_message_reminder` categories, each an independent Email/Text checkbox pair mapping onto a four-value channel (`email`/`sms`/`both`/`off`, defaulting to `email`). Choosing Text for either requires both phone verification and SMS opt-in — enforced server-side in `NotificationPreferenceService.SetPreference` regardless of what the form renders.
- **Calendar feed** (`Player.CalendarToken`, `CalendarService`): a per-player, token-authenticated RFC 5545 (`.ics`) feed of every upcoming match across every active-roster team, served at `GET /calendar/:token/schedule.ics` — deliberately outside the session/CSRF chain, since a phone's calendar app fetches it with no cookies at all; the secret token is the sole access control. Regenerating the token immediately invalidates the old URL (the leak-revocation path).

**Routes:**

| Method | Path | Purpose |
|---|---|---|
| GET | `/player/notifications/:id` | The preferences page |
| POST | `/player/notifications/:id/phone` | Request a verification code (requires the SMS opt-in checkbox) |
| POST | `/player/notifications/:id/phone/confirm` | Confirm the code |
| POST | `/player/notifications/:id/phone/remove` | Clear the phone number and verification state |
| POST | `/player/notifications/:id/sms/optOut` | Revoke SMS program consent |
| POST | `/player/notifications/:id/preferences` | Save per-category Email/Text choices |
| POST | `/player/notifications/:id/calendar/regenerate` | Reissue the calendar token |
| GET | `/calendar/:token/schedule.ics` | The feed itself (token-authenticated, no session) |

---

## Users (`handlers_users.go`, `services/users.go`, `models/users.go`)

A **user** is an account with login credentials. Users are separate from players but linked by `users.playerID` (a nullable foreign key, not the reference project's join-by-email).

**Key concepts:**
- Users have an `active` flag. An inactive user can log in but cannot access protected routes.
- Only one role exists: `ADMIN`, stored in a `userRole` junction table (kept as real tables so adding e.g. `COACH` later is a data change, not a schema change).
- The full registration flow is: signup → email verification → account activation, optionally carrying an invite token. See [Authentication Flow](../security/authentication.md).
- When a user is activated, they're linked to an existing unlinked player row with a matching email if one exists (an admin/captain pre-added roster entry), otherwise a placeholder player record is created — joined to the invite's team if the signup carried one.
- When an admin activates a user (via toggle), the same link-or-create happens if the user has no linked player.
- **Remember me**: an opt-in login checkbox that extends the session from the ordinary 12 hours out to 30 days — see [Security Model: Sessions](../security/security.md#sessions).
- **View as player**: an admin can toggle into seeing the site exactly as their own linked player would, without logging out — see [Middleware Chain](./middleware.md#authenticate).

**Public routes:**
| Method | Path | Purpose |
|---|---|---|
| GET/POST | `/user/signup` | Register a new account (reads/threads `?invite=<token>`) |
| GET/POST | `/user/login` | Log in (with Remember Me) |
| GET | `/user/activate` | Activate account from email link |
| GET/POST | `/user/forgotPassword` | Request a password reset |
| GET/POST | `/user/resetPassword` | Complete a password reset |

**Protected (authenticated):**
| Method | Path | Purpose |
|---|---|---|
| POST | `/user/logout` | Log out |

**Active:**
| Method | Path | Purpose |
|---|---|---|
| POST | `/user/toggleViewAsPlayer` | Toggle "view as player" |

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
| `/` | Active/Public | Home dashboard (public shell for a logged-out visitor; upcoming matches, My Leagues/Teams for an active user) |
| `/privacy`, `/terms`, `/contact` | Public | Legal/business pages |
| `/captains` | Public | Captain onboarding guide |
| POST `/captains/dismiss` | Active | Permanently dismiss the home page's captain-guide banner |
| `/sms-optin` | Public (unlinked) | Screenshot walkthrough of the real SMS consent flow, for carrier/Twilio review — not linked from site nav |

**Also global/outside the dynamic chain:**

| Route | Purpose |
|---|---|
| `GET /static/*filepath` | Embedded static assets |
| `GET /calendar/:token/schedule.ics` | Per-player calendar feed — see [Notifications](#notifications) |

---

## Locations (`handlers_locations.go`, `models/locations.go` + `models/address.go`)

A **location** is a home field a team can be assigned to (`teams.locationID`) and matches can be played at (`matches.locationID`) — name plus a linked `address` row, used to build a Google Maps link on match/team pages. Admin-only CRUD, the same tier as leagues; any team manager can still pick an existing location for their own team via the team edit form.

| Method | Path | Purpose |
|---|---|---|
| GET | `/location` | Browse all locations (any active user) |
| GET/POST | `/admin/location/create`, `/admin/location/update/:id` | Admin CRUD |
| DELETE | `/admin/location/delete/:id` | Delete (blocked if a team still uses it as its home field) |
