package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/letitloose/league-buddy/internal/models"
	"github.com/letitloose/league-buddy/internal/services"
)

// locationListItem pairs a location with its address, so the list page can
// render the Google Maps link without a second round trip per row.
type locationListItem struct {
	Location *models.Location
	Address  *models.Address
}

func (app *application) locationList(w http.ResponseWriter, r *http.Request) {
	lm := &models.LocationModel{DB: app.playerService.DB}
	locations, err := lm.List()
	if err != nil {
		app.serverError(w, err)
		return
	}

	am := &models.AddressModel{DB: app.playerService.DB}
	items := make([]*locationListItem, 0, len(locations))
	for _, location := range locations {
		address, err := am.Get(location.AddressID)
		if err != nil {
			app.serverError(w, err)
			return
		}
		items = append(items, &locationListItem{Location: location, Address: address})
	}

	data := app.newTemplateData(r)
	data.Data = items
	data.Breadcrumbs = []Breadcrumb{{Label: "Locations"}}

	app.render(w, http.StatusOK, "location-list.html", data)
}

// locationFormBreadcrumbs is the shared "Locations / Add Location" trail for
// the create form and its validation-error re-render.
func locationFormBreadcrumbs() []Breadcrumb {
	return []Breadcrumb{
		{Label: "Locations", URL: "/location"},
		{Label: "Add Location"},
	}
}

func (app *application) locationForm(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = services.LocationForm{}
	data.SupportData = models.USStates
	data.Breadcrumbs = locationFormBreadcrumbs()
	app.render(w, http.StatusOK, "location-create.html", data)
}

func (app *application) locationCreate(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	form := &services.LocationForm{
		Name:          r.PostForm.Get("name"),
		Address1:      r.PostForm.Get("address1"),
		Address2:      r.PostForm.Get("address2"),
		City:          r.PostForm.Get("city"),
		StateProvince: r.PostForm.Get("stateprovince"),
		ZipCode:       r.PostForm.Get("zipcode"),
	}

	_, err = app.locationService.CreateLocation(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			data := app.newTemplateData(r)
			data.Form = form
			data.SupportData = models.USStates
			data.Breadcrumbs = locationFormBreadcrumbs()
			app.render(w, http.StatusUnprocessableEntity, "location-create.html", data)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", form.Name+" has been created!")
	http.Redirect(w, r, "/location", http.StatusSeeOther)
}

func (app *application) locationUpdate(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		app.notFound(w)
		return
	}

	lm := &models.LocationModel{DB: app.playerService.DB}
	location, err := lm.Get(id)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.notFound(w)
		} else {
			app.serverError(w, err)
		}
		return
	}

	am := &models.AddressModel{DB: app.playerService.DB}
	address, err := am.Get(location.AddressID)
	if err != nil {
		app.serverError(w, err)
		return
	}

	form := &services.LocationForm{
		ID:            location.ID,
		Name:          location.Name,
		Address1:      address.Address1.String,
		Address2:      address.Address2.String,
		City:          address.City.String,
		StateProvince: address.StateProvince.String,
		ZipCode:       address.ZipCode.String,
	}

	data := app.newTemplateData(r)
	data.Form = form
	data.SupportData = models.USStates
	data.Breadcrumbs = []Breadcrumb{
		{Label: "Locations", URL: "/location"},
		{Label: location.Name},
		{Label: "Edit"},
	}

	app.render(w, http.StatusOK, "location-update.html", data)
}

func (app *application) locationUpdatePost(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PostForm.Get("location-id"))
	if err != nil || id < 1 {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	form := &services.LocationForm{
		ID:            id,
		Name:          r.PostForm.Get("name"),
		Address1:      r.PostForm.Get("address1"),
		Address2:      r.PostForm.Get("address2"),
		City:          r.PostForm.Get("city"),
		StateProvince: r.PostForm.Get("stateprovince"),
		ZipCode:       r.PostForm.Get("zipcode"),
	}

	err = app.locationService.UpdateLocation(form, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrBadData) {
			data := app.newTemplateData(r)
			data.Form = form
			data.SupportData = models.USStates
			data.Breadcrumbs = []Breadcrumb{
				{Label: "Locations", URL: "/location"},
				{Label: form.Name},
				{Label: "Edit"},
			}
			app.render(w, http.StatusUnprocessableEntity, "location-update.html", data)
			return
		}
		app.serverError(w, err)
		return
	}

	app.sessionManager.Put(r.Context(), "flash", form.Name+" has been updated!")
	http.Redirect(w, r, "/location", http.StatusSeeOther)
}

func (app *application) locationDelete(w http.ResponseWriter, r *http.Request) {
	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.Atoi(params.ByName("id"))
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}

	err = app.locationService.DeleteLocation(id, app.getUserName(r))
	if err != nil {
		if errors.Is(err, models.ErrHasDependents) {
			tm := &models.TeamModel{DB: app.playerService.DB}
			teams, terr := tm.GetByLocation(id)
			if terr != nil {
				app.serverError(w, terr)
				return
			}
			names := make([]string, len(teams))
			for i, team := range teams {
				names[i] = team.Name
			}
			w.WriteHeader(http.StatusConflict)
			fmt.Fprintf(w, "Still the home field for: %s. Remove it from those teams first.", strings.Join(names, ", "))
			return
		}
		app.serverError(w, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}
