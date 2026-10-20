package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/dashboard/api"
	"github.com/kiry163/mole/pkg/dashboard/middleware"
	"github.com/kiry163/mole/pkg/dashboard/utils"
	"github.com/kiry163/mole/pkg/gateway"
	"github.com/kiry163/mole/pkg/logger"
)

// Server holds the mole web console: the REST API, the login/setup pages and
// the embedded SPA, together with the session and token-store state that guards
// them.
//
// A Server never binds a listener and never supervises another process. The host
// owns the listener and mounts the three handlers through its own mux, wrapped
// in the middleware chain returned by chain.
type Server struct {
	apiHandler *api.Handler

	// pages serves the login/setup pages. Those routes live in the SPA bundle,
	// so the handler serves the same entry point as spa; it stays a separate
	// field so a host can mount them under the dashboard middleware explicitly.
	pages http.Handler
	spa   http.Handler

	// configPath is the absolute path of the mole config file the API reads and writes.
	configPath string
	moleHome   string

	// webchatConfigured reports that New had to enable the webchat channel (or
	// mint its token) in the config file. Mount uses it to ask the host for a
	// reload once, because the host already loaded the config before mounting.
	webchatConfigured bool

	// sessionCookie is the per-process dashboard session cookie value: the login
	// handler sets it and the auth middleware requires it.
	sessionCookie string
	// tokenStore backs dashboard token login. It reads the token from the
	// config's dashboard section (promoted to DASHBOARD_TOKEN in .env).
	tokenStore tokenStore
}

// Options configures one console mount on a host's HTTP mux.
type Options struct {
	// ConfigPath is the mole config file the console reads and writes.
	ConfigPath string
	// Debug enables the endpoints that expose raw config and command output.
	Debug bool
}

// Mount attaches the console to the running gateway described by env. It is the
// gateway.Options.Mount the `mole serve` command passes, so the console runs
// inside the gateway process: it shares the gateway listener and reports the
// gateway it is served by instead of supervising one of its own.
func Mount(env *gateway.RuntimeEnv, opts Options) error {
	s := New(opts.ConfigPath)

	s.SetDebug(opts.Debug)
	s.apiHandler.SetRuntime(&api.Runtime{
		Address:   env.Address,
		Version:   config.FormatVersion(),
		PID:       os.Getpid(),
		Channels:  env.Manager.GetEnabledChannels(),
		Reload:    env.RequestReload,
		Approvals: env.Approvals,
	})
	s.Register(env.Manager)

	// The workspace template is materialised before the console serves its first
	// request and before the reload below rebuilds the agent, so a fresh install
	// gets its definition files and skills into the very first turn's prompt.
	// Existing files are never touched: this is a per-file "create if missing".
	if env.Config != nil {
		ensureWorkspaceTemplate(env.Config.WorkspacePath())
	}

	// The host loaded the config before mounting, so a channel the console just
	// enabled is not running yet and its HTTP route is not on the shared mux.
	// Ask for one reload to pick it up; a steady-state start changed nothing and
	// stays a no-op.
	if s.webchatConfigured && env.RequestReload != nil {
		env.RequestReload()
	}

	return nil
}

// New builds the console handlers for the mole config file at configPath.
//
// It constructs the API routes (REST plus the dashboard auth endpoints), the
// login/setup pages and the SPA handler, and wires the token store, but binds
// nothing and starts nothing: a host mounts the handlers once it owns a
// listener.
func New(configPath string) *Server {
	s := &Server{
		moleHome: utils.GetMoleHome(),
	}
	if abs, err := filepath.Abs(configPath); err == nil {
		s.configPath = abs
	} else {
		logger.Errorf("Warning: failed to resolve config path %q: %v", configPath, err)
		s.configPath = configPath
	}

	// The cookie is keyed by the configured token, so a restart invalidates every
	// existing browser session and the token itself never reaches the browser.
	cookie, err := middleware.NewDashboardSessionCookie(s.openTokenStore())
	if err != nil {
		// Only a broken entropy source lands here; refusing to serve is the
		// fail-closed choice.
		logger.Fatalf("Dashboard auth setup failed: %v", err)
	}
	s.sessionCookie = cookie

	s.apiHandler = api.NewHandler(s.configPath)
	changed, err := s.apiHandler.EnsureWebChatChannel()
	if err != nil {
		logger.ErrorC("web", fmt.Sprintf("Warning: failed to ensure webchat channel on startup: %v", err))
	}
	s.webchatConfigured = changed
	api.RegisterAuthRoutes(s.apiHandler.Routes(), api.AuthRouteOpts{
		SessionCookie: s.sessionCookie,
		TokenStore:    s.tokenStore,
	})

	s.pages = SPAHandler()
	s.spa = SPAHandler()
	return s
}

// openTokenStore wires the config-backed token store, records whether the
// initial token still has to be set, says so loudly (an unset token means /setup
// is open to anyone who can reach the port), and returns the token so the caller
// does not have to read the config a second time.
func (s *Server) openTokenStore() string {
	s.tokenStore = newTokenStore(s.configPath)

	token, err := s.tokenStore.token(context.Background())
	if err != nil {
		logger.Fatalf("Dashboard auth setup failed: %v", err)
	}
	if token == "" {
		logger.WarnC("web", fmt.Sprintf(
			"No console access token configured: /setup is open to anyone who can reach this port; "+
				"set DASHBOARD_TOKEN in %s and reference it as $DASHBOARD_TOKEN in config.json",
			config.EnvFilePath(s.configPath)))
	}
	return token
}

// SetDebug mirrors the host's debug flag into the API handler, enabling the
// endpoints that expose raw config and command output.
func (s *Server) SetDebug(debug bool) {
	if s.apiHandler != nil {
		s.apiHandler.SetDebug(debug)
	}
}

// chain wraps one console handler in the middleware stack the dashboard runs
// behind, outermost first:
//
//	dashboard session auth
//	  -> Recoverer(Logger(ReferrerPolicyNoReferrer(JSONContentType(next))))
//
// The session check exempts the paths the middleware itself declares public
// (the login/setup pages, the embedded assets and the /api/auth/* verbs), so
// the pages and the login endpoints stay reachable without a session.
func (s *Server) chain(next http.Handler) http.Handler {
	authed := middleware.DashboardAuth(middleware.DashboardAuthConfig{
		ExpectedCookie: s.sessionCookie,
	}, next)

	return middleware.Recoverer(
		middleware.Logger(
			middleware.ReferrerPolicyNoReferrer(
				middleware.JSONContentType(authed),
			),
		),
	)
}

// Register mounts the console on the host's shared mux. It registers four
// patterns, each wrapped in chain so the middleware guards all of them:
//
//	/api/   the REST API, including the dashboard auth verbs
//	/login  the login page
//	/setup  the setup page
//	/       the embedded SPA, as the fallback for client-side routes
//
// The login and setup pages are mounted at the paths the auth middleware
// serves without a session, so the pages stay reachable before login.
//
// Anything the host registers itself - /health, /ready, /reload, channel
// webhooks such as /webchat/ - stays outside the console: the shared mux wins
// by longest pattern, so those paths never reach the console handlers and are
// never subject to dashboard authentication.
func (s *Server) Register(mux interface {
	HandleHTTP(pattern string, h http.Handler)
}) {
	mux.HandleHTTP("/api/", s.chain(s.apiHandler.Routes()))
	mux.HandleHTTP("/login", s.chain(s.pages))
	mux.HandleHTTP(middleware.DashboardSetupPath, s.chain(s.pages))
	mux.HandleHTTP("/", s.chain(s.spa))
}
