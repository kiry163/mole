package dashboard

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/kiry163/mole/pkg/dashboard/middleware"
)

func TestNewServesDashboardAuthRoutesWithAPIRoutes(t *testing.T) {
	t.Setenv("MOLE_HOME", t.TempDir())
	s := New(filepath.Join(t.TempDir(), "config.json"))

	rec := httptest.NewRecorder()
	s.apiHandler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/auth/status status = %d, want %d (body %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestChainAllowsSetupPageAndGuardsConsoleRoutes(t *testing.T) {
	t.Setenv("MOLE_HOME", t.TempDir())
	s := New(filepath.Join(t.TempDir(), "config.json"))

	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	})
	handler := s.chain(next)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, middleware.DashboardSetupPath, nil))
	if !reached || rec.Code != http.StatusNoContent {
		t.Fatalf("setup page: reached = %t, status = %d, want true/204", reached, rec.Code)
	}

	reached = false
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/models", nil))
	if reached {
		t.Fatal("unauthenticated request reached the console handler")
	}
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login" {
		t.Fatalf("unauthenticated console route returned %d %q, want 302 /login", rec.Code, rec.Header().Get("Location"))
	}
}

func TestChainDoesNotSpecialCaseAutoLogin(t *testing.T) {
	t.Setenv("MOLE_HOME", t.TempDir())
	s := New(filepath.Join(t.TempDir(), "config.json"))

	reached := false
	handler := s.chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auto-login", nil))

	if reached {
		t.Fatal("/auto-login must not bypass dashboard auth")
	}
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login" {
		t.Fatalf("/auto-login returned %d %q, want 302 /login", rec.Code, rec.Header().Get("Location"))
	}
}
