package main

import (
	"net/http"
	"testing"
)

func TestPublicRoutes(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	tests := []struct {
		name string
		path string
	}{
		{"home", "/"},
		{"login", "/user/login"},
		{"signup", "/user/signup"},
		{"forgot password", "/user/forgotPassword"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, _ := ts.get(t, tt.path)
			if code != http.StatusOK {
				t.Errorf("want %d; got %d", http.StatusOK, code)
			}
		})
	}
}

// Unauthenticated requests to active and admin routes are redirected to /.
func TestUnauthenticatedAccessRedirects(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())

	tests := []struct {
		name string
		path string
	}{
		{"player list", "/player"},
		{"player search", "/player/search"},
		{"player create", "/player/create"},
		{"user search", "/user/search"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, headers, _ := ts.get(t, tt.path)
			if code != http.StatusSeeOther {
				t.Errorf("want %d; got %d", http.StatusSeeOther, code)
			}
			if loc := headers.Get("Location"); loc != "/" {
				t.Errorf("want Location %q; got %q", "/", loc)
			}
		})
	}
}

// Active (non-admin) users are redirected away from admin-only routes.
func TestAdminRoutesRedirectActiveNonAdmin(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	ts.login(t, testActiveEmail, testActivePass)

	getRoutes := []struct {
		name string
		path string
	}{
		{"user search", "/user/search"},
		{"player create form", "/player/create"},
	}

	for _, tt := range getRoutes {
		t.Run(tt.name, func(t *testing.T) {
			code, headers, _ := ts.get(t, tt.path)
			if code != http.StatusSeeOther {
				t.Errorf("want %d; got %d", http.StatusSeeOther, code)
			}
			if loc := headers.Get("Location"); loc != "/" {
				t.Errorf("want Location %q; got %q", "/", loc)
			}
		})
	}

	t.Run("player delete", func(t *testing.T) {
		code, headers, _ := ts.delete(t, "/player/delete/1")
		if code != http.StatusSeeOther {
			t.Errorf("want %d; got %d", http.StatusSeeOther, code)
		}
		if loc := headers.Get("Location"); loc != "/" {
			t.Errorf("want Location %q; got %q", "/", loc)
		}
	})
}

// Active users can access active-chain routes.
func TestActiveRoutes(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	ts.login(t, testActiveEmail, testActivePass)

	tests := []struct {
		name string
		path string
	}{
		{"player list", "/player"},
		{"player search", "/player/search"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, _ := ts.get(t, tt.path)
			if code != http.StatusOK {
				t.Errorf("want %d; got %d", http.StatusOK, code)
			}
		})
	}
}

// Admin users can access admin-chain routes.
func TestAdminRoutes(t *testing.T) {
	app := newTestApplication(t)
	ts := newTestServer(t, app.routes())
	ts.login(t, testAdminEmail, testAdminPass)

	tests := []struct {
		name string
		path string
	}{
		{"user search", "/user/search"},
		{"player create form", "/player/create"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, _ := ts.get(t, tt.path)
			if code != http.StatusOK {
				t.Errorf("want %d; got %d", http.StatusOK, code)
			}
		})
	}
}
