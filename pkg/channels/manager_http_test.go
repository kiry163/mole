package channels

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestManagerHandleHTTP_ServesDashboardAndKeepsChannelPriority(t *testing.T) {
	m := &Manager{}
	m.mux = newDynamicServeMux()
	m.mux.Handle("/webchat/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	m.HandleHTTP("/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// A channel prefix is longer, so it must take priority.
	req := httptest.NewRequest(http.MethodGet, "/webchat/ws", nil)
	rec := httptest.NewRecorder()
	m.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusTeapot {
		t.Fatalf("channel handler should win for /webchat/ws, got %d", rec.Code)
	}

	// SPA fallback.
	req = httptest.NewRequest(http.MethodGet, "/models", nil)
	rec = httptest.NewRecorder()
	m.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard fallback should serve /models, got %d", rec.Code)
	}

	m.UnhandleHTTP("/")
	req = httptest.NewRequest(http.MethodGet, "/models", nil)
	rec = httptest.NewRecorder()
	m.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("after UnhandleHTTP expect 404, got %d", rec.Code)
	}
}
