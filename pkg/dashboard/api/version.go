package api

import (
	"encoding/json"
	"net/http"

	"github.com/kiry163/mole/pkg/config"
)

// systemVersionResponse reports the build metadata of the running process, which
// serves both the gateway and the console.
type systemVersionResponse struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit,omitempty"`
	BuildTime string `json:"build_time,omitempty"`
	GoVersion string `json:"go_version"`
}

func (h *Handler) registerVersionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/system/version", h.handleGetVersion)
}

// handleGetVersion returns runtime version information for web clients.
//
//	GET /api/system/version
func (h *Handler) handleGetVersion(w http.ResponseWriter, _ *http.Request) {
	buildTime, goVersion := config.FormatBuildInfo()
	versionInfo := systemVersionResponse{
		Version:   config.FormatVersion(),
		GitCommit: config.GitCommit,
		BuildTime: buildTime,
		GoVersion: goVersion,
	}
	if runtime := h.currentRuntime(); runtime != nil && runtime.Version != "" {
		versionInfo.Version = runtime.Version
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(versionInfo); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
