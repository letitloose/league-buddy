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
	router.Handler(http.MethodGet, "/team/:teamID/player", active.ThenFunc(app.playerList))
	router.Handler(http.MethodGet, "/team/:teamID/player/search", active.ThenFunc(app.playerSearch))
	router.Handler(http.MethodPost, "/team/:teamID/joinRequest", active.ThenFunc(app.joinRequestSubmit))

	// team-manager routes (logged in + active + (admin OR captain of :teamID))
	teamManager := dynamic.Append(app.requireActive, app.requireTeamManager)
	router.Handler(http.MethodGet, "/team/:teamID/player/create", teamManager.ThenFunc(app.playerForm))
	router.Handler(http.MethodPost, "/team/:teamID/player/create", teamManager.ThenFunc(app.playerCreate))
	router.Handler(http.MethodDelete, "/team/:teamID/player/delete/:id", teamManager.ThenFunc(app.playerDelete))
	router.Handler(http.MethodGet, "/team/:teamID/invite", teamManager.ThenFunc(app.teamInviteForm))
	router.Handler(http.MethodPost, "/team/:teamID/invite", teamManager.ThenFunc(app.teamInviteSend))
	router.Handler(http.MethodGet, "/team/:teamID/joinRequests", teamManager.ThenFunc(app.joinRequestList))
	router.Handler(http.MethodPost, "/team/:teamID/joinRequests/:requestID/approve", teamManager.ThenFunc(app.joinRequestApprove))
	router.Handler(http.MethodPost, "/team/:teamID/joinRequests/:requestID/reject", teamManager.ThenFunc(app.joinRequestReject))

	// admin routes (logged in + active + ADMIN role)
	admin := dynamic.Append(app.requireAdmin)
	router.Handler(http.MethodGet, "/user/search", admin.ThenFunc(app.userSearch))
	router.Handler(http.MethodGet, "/user/view/:id", admin.ThenFunc(app.userView))
	router.Handler(http.MethodPost, "/user/toggleActive", admin.ThenFunc(app.toggleActive))
	router.Handler(http.MethodPost, "/user/toggleAdmin", admin.ThenFunc(app.toggleAdmin))
	router.Handler(http.MethodDelete, "/user/delete/:id", admin.ThenFunc(app.deleteUser))
	// Admin-only league/team create+update+delete+setCaptain routes live
	// under /admin/... rather than /league/... or /team/..., because
	// httprouter v1.3.0 refuses to register a static route (e.g.
	// "/league/create", or "/league/delete/:id" alongside the DELETE
	// method's own "/league/:id"-shaped wildcard tree) alongside a wildcard
	// sibling at the same path depth — it panics at startup with a wildcard
	// conflict. The view routes (/league/:id, /team/:teamID and everything
	// nested under it) keep their plan-specified paths; only these admin
	// CRUD entry points had to move.
	router.Handler(http.MethodGet, "/admin/league/create", admin.ThenFunc(app.leagueForm))
	router.Handler(http.MethodPost, "/admin/league/create", admin.ThenFunc(app.leagueCreate))
	router.Handler(http.MethodGet, "/admin/league/update/:id", admin.ThenFunc(app.leagueUpdate))
	router.Handler(http.MethodPost, "/admin/league/update", admin.ThenFunc(app.leagueUpdatePost))
	router.Handler(http.MethodDelete, "/admin/league/delete/:id", admin.ThenFunc(app.leagueDelete))
	router.Handler(http.MethodGet, "/admin/team/create", admin.ThenFunc(app.teamForm))
	router.Handler(http.MethodPost, "/admin/team/create", admin.ThenFunc(app.teamCreate))
	router.Handler(http.MethodGet, "/admin/team/update/:id", admin.ThenFunc(app.teamUpdate))
	router.Handler(http.MethodPost, "/admin/team/update", admin.ThenFunc(app.teamUpdatePost))
	router.Handler(http.MethodDelete, "/admin/team/delete/:id", admin.ThenFunc(app.teamDelete))
	router.Handler(http.MethodPost, "/admin/team/setCaptain", admin.ThenFunc(app.teamSetCaptain))
	router.Handler(http.MethodGet, "/admin/joinRequests", admin.ThenFunc(app.adminJoinRequestList))

	standard := alice.New(app.recoverPanic, app.logRequest, secureHeaders)

	return standard.Then(router)
}
