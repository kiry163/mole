package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewDashboardSessionCookie(t *testing.T) {
	a, err := NewDashboardSessionCookie("token-a")
	if err != nil {
		t.Fatalf("NewDashboardSessionCookie() error = %v", err)
	}
	b, err := NewDashboardSessionCookie("token-a")
	if err != nil {
		t.Fatalf("NewDashboardSessionCookie() second error = %v", err)
	}
	if a == b {
		t.Fatal("each process must derive a fresh cookie value")
	}
	if a == "" || b == "" {
		t.Fatal("cookie value must never be empty")
	}

	// No token yet: the console must still be able to serve /setup, so a value
	// is produced, but it is random rather than derived from the (absent) token.
	empty, err := NewDashboardSessionCookie("")
	if err != nil {
		t.Fatalf("NewDashboardSessionCookie(\"\") error = %v", err)
	}
	if empty == "" {
		t.Fatal("cookie value must not be empty even without a token")
	}
}

func TestValidDashboardAuth_RejectsEmptyCookie(t *testing.T) {
	// A blank expected cookie would otherwise authenticate a blank cookie value
	// (ConstantTimeCompare("", "") == 1) and open the console to everyone.
	cfg := DashboardAuthConfig{ExpectedCookie: ""}
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.AddCookie(&http.Cookie{Name: DashboardCookieName, Value: ""})

	if validDashboardAuth(req, cfg) {
		t.Fatal("an empty expected cookie must never authenticate")
	}
}

func TestDashboardAuth_AllowsPublicPaths(t *testing.T) {
	cfg := DashboardAuthConfig{ExpectedCookie: "deadbeef"}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := DashboardAuth(cfg, next)

	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/login", http.StatusTeapot},
		{http.MethodGet, "/setup", http.StatusTeapot},
		{http.MethodGet, "/assets/index.js", http.StatusTeapot},
		{http.MethodPost, "/api/auth/login", http.StatusTeapot},
		{http.MethodGet, "/api/auth/status", http.StatusTeapot},
		{http.MethodPost, "/api/auth/setup", http.StatusTeapot},
		{http.MethodPost, "/api/auth/logout", http.StatusTeapot},
		{http.MethodGet, "/api/auth/logout", http.StatusUnauthorized},
		{http.MethodGet, "/api/config", http.StatusUnauthorized},
		{http.MethodGet, "/webchat/ws", http.StatusFound},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s %s: status = %d, want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}
}

func TestDashboardAuth_QueryTokenDoesNotAuthenticate(t *testing.T) {
	cfg := DashboardAuthConfig{ExpectedCookie: "deadbeef"}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("next handler should not run without session cookie")
	})
	h := DashboardAuth(cfg, next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?token=secret", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login" {
		t.Fatalf("GET /?token=secret: code=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestDashboardAuth_DotDotCannotBypass(t *testing.T) {
	cfg := DashboardAuthConfig{ExpectedCookie: "deadbeef"}
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("next handler should not run without auth")
	})
	h := DashboardAuth(cfg, next)

	for _, p := range []string{
		"/assets/../api/config",
		"/login/../api/config",
		"/./api/config",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%q: status = %d, want %d", p, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestDashboardAuth_CookieOnly(t *testing.T) {
	cookieVal := "session-cookie-value"
	cfg := DashboardAuthConfig{ExpectedCookie: cookieVal}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := DashboardAuth(cfg, next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: DashboardCookieName, Value: cookieVal})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cookie auth: status = %d", rec.Code)
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req2.Header.Set("Authorization", "Bearer dashboard-secret-9")
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("bearer auth should not be accepted: status = %d", rec2.Code)
	}
}
