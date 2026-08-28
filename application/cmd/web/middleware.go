package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/justinas/nosurf"
	"github.com/letitloose/league-buddy/internal/models"
)

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", `default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; upgrade-insecure-requests`)
		w.Header().Set("Referrer-Policy", "origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "deny")
		w.Header().Set("X-XSS-Protection", "0")

		next.ServeHTTP(w, r)
	})
}

func (app *application) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RequestURI(), "/static") {
			app.infoLog.Printf("%s - %s %s %s", r.RemoteAddr, r.Proto, r.Method, r.URL.RequestURI())
		}
		next.ServeHTTP(w, r)
	})
}

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				w.Header().Set("Connection", "close")
				app.serverError(w, fmt.Errorf("%s", err))
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// loginRedirectURL builds a /user/login?next=... URL that carries r's own
// path+query through the login flow, so a logged-out hit on a deep link
// (an email's "RSVP Now!" link, for instance) lands back where it was
// headed instead of just the homepage. userLoginPost only honors next when
// isSafeNextURL approves it.
func loginRedirectURL(r *http.Request) string {
	return "/user/login?next=" + url.QueryEscape(r.URL.RequestURI())
}

func (app *application) requireAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !app.isAuthenticated(r) {
			http.Redirect(w, r, loginRedirectURL(r), http.StatusSeeOther)
			return
		}

		w.Header().Add("Cache-Control", "no-store")

		next.ServeHTTP(w, r)
	})
}

func (app *application) requireActive(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !app.isActive(r) {
			// Not authenticated at all: send them through login, carrying
			// this URL as next. Authenticated but not yet active: a login
			// redirect can't fix that (they need to activate their
			// account), so fall back to the homepage as before.
			if !app.isAuthenticated(r) {
				http.Redirect(w, r, loginRedirectURL(r), http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		w.Header().Add("Cache-Control", "no-store")

		next.ServeHTTP(w, r)
	})
}

func (app *application) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !app.isAdmin(r) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		w.Header().Add("Cache-Control", "no-store")

		next.ServeHTTP(w, r)
	})
}

// requireTeamManager allows Admins through unconditionally, and captains or
// league admins through when they manage the :teamID in the current route.
// Chain after app.authenticate + app.requireActive.
func (app *application) requireTeamManager(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := httprouter.ParamsFromContext(r.Context())
		teamID, err := strconv.Atoi(params.ByName("teamID"))
		if err != nil || teamID < 1 {
			app.notFound(w)
			return
		}

		if !app.canManageTeam(r, teamID) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		w.Header().Add("Cache-Control", "no-store")

		next.ServeHTTP(w, r)
	})
}

// requireLeagueManager allows Admins through unconditionally, and league
// admins through when they administer the league that owns the :teamID in
// the current route. Unlike requireTeamManager, a plain team captain is not
// enough — used only for team deletion. Chain after app.authenticate +
// app.requireActive.
func (app *application) requireLeagueManager(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := httprouter.ParamsFromContext(r.Context())
		teamID, err := strconv.Atoi(params.ByName("teamID"))
		if err != nil || teamID < 1 {
			app.notFound(w)
			return
		}

		if !app.canDeleteTeam(r, teamID) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		w.Header().Add("Cache-Control", "no-store")

		next.ServeHTTP(w, r)
	})
}

func noSurf(next http.Handler) http.Handler {
	csrfHandler := nosurf.New(next)
	csrfHandler.SetBaseCookie(http.Cookie{
		HttpOnly: true,
		Path:     "/",
		Secure:   true,
	})
	// nosurf's same-origin check (Origin/Referer) defaults to assuming every
	// request is HTTPS, which is right in production (TLS terminates at
	// nginx — r.TLS is nil at the Go process, but nginx-proxy sets
	// X-Forwarded-Proto: https) and right for the test suite (real TLS via
	// httptest.NewTLSServer, so r.TLS is non-nil). It's wrong for local dev
	// via docker-compose-dev.yml, which has no proxy in front and is genuine
	// plain HTTP — the hardcoded default would reject every real login there.
	csrfHandler.SetIsTLSFunc(func(r *http.Request) bool {
		return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	})

	return csrfHandler
}

func (app *application) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")

		if id == 0 {
			next.ServeHTTP(w, r)
			return
		}

		ac, err := app.userService.GetAuthContext(id)
		if err != nil {
			if errors.Is(err, models.ErrNoRecord) {
				next.ServeHTTP(w, r)
				return
			}
			app.serverError(w, err)
			return
		}

		ctx := context.WithValue(r.Context(), isAuthenticatedContextKey, true)
		r = r.WithContext(ctx)

		// realIsAdminContextKey is set unconditionally — it's what lets an
		// admin who's toggled into "view as player" still see the toggle
		// control to switch back, even though every other elevated field
		// below is suppressed for them exactly as if they held no special
		// roles at all (see the "view as player" admin feature).
		ctx = context.WithValue(r.Context(), realIsAdminContextKey, ac.IsAdmin)
		r = r.WithContext(ctx)

		viewingAsPlayer := ac.IsAdmin && app.sessionManager.GetBool(r.Context(), "viewAsPlayer")
		if viewingAsPlayer {
			ctx = context.WithValue(r.Context(), viewingAsPlayerContextKey, true)
			r = r.WithContext(ctx)
		}

		if ac.Active {
			ctx = context.WithValue(r.Context(), isActiveContextKey, true)
			r = r.WithContext(ctx)
		}
		if ac.IsAdmin && !viewingAsPlayer {
			ctx = context.WithValue(r.Context(), isAdminContextKey, true)
			r = r.WithContext(ctx)
		}
		if ac.PlayerID.Valid {
			ctx = context.WithValue(r.Context(), playerIDContextKey, int(ac.PlayerID.Int32))
			r = r.WithContext(ctx)
		}
		if len(ac.TeamIDs) > 0 {
			ctx = context.WithValue(r.Context(), teamIDsContextKey, ac.TeamIDs)
			r = r.WithContext(ctx)
		}
		if len(ac.CaptainTeamIDs) > 0 && !viewingAsPlayer {
			ctx = context.WithValue(r.Context(), captainTeamIDsContextKey, ac.CaptainTeamIDs)
			r = r.WithContext(ctx)
		}
		if len(ac.ScorekeeperTeamIDs) > 0 && !viewingAsPlayer {
			ctx = context.WithValue(r.Context(), scorekeeperTeamIDsContextKey, ac.ScorekeeperTeamIDs)
			r = r.WithContext(ctx)
		}
		if len(ac.LeagueAdminLeagueIDs) > 0 && !viewingAsPlayer {
			ctx = context.WithValue(r.Context(), leagueAdminLeagueIDsContextKey, ac.LeagueAdminLeagueIDs)
			r = r.WithContext(ctx)
		}
		if len(ac.LeagueAdminTeamIDs) > 0 && !viewingAsPlayer {
			ctx = context.WithValue(r.Context(), leagueAdminTeamIDsContextKey, ac.LeagueAdminTeamIDs)
			r = r.WithContext(ctx)
		}

		ctx = context.WithValue(r.Context(), userNameContextKey, ac.UserName)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}
