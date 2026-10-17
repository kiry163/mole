package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbeddedSPAFallback_DoesNotShadowAPIPaths(t *testing.T) {
	h := SPAHandler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("/api/* must stay 404, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/models", nil))
	// With an empty dist (only .gitkeep) the index.html fallback still reaches
	// http.FileServer, which answers 200 with a directory listing because there is
	// no index.html; once build artifacts are embedded the same path serves the
	// SPA entry point.
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusOK {
		t.Fatalf("unexpected status for SPA route: %d", rec.Code)
	}
}

func TestUnknownAPIPathStays404(t *testing.T) {
	rec := httptest.NewRecorder()
	SPAHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/not-found", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestMissingAssetStays404(t *testing.T) {
	rec := httptest.NewRecorder()
	SPAHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/not-found.js", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
