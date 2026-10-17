package dashboard

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"
)

// fakeMux records the patterns a host registers on its shared gateway mux.
type fakeMux struct{ reg map[string]http.Handler }

func (m *fakeMux) HandleHTTP(pattern string, h http.Handler) { m.reg[pattern] = h }

func newFakeMux() *fakeMux { return &fakeMux{reg: map[string]http.Handler{}} }

func TestRegisterMountsFourPatterns(t *testing.T) {
	t.Setenv("MOLE_HOME", t.TempDir())
	s := New(filepath.Join(t.TempDir(), "config.json"))

	m := newFakeMux()
	s.Register(m)

	// "/api/" is the REST surface, the two page paths are the routes the auth
	// middleware serves without a session, and "/" is the SPA fallback.
	want := []string{"/", "/api/", "/login", "/setup"}
	for _, p := range want {
		if m.reg[p] == nil {
			t.Fatalf("pattern %q not mounted", p)
		}
	}
	if len(m.reg) != len(want) {
		t.Fatalf("mounted %d patterns, want %d (got %v)", len(m.reg), len(want), mountedPatterns(m))
	}

	// Channel webhooks are registered by the channel manager on the same mux and
	// must never be shadowed by the console's "/" fallback.
	if slices.Contains(mountedPatterns(m), "/webchat/") {
		t.Fatal("dashboard must not mount channel paths")
	}
}

func TestRegisterPutsTheAPISurfaceBehindTheAuthChain(t *testing.T) {
	t.Setenv("MOLE_HOME", t.TempDir())
	s := New(filepath.Join(t.TempDir(), "config.json"))

	m := newFakeMux()
	s.Register(m)

	// An unauthenticated REST call is refused by the session check, which proves
	// the API is reached through the middleware chain and not mounted raw.
	rec := httptest.NewRecorder()
	m.reg["/api/"].ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/gateway/status", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf(
			"unauthenticated GET /api/gateway/status status = %d, want %d (body %s)",
			rec.Code, http.StatusUnauthorized, rec.Body.String(),
		)
	}

	// The login/setup pages stay public, so the chain lets them through.
	rec = httptest.NewRecorder()
	m.reg["/setup"].ServeHTTP(
		rec, httptest.NewRequest(http.MethodGet, "/setup", nil),
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /setup status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func mountedPatterns(m *fakeMux) []string {
	patterns := make([]string, 0, len(m.reg))
	for pattern := range m.reg {
		patterns = append(patterns, pattern)
	}
	slices.Sort(patterns)
	return patterns
}
