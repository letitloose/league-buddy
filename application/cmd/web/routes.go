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
	router.Handler(http.MethodGet, "/player", active.ThenFunc(app.playerList))
	router.Handler(http.MethodGet, "/player/search", active.ThenFunc(app.playerSearch))
	router.Handler(http.MethodGet, "/player/view/:id", active.ThenFunc(app.playerView))
	router.Handler(http.MethodGet, "/player/update/:id", active.ThenFunc(app.playerUpdate))
	router.Handler(http.MethodPost, "/player/update", active.ThenFunc(app.playerUpdatePost))

	// admin routes (logged in + active + ADMIN role)
	admin := dynamic.Append(app.requireAdmin)
	router.Handler(http.MethodGet, "/player/create", admin.ThenFunc(app.playerForm))
	router.Handler(http.MethodPost, "/player/create", admin.ThenFunc(app.playerCreate))
	router.Handler(http.MethodDelete, "/player/delete/:id", admin.ThenFunc(app.playerDelete))
	router.Handler(http.MethodGet, "/user/search", admin.ThenFunc(app.userSearch))
	router.Handler(http.MethodGet, "/user/view/:id", admin.ThenFunc(app.userView))
	router.Handler(http.MethodPost, "/user/toggleActive", admin.ThenFunc(app.toggleActive))
	router.Handler(http.MethodPost, "/user/toggleAdmin", admin.ThenFunc(app.toggleAdmin))
	router.Handler(http.MethodDelete, "/user/delete/:id", admin.ThenFunc(app.deleteUser))

	standard := alice.New(app.recoverPanic, app.logRequest, secureHeaders)

	return standard.Then(router)
}
