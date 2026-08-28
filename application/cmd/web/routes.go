package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/justinas/alice"
	"github.com/letitloose/league-buddy/ui"
)

func (app *application) routes() http.Handler {

	router := httprouter.New()

	router.NotFound = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.notFound(w)
	})

	fileServer := http.FileServer(http.FS(ui.Files))
	router.Handler(http.MethodGet, "/static/*filepath", fileServer)

	dynamic := alice.New(app.sessionManager.LoadAndSave, noSurf, app.authenticate)

	// public routes
	router.Handler(http.MethodGet, "/", dynamic.ThenFunc(app.home))
	router.Handler(http.MethodGet, "/user/signup", dynamic.ThenFunc(app.userSignup))
	router.Handler(http.MethodPost, "/user/signup", dynamic.ThenFunc(app.userSignupPost))
	router.Handler(http.MethodGet, "/user/login", dynamic.ThenFunc(app.userLogin))
	router.Handler(http.MethodPost, "/user/login", dynamic.ThenFunc(app.userLoginPost))
	router.Handler(http.MethodGet, "/user/activate", dynamic.ThenFunc(app.activateUser))
	router.Handler(http.MethodGet, "/user/forgotPassword", dynamic.ThenFunc(app.forgotPassword))
	router.Handler(http.MethodPost, "/user/forgotPassword", dynamic.ThenFunc(app.forgotPasswordPost))
	router.Handler(http.MethodGet, "/user/resetPassword", dynamic.ThenFunc(app.resetPassword))
	router.Handler(http.MethodPost, "/user/resetPassword", dynamic.ThenFunc(app.resetPasswordPost))

	// authenticated routes (logged in; need not be active)
	authenticated := dynamic.Append(app.requireAuthentication)
	router.Handler(http.MethodPost, "/user/logout", authenticated.ThenFunc(app.userLogoutPost))

	// active routes (logged in + active)
	active := dynamic.Append(app.requireActive)
	router.Handler(http.MethodGet, "/player/view/:id", active.ThenFunc(app.playerView))
	router.Handler(http.MethodGet, "/player/update/:id", active.ThenFunc(app.playerUpdate))
	router.Handler(http.MethodPost, "/player/update", active.ThenFunc(app.playerUpdatePost))
	router.Handler(http.MethodGet, "/league", active.ThenFunc(app.leagueList))
	router.Handler(http.MethodGet, "/league/:id", active.ThenFunc(app.leagueView))
	router.Handler(http.MethodGet, "/team/:teamID", active.ThenFunc(app.teamView))
	router.Handler(http.MethodPost, "/team/:teamID/joinRequest", active.ThenFunc(app.joinRequestSubmit))
	router.Handler(http.MethodGet, "/location", active.ThenFunc(app.locationList))
	router.Handler(http.MethodGet, "/season/:id", active.ThenFunc(app.seasonView))
	router.Handler(http.MethodGet, "/team/:teamID/season/:seasonID", active.ThenFunc(app.teamSeasonView))
	router.Handler(http.MethodGet, "/match/:id", active.ThenFunc(app.matchView))
	router.Handler(http.MethodPost, "/match/:id/rsvp", active.ThenFunc(app.matchRSVPSubmit))
	router.Handler(http.MethodPost, "/match/:id/notes", active.ThenFunc(app.matchTeamNoteSubmit))
	router.Handler(http.MethodPost, "/match/:id/testReminder", active.ThenFunc(app.matchTestReminderSubmit))

	// team-manager routes (logged in + active + (admin OR captain OR league
	// admin of :teamID))
	teamManager := dynamic.Append(app.requireActive, app.requireTeamManager)
	router.Handler(http.MethodGet, "/team/:teamID/player/create", teamManager.ThenFunc(app.playerForm))
	router.Handler(http.MethodPost, "/team/:teamID/player/create", teamManager.ThenFunc(app.playerCreate))
	router.Handler(http.MethodDelete, "/team/:teamID/player/:id/remove", teamManager.ThenFunc(app.playerRemoveFromTeam))
	router.Handler(http.MethodGet, "/team/:teamID/invite", teamManager.ThenFunc(app.teamInviteForm))
	router.Handler(http.MethodPost, "/team/:teamID/invite", teamManager.ThenFunc(app.teamInviteSend))
	router.Handler(http.MethodPost, "/team/:teamID/invite/roster", teamManager.ThenFunc(app.teamInviteFromRoster))
	router.Handler(http.MethodDelete, "/team/:teamID/invite/:inviteID/cancel", teamManager.ThenFunc(app.inviteCancel))
	router.Handler(http.MethodGet, "/team/:teamID/joinRequests", teamManager.ThenFunc(app.joinRequestList))
	router.Handler(http.MethodPost, "/team/:teamID/joinRequests/:requestID/approve", teamManager.ThenFunc(app.joinRequestApprove))
	router.Handler(http.MethodPost, "/team/:teamID/joinRequests/:requestID/reject", teamManager.ThenFunc(app.joinRequestReject))
	// GET /admin/team/update/:teamID edits a team's own info (name/motto/
	// established date) — allowed to the same tier as roster management
	// (admin, captain, or league admin), unlike team creation/deletion below.
	router.Handler(http.MethodGet, "/admin/team/update/:teamID", teamManager.ThenFunc(app.teamUpdate))

	// league-manager routes (logged in + active + (admin OR league admin of
	// the team's league) — deliberately excludes plain captains, since
	// deleting a team is more destructive than editing or managing its
	// roster.
	leagueManager := dynamic.Append(app.requireActive, app.requireLeagueManager)
	router.Handler(http.MethodDelete, "/admin/team/delete/:teamID", leagueManager.ThenFunc(app.teamDelete))

	// active routes with an in-handler authorization check, used where the
	// scoping ID (leagueID/teamID) arrives in the POST body rather than the
	// URL, so no httprouter wildcard param is available for the middleware
	// tiers above to key off of.
	router.Handler(http.MethodGet, "/admin/team/create", active.ThenFunc(app.teamForm))
	router.Handler(http.MethodPost, "/admin/team/create", active.ThenFunc(app.teamCreate))
	router.Handler(http.MethodPost, "/admin/team/update", active.ThenFunc(app.teamUpdatePost))
	router.Handler(http.MethodPost, "/admin/team/setCaptain", active.ThenFunc(app.teamSetCaptain))
	router.Handler(http.MethodPost, "/admin/team/scorekeepers/add", active.ThenFunc(app.teamAddScorekeeper))
	router.Handler(http.MethodPost, "/admin/team/scorekeepers/remove", active.ThenFunc(app.teamRemoveScorekeeper))
	// Season/match administration is scoped to "anyone who can administer the
	// league" (canManageLeague — admin or league admin, captains excluded).
	// All on the active tier with an in-handler check, same rationale as the
	// team routes just above: create has no URL param to key middleware off
	// of, and match-scoped routes would need a two-hop match->season->league
	// lookup anyway, so one consistent pattern beats two.
	router.Handler(http.MethodGet, "/admin/season/create", active.ThenFunc(app.seasonForm))
	router.Handler(http.MethodPost, "/admin/season/create", active.ThenFunc(app.seasonCreate))
	router.Handler(http.MethodGet, "/admin/season/update/:id", active.ThenFunc(app.seasonUpdate))
	router.Handler(http.MethodPost, "/admin/season/update", active.ThenFunc(app.seasonUpdatePost))
	router.Handler(http.MethodDelete, "/admin/season/delete/:id", active.ThenFunc(app.seasonDelete))
	router.Handler(http.MethodGet, "/admin/match/create", active.ThenFunc(app.matchForm))
	router.Handler(http.MethodPost, "/admin/match/create", active.ThenFunc(app.matchCreate))
	router.Handler(http.MethodGet, "/admin/match/update/:id", active.ThenFunc(app.matchUpdate))
	router.Handler(http.MethodPost, "/admin/match/update", active.ThenFunc(app.matchUpdatePost))
	router.Handler(http.MethodDelete, "/admin/match/delete/:id", active.ThenFunc(app.matchDelete))

	// admin routes (logged in + active + ADMIN role)
	admin := dynamic.Append(app.requireAdmin)
	router.Handler(http.MethodGet, "/user/search", admin.ThenFunc(app.userSearch))
	router.Handler(http.MethodGet, "/user/view/:id", admin.ThenFunc(app.userView))
	router.Handler(http.MethodPost, "/user/toggleActive", admin.ThenFunc(app.toggleActive))
	router.Handler(http.MethodPost, "/user/toggleAdmin", admin.ThenFunc(app.toggleAdmin))
	router.Handler(http.MethodDelete, "/user/delete/:id", admin.ThenFunc(app.deleteUser))
	// Full destructive player delete (bio, address, every team membership,
	// orphans their login) is admin-only and flat, not per-team, since a
	// player can now belong to several teams — see playerRemoveFromTeam
	// above for the lightweight per-roster action captains/league admins use
	// instead.
	router.Handler(http.MethodDelete, "/player/delete/:id", admin.ThenFunc(app.playerDelete))
	// Admin-only league create+update+delete routes live under /admin/...
	// rather than /league/..., because httprouter v1.3.0 refuses to register
	// a static route (e.g. "/league/create", or "/league/delete/:id"
	// alongside the DELETE method's own "/league/:id"-shaped wildcard tree)
	// alongside a wildcard sibling at the same path depth — it panics at
	// startup with a wildcard conflict. The view routes (/league/:id,
	// /team/:teamID and everything nested under it) keep their
	// plan-specified paths; only these admin CRUD entry points had to move.
	// League admins do not manage the league record itself (name/motto/
	// established date) or delete it — only system admins do, so these stay
	// admin-only.
	router.Handler(http.MethodGet, "/admin/league/create", admin.ThenFunc(app.leagueForm))
	router.Handler(http.MethodPost, "/admin/league/create", admin.ThenFunc(app.leagueCreate))
	router.Handler(http.MethodGet, "/admin/league/update/:id", admin.ThenFunc(app.leagueUpdate))
	router.Handler(http.MethodPost, "/admin/league/update", admin.ThenFunc(app.leagueUpdatePost))
	router.Handler(http.MethodDelete, "/admin/league/delete/:id", admin.ThenFunc(app.leagueDelete))
	// Who administers a league is itself admin-only to assign/revoke, to
	// avoid a privilege-escalation loop — plain POST with body params (not
	// DELETE+wildcard), mirroring teamSetCaptain's "plain form POST for a
	// one-off admin action" pattern and avoiding a third static-vs-wildcard
	// route at this same path depth.
	router.Handler(http.MethodPost, "/admin/league/admins/add", admin.ThenFunc(app.leagueAddAdmin))
	router.Handler(http.MethodPost, "/admin/league/admins/remove", admin.ThenFunc(app.leagueRemoveAdmin))
	router.Handler(http.MethodGet, "/admin/joinRequests", admin.ThenFunc(app.adminJoinRequestList))
	// Locations (home fields) are admin-only to create/edit/delete, same
	// tier as leagues — any team manager can still pick an existing one for
	// their team's home field via the team edit form.
	router.Handler(http.MethodGet, "/admin/location/create", admin.ThenFunc(app.locationForm))
	router.Handler(http.MethodPost, "/admin/location/create", admin.ThenFunc(app.locationCreate))
	router.Handler(http.MethodGet, "/admin/location/update/:id", admin.ThenFunc(app.locationUpdate))
	router.Handler(http.MethodPost, "/admin/location/update", admin.ThenFunc(app.locationUpdatePost))
	router.Handler(http.MethodDelete, "/admin/location/delete/:id", admin.ThenFunc(app.locationDelete))

	standard := alice.New(app.recoverPanic, app.logRequest, secureHeaders)

	return standard.Then(router)
}
