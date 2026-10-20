package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"path"
	"strings"
	"time"
)

// DashboardCookieName is the HttpOnly cookie set after a successful password login.
const DashboardCookieName = "mole_dashboard_auth"

// dashboardSessionMaxAgeSec is the dashboard session cookie lifetime (31 days).
const dashboardSessionMaxAgeSec = 31 * 24 * 3600

const (
	sessionCookieBytes = 32
	// DashboardSetupPath is the setup page used before the dashboard
	// password is initialized.
	DashboardSetupPath = "/setup"
)

// randomBytes returns n cryptographically random bytes.
func randomBytes(n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// NewDashboardSessionCookie derives the console session cookie value for this
// process: HMAC-SHA256 keyed by the configured token over a per-process random
// nonce. The token itself never reaches the browser, and every restart
// invalidates existing cookies. Without a token (first run) the value is random:
// nobody can authenticate until /setup stores one, but /setup stays reachable.
func NewDashboardSessionCookie(token string) (string, error) {
	token = strings.TrimSpace(token)
	nonce, err := randomBytes(sessionCookieBytes)
	if err != nil {
		return "", err
	}
	if token == "" {
		return base64.RawURLEncoding.EncodeToString(nonce), nil
	}
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write(nonce)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// DashboardAuthConfig holds runtime material for dashboard access checks.
type DashboardAuthConfig struct {
	ExpectedCookie string
}

// DefaultDashboardSecureCookie mirrors typical production HTTPS detection (TLS or X-Forwarded-Proto).
func DefaultDashboardSecureCookie(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// SetDashboardSessionCookie writes the HttpOnly session cookie after successful dashboard password login.
func SetDashboardSessionCookie(w http.ResponseWriter, r *http.Request, sessionValue string) {
	http.SetCookie(w, &http.Cookie{
		Name:     DashboardCookieName,
		Value:    sessionValue,
		Path:     "/",
		MaxAge:   dashboardSessionMaxAgeSec,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   DefaultDashboardSecureCookie(r),
	})
}

// ClearDashboardSessionCookie clears the dashboard session (e.g. logout).
func ClearDashboardSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     DashboardCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   DefaultDashboardSecureCookie(r),
		Expires:  time.Unix(0, 0),
	})
}

// DashboardAuth requires a valid session cookie before calling next.
// Public paths are login/setup pages and /api/auth/* handlers.
func DashboardAuth(cfg DashboardAuthConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := canonicalAuthPath(r.URL.Path)
		if isPublicDashboardPath(r.Method, p) {
			next.ServeHTTP(w, r)
			return
		}
		if validDashboardAuth(r, cfg) {
			next.ServeHTTP(w, r)
			return
		}
		rejectDashboardAuth(w, r, p)
	})
}

// canonicalAuthPath matches path cleaning used for routing decisions so
// prefixes like /assets/../ cannot bypass auth (CVE-class traversal).
func canonicalAuthPath(raw string) string {
	if raw == "" {
		return "/"
	}
	c := path.Clean(raw)
	switch c {
	case ".", "":
		return "/"
	default:
		if c[0] != '/' {
			return "/" + c
		}
		return c
	}
}

func isPublicDashboardPath(method, p string) bool {
	if isPublicDashboardStatic(method, p) {
		return true
	}
	switch p {
	case "/api/auth/login":
		return method == http.MethodPost
	case "/api/auth/logout":
		return method == http.MethodPost
	case "/api/auth/status":
		return method == http.MethodGet
	case "/api/auth/setup":
		return method == http.MethodPost
	}
	return false
}

// isPublicDashboardStatic allows the SPA login route and embedded
// frontend assets without a session (GET/HEAD only).
func isPublicDashboardStatic(method, p string) bool {
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	if p == "/login" || p == "/setup" {
		return true
	}
	if strings.HasPrefix(p, "/assets/") {
		return true
	}
	switch p {
	case "/favicon.ico", "/favicon.svg", "/favicon-96x96.png",
		"/apple-touch-icon.png", "/site.webmanifest", "/robots.txt":
		return true
	default:
		return false
	}
}

// ValidDashboardSessionCookie reports whether value matches the expected
// session cookie in constant time. An empty expectation never matches: a blank
// cookie value compares equal to a blank expectation
// (subtle.ConstantTimeCompare("", "") == 1), which would open the console to
// everyone.
func ValidDashboardSessionCookie(value, expected string) bool {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(expected) == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(value), []byte(expected)) == 1
}

func validDashboardAuth(r *http.Request, cfg DashboardAuthConfig) bool {
	c, err := r.Cookie(DashboardCookieName)
	if err != nil {
		return false
	}
	return ValidDashboardSessionCookie(c.Value, cfg.ExpectedCookie)
}

func rejectDashboardAuth(w http.ResponseWriter, r *http.Request, canonicalPath string) {
	if strings.HasPrefix(canonicalPath, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}
