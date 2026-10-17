package api

import (
	"net/http"

	"github.com/kiry163/mole/pkg/agent"
)

// registerHookRoutes binds hook support endpoints to the mux.
func (h *Handler) registerHookRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/hooks/builtins", h.handleListBuiltinHooks)
}

// handleListBuiltinHooks returns the builtin hooks this build can mount, so the
// console's hooks form can offer names instead of free text.
//
//	GET /api/hooks/builtins
func (h *Handler) handleListBuiltinHooks(w http.ResponseWriter, r *http.Request) {
	names := agent.BuiltinHookNames()
	if names == nil {
		names = []string{}
	}
	writeJSONResponse(w, map[string]any{"builtins": names})
}
