package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kiry163/mole/pkg/dashboard/middleware"
)

type fakeTokenStore struct {
	initialized bool
	token       string
	err         error
	reject      *string
}

func (s *fakeTokenStore) IsInitialized(context.Context) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.initialized, nil
}

func (s *fakeTokenStore) SetToken(_ context.Context, tok string) error {
	if s.err != nil {
		return s.err
	}
	if s.reject != nil && tok == *s.reject {
		return fmt.Errorf("%w: rejected", ErrInvalidToken)
	}
	s.token = tok
	s.initialized = true
	return nil
}

func (s *fakeTokenStore) VerifyToken(_ context.Context, tok string) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.initialized && tok == s.token, nil
}

func TestAuthLoginAndStatus(t *testing.T) {
	const token = "dashboard-test-token"
	const sess = "session-cookie-value"
	store := &fakeTokenStore{initialized: true, token: token}
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, AuthRouteOpts{
		SessionCookie: sess,
		TokenStore:    store,
	})

	t.Run("status_unauthenticated", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/status", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status code = %d", rec.Code)
		}
		var body struct {
			Authenticated bool `json:"authenticated"`
			Initialized   bool `json:"initialized"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Authenticated {
			t.Fatalf("unexpected authenticated=true: %+v", body)
		}
	})

	t.Run("login_ok", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"`+token+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:12345"
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("login code = %d body=%s", rec.Code, rec.Body.String())
		}
		cookies := rec.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Name != middleware.DashboardCookieName {
			t.Fatalf("cookies = %#v", cookies)
		}
	})

	t.Run("status_authenticated", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
		req.AddCookie(&http.Cookie{Name: middleware.DashboardCookieName, Value: sess})
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status code = %d", rec.Code)
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte(`"authenticated":true`)) {
			t.Fatalf("body = %s", rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "token_help") {
			t.Fatalf("authenticated response should omit token_help: %s", rec.Body.String())
		}
	})
}

func TestAuthUninitializedStoreRequiresSetup(t *testing.T) {
	const sess = "session-cookie-value"
	store := &fakeTokenStore{}
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, AuthRouteOpts{
		SessionCookie: sess,
		TokenStore:    store,
	})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d body=%s", rec.Code, rec.Body.String())
	}

	var body struct {
		Authenticated bool `json:"authenticated"`
		Initialized   bool `json:"initialized"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Initialized {
		t.Fatalf("initialized = true, want false before setup")
	}
	if body.Authenticated {
		t.Fatalf("unexpected authenticated=true: %+v", body)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"not-set-yet"}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("login before setup code = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(
		http.MethodPost,
		"/api/auth/setup",
		strings.NewReader(`{"password":"12345678","confirm":"12345678"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup code = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"12345678"}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login after setup code = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthSetupRequiresSessionWhenInitialized(t *testing.T) {
	const sess = "session-cookie-value"
	store := &fakeTokenStore{initialized: true, token: "old-token"}
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, AuthRouteOpts{
		SessionCookie: sess,
		TokenStore:    store,
	})

	body := strings.NewReader(`{"password":"new-password","confirm":"new-password"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/setup", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("setup without session code = %d body=%s", rec.Code, rec.Body.String())
	}

	body = strings.NewReader(`{"password":"new-password","confirm":"new-password"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/auth/setup", body)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: middleware.DashboardCookieName, Value: sess})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup with session code = %d body=%s", rec.Code, rec.Body.String())
	}
	if store.token != "new-password" {
		t.Fatalf("token = %q, want new-password", store.token)
	}
}

func TestAuthInitialSetupAllowsDirectSetup(t *testing.T) {
	store := &fakeTokenStore{}
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, AuthRouteOpts{
		SessionCookie: "session-cookie-value",
		TokenStore:    store,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/auth/setup",
		strings.NewReader(`{"password":"12345678","confirm":"12345678"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup without grant code = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthWithoutStoreFailsClosed(t *testing.T) {
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, AuthRouteOpts{
		SessionCookie: "session-cookie-value",
	})

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "status", method: http.MethodGet, path: "/api/auth/status"},
		{name: "login", method: http.MethodPost, path: "/api/auth/login", body: `{"password":"password"}`},
		{name: "setup", method: http.MethodPost, path: "/api/auth/setup", body: `{"password":"12345678","confirm":"12345678"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("code = %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAuthLogoutRequiresPostAndJSON(t *testing.T) {
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, AuthRouteOpts{
		SessionCookie: "session-cookie-value",
	})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/auth/logout", nil))
	if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
		t.Fatalf("GET logout: code = %d (expected 404 or 405)", rec.Code)
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("wrong content-type: code = %d body=%s", rec2.Code, rec2.Body.String())
	}

	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "/api/auth/logout", strings.NewReader(`{}`))
	req3.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("POST json logout: code = %d", rec3.Code)
	}
}

func TestAuthLoginRateLimit(t *testing.T) {
	store := &fakeTokenStore{initialized: true, token: "correct-token"}
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, AuthRouteOpts{
		SessionCookie: "session-cookie-value",
		TokenStore:    store,
	})

	// 11 failing logins by wrong password; each consumes allow() slot after valid JSON.
	wrongBody := `{"password":"wrong"}`
	for i := 0; i < loginAttemptsPerIP; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(wrongBody))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.168.5.5:9999"
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("iter %d: want 401 got %d %s", i, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(wrongBody))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.5.5:9999"
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("11th attempt: want 429 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestLoginRateLimiterWindow(t *testing.T) {
	l := newLoginRateLimiter()
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return t0 }
	for i := 0; i < loginAttemptsPerIP; i++ {
		if !l.allow("ip") {
			t.Fatalf("want allow at %d", i)
		}
	}
	if l.allow("ip") {
		t.Fatal("want deny on 11th")
	}
	l.now = func() time.Time { return t0.Add(loginAttemptWindow + time.Second) }
	if !l.allow("ip") {
		t.Fatal("want allow after window")
	}
}

func TestReferrerPolicyMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := middleware.ReferrerPolicyNoReferrer(next)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q", got)
	}
}

func TestAuthLogoutEmptyBody(t *testing.T) {
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, AuthRouteOpts{
		SessionCookie: "session-cookie-value",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = http.NoBody
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestAuthLogoutRejectsTrailingJSON(t *testing.T) {
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, AuthRouteOpts{
		SessionCookie: "session-cookie-value",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", strings.NewReader(`{}{}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestSetupRejectsEmptySessionCookie(t *testing.T) {
	// An empty expected session cookie must never authorize a request: a blank
	// cookie value compares equal to it, which would let anyone rewrite the
	// token of an initialized console.
	store := &fakeTokenStore{initialized: true, token: "existing-token"}
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, AuthRouteOpts{SessionCookie: "", TokenStore: store})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/auth/setup",
		strings.NewReader(`{"password":"rewritten-token","confirm":"rewritten-token"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: middleware.DashboardCookieName, Value: ""})
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("setup with empty expected cookie = %d body=%s, want 401", rec.Code, rec.Body.String())
	}
	if store.token != "existing-token" {
		t.Fatalf("store token = %q, want it unchanged", store.token)
	}
}

func TestSetupRejectsInvalidTokenValue(t *testing.T) {
	// A value the store refuses is a client error, not a server failure: the
	// caller has to be able to tell the two apart.
	rejected := "[NOT_HERE]"
	store := &fakeTokenStore{initialized: true, token: "existing-token", reject: &rejected}
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, AuthRouteOpts{SessionCookie: "session-cookie-value", TokenStore: store})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/auth/setup",
		strings.NewReader(`{"password":"[NOT_HERE]","confirm":"[NOT_HERE]"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: middleware.DashboardCookieName, Value: "session-cookie-value"})
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("setup with a rejected token = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
	if store.token != "existing-token" {
		t.Fatalf("store token = %q, want it unchanged", store.token)
	}
}
