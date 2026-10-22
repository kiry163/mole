package api

import (
	"net/http"
	"sync"
)

// Handler serves HTTP API requests.
type Handler struct {
	configPath string
	debug      bool
	// Serializes model-related config writes. Other config endpoints still
	// coordinate their own load-modify-save cycles.
	configMu    sync.Mutex
	weixinMu    sync.Mutex
	weixinFlows map[string]*weixinFlow
	// runtime is the gateway this console runs inside, attached by the host
	// through SetRuntime. It stays nil when the handler serves without one.
	runtimeMu sync.RWMutex
	runtime   *Runtime
	// routes caches the mux returned by Routes so every caller mounts the same
	// route table.
	routesOnce sync.Once
	routes     *http.ServeMux
}

// NewHandler creates an instance of the API handler.
func NewHandler(configPath string) *Handler {
	return &Handler{
		configPath:  configPath,
		weixinFlows: make(map[string]*weixinFlow),
	}
}

func (h *Handler) SetDebug(debug bool) {
	h.debug = debug
}

// RegisterRoutes binds all API endpoint handlers to the ServeMux. Every pattern
// lives under /api/, so a host mounts the mux on that prefix.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Config CRUD
	h.registerConfigRoutes(mux)

	// WebChat Channel (WebSocket chat)
	h.registerWebChatRoutes(mux)

	// In-process gateway runtime status and logs
	h.registerRuntimeRoutes(mux)

	// Session history
	h.registerSessionRoutes(mux)

	// Model list management
	h.registerModelRoutes(mux)

	// Channel catalog (for frontend navigation/config pages)
	h.registerChannelRoutes(mux)

	// Skills and tools support/actions
	h.registerSkillRoutes(mux)
	h.registerToolRoutes(mux)
	// Runtime build/version metadata
	h.registerVersionRoutes(mux)

	// WeChat QR login flow
	h.registerWeixinRoutes(mux)

}

// Routes returns the mux holding every API route, built on first use.
//
// Every pattern in the mux is under /api/, and the dashboard registers its auth
// endpoints on the same mux, so a host mounts the whole API surface by mounting
// Routes on the /api/ prefix of its own mux.
func (h *Handler) Routes() *http.ServeMux {
	h.routesOnce.Do(func() {
		h.routes = http.NewServeMux()
		h.RegisterRoutes(h.routes)
	})
	return h.routes
}
