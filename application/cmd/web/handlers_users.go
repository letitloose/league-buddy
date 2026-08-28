package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/services"
)

func (app *application) userSignup(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = services.UserForm{InviteToken: r.URL.Query().Get("invite")}
	app.render(w, http.StatusOK, "signup.html", data)
}

func (app *application) forgotPassword(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = services.UserForm{}
	app.render(w, http.StatusOK, "forgot-password.html", data)
}

func (app *application) forgotPasswordPost(w http.ResponseWriter, r *http.Request) {

	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	form := services.UserForm{
		Email: r.PostForm.Get("email"),
	}

	err = app.userService.ForgotPassword(&form)

	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			data := app.newTemplateData(r)
			data.Form = form
			app.render(w, http.StatusUnprocessableEntity, "forgot-password.html", data)
			return
		} else {
			app.serverError(w, err)
		}

		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Please check your email for a link to reset your password.")
	http.Redirect(w, r, "/user/login", http.StatusSeeOther)
}

func (app *application) resetPassword(w http.ResponseWriter, r *http.Request) {

	hash := r.URL.Query().Get("hash")
	if hash == "" {
		app.notFound(w)
		return
	}

	user, err := app.userService.GetByVerificationHash(hash)
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Form = services.UserForm{Email: user.Email}
	app.render(w, http.StatusOK, "reset-password.html", data)
}

func (app *application) resetPasswordPost(w http.ResponseWriter, r *http.Request) {

	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	form := services.UserForm{
		Email:           r.PostForm.Get("email"),
		Password:        r.PostForm.Get("password"),
		ConfirmPassword: r.PostForm.Get("confirmPassword"),
	}

	err = app.userService.ResetPassword(&form)

	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			data := app.newTemplateData(r)
			data.Form = form
			app.render(w, http.StatusUnprocessableEntity, "reset-password.html", data)
			return
		} else {
			app.serverError(w, err)
		}

		return
	}

	app.sessionManager.Put(r.Context(), "flash", "Your password has been reset!")
	http.Redirect(w, r, "/user/login", http.StatusSeeOther)
}

func (app *application) userSignupPost(w http.ResponseWriter, r *http.Request) {

	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	form := services.UserForm{
		Email:           r.PostForm.Get("email"),
		Password:        r.PostForm.Get("password"),
		ConfirmPassword: r.PostForm.Get("confirmPassword"),
		InviteToken:     r.PostForm.Get("inviteToken"),
	}

	err = app.userService.InsertUser(&form)

	if err != nil {
		if errors.Is(err, models.ErrDuplicateEmail) {
			form.AddFieldError("email", "Email address is already in use")
			data := app.newTemplateData(r)
			data.Form = form
			app.render(w, http.StatusUnprocessableEntity, "signup.html", data)
		} else if errors.Is(err, models.ErrBadData) {
			data := app.newTemplateData(r)
			data.Form = form
			app.render(w, http.StatusUnprocessableEntity, "signup.html", data)
			return
		} else {
			app.serverError(w, err)
		}

		return
	}
	app.sessionManager.Put(r.Context(), "flash", "Your signup was successful. Please check your email to activate your account.")
	http.Redirect(w, r, "/user/login", http.StatusSeeOther)
}

// isSafeNextURL reports whether next is safe to redirect to after login: a
// same-origin path (starts with exactly one "/"), never an absolute URL or
// protocol-relative one ("//evil.example.com") that would send the user
// somewhere else entirely.
func isSafeNextURL(next string) bool {
	return strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//")
}

func (app *application) userLogin(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = services.UserForm{}
	if next := r.URL.Query().Get("next"); isSafeNextURL(next) {
		data.NextURL = next
	}
	app.render(w, http.StatusOK, "login.html", data)
}

func (app *application) userLoginPost(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	next := r.PostForm.Get("next")
	if !isSafeNextURL(next) {
		next = ""
	}

	form := services.UserForm{
		Email:    r.PostForm.Get("email"),
		Password: r.PostForm.Get("password"),
	}

	id, err := app.userService.AuthenticateUser(&form)
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			data := app.newTemplateData(r)
			data.Form = form
			data.NextURL = next
			app.render(w, http.StatusUnprocessableEntity, "login.html", data)
			return
		} else if errors.Is(err, models.ErrInvalidCredentials) {
			form.AddNonFieldError("Email or password is incorrect")
			data := app.newTemplateData(r)
			data.Form = form
			data.NextURL = next
			app.render(w, http.StatusUnprocessableEntity, "login.html", data)
		} else {
			app.serverError(w, err)
		}
		return
	}

	err = app.sessionManager.RenewToken(r.Context())
	if err != nil {
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "authenticatedUserID", id)

	if next != "" {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (app *application) userLogoutPost(w http.ResponseWriter, r *http.Request) {
	err := app.sessionManager.RenewToken(r.Context())
	if err != nil {
		app.serverError(w, err)
		return
	}
	app.sessionManager.Remove(r.Context(), "authenticatedUserID")
	app.sessionManager.Put(r.Context(), "flash", "You've been logged out successfully!")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (app *application) activateUser(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("hash")
	if hash == "" {
		app.notFound(w)
		return
	}

	err := app.userService.ActivateUser(hash)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}

	data := app.newTemplateData(r)
	app.render(w, http.StatusOK, "activated.html", data)
}

func (app *application) toggleActive(w http.ResponseWriter, r *http.Request) {

	user := &services.UserPost{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(user)
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	loggedInUserID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
	err = app.userService.ToggleActive(user.ID, loggedInUserID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (app *application) toggleAdmin(w http.ResponseWriter, r *http.Request) {
	user := &services.UserPost{}
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(user)
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	loggedInUserID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
	err = app.userService.ToggleAdmin(user.ID, loggedInUserID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (app *application) deleteUser(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}

	loggedInUserID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
	err = app.userService.DeleteUser(id, loggedInUserID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (app *application) userSearch(w http.ResponseWriter, r *http.Request) {
	form := services.UserSearchForm{UserSearchCriteria: &models.UserSearchCriteria{Limit: 20}}

	var err error
	params := r.URL.Query()

	form.FirstName = params.Get("firstname")
	form.LastName = params.Get("lastname")
	form.Email = params.Get("email")
	form.Sort = params.Get("sort")
	form.Order = params.Get("order")
	if form.Sort == "" {
		// Same default UserModel.buildUserSearchStatement falls back to —
		// made explicit here too so the template can render the Last Login
		// column as the active sort (and compute the right toggle
		// direction) on a plain, param-less page load.
		form.Sort = "lastlogin"
		form.Order = "DESC"
	}

	offset := params.Get("offset")
	if len(offset) > 0 {
		form.Offset, err = strconv.Atoi(offset)
		if err != nil {
			app.clientError(w, http.StatusBadRequest)
			return
		}
	}

	limit := params.Get("limit")
	if len(limit) > 0 {
		form.Limit, err = strconv.Atoi(limit)
		if err != nil {
			app.clientError(w, http.StatusBadRequest)
			return
		}
	} else {
		form.Limit = 20
	}

	results, err := app.userService.Search(form.UserSearchCriteria)
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Form = form
	data.Data = results

	app.render(w, http.StatusOK, "user-list.html", data)
}

func (app *application) userView(w http.ResponseWriter, r *http.Request) {

	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}

	user, err := app.userService.GetUser(id)
	if err != nil {
		app.serverError(w, err)
		return
	}

	data := app.newTemplateData(r)
	data.Data = user
	data.Breadcrumbs = []Breadcrumb{
		{Label: "Users", URL: "/user/search"},
		{Label: user.Email},
	}

	app.render(w, http.StatusOK, "user-view.html", data)
}
