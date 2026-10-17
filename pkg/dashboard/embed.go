// Package dashboard embeds the compiled console SPA and serves it over HTTP.
package dashboard

import (
	"embed"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/kiry163/mole/pkg/logger"
)

//go:embed all:dist
var frontendFS embed.FS

// SPAHandler returns the HTTP handler that serves the embedded frontend files,
// falling back to index.html for client-side routes. Unknown /api paths are kept
// as 404 so they never resolve to the SPA entry point.
//
// The embedded dist/ directory only ever holds the tracked .gitkeep placeholder
// until a frontend build fills it. fs.Sub can therefore only fail if the embed
// directive and the on-disk tree disagree, which is a build-time bug: a missing
// dist/ already breaks compilation. The signature deliberately carries no error,
// and this package cannot decide to abort the host process, so a failure is
// logged and degrades to a handler that answers 404 for every route - exactly
// what callers observed before the split, when a missing dist/ left the route
// unregistered and the mux answered 404.
func SPAHandler() http.Handler {
	registerSVGMIME()

	// Attempt to get the subdirectory 'dist' where Vite usually builds
	subFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		// Log a warning if dist doesn't exist yet (e.g., during development before a frontend build)
		logger.WarnC("web",
			"Warning: no 'dist' folder found in embedded frontend. "+
				"Ensure you run `pnpm build:backend` in the frontend directory "+
				"before building the Go backend.",
		)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
	}

	fileServer := http.FileServer(http.FS(subFS))

	// Serve static assets and fallback to index.html for SPA routes.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}

		// Keep unknown API paths as 404 instead of falling back to SPA entry.
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		cleanPath := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if cleanPath == "." {
			cleanPath = ""
		}

		// Existing static files/directories should be served directly.
		if cleanPath != "" {
			if _, statErr := fs.Stat(subFS, cleanPath); statErr == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
			// Missing asset-like paths should remain 404.
			if strings.Contains(path.Base(cleanPath), ".") {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		indexReq := r.Clone(r.Context())
		indexReq.URL.Path = "/"
		fileServer.ServeHTTP(w, indexReq)
	})
}

// registerSVGMIME registers the correct MIME type for SVG files.
// Go's built-in mime.TypeByExtension returns "image/svg" which is incorrect;
// the correct MIME type per RFC 6838 is "image/svg+xml".
func registerSVGMIME() {
	if err := mime.AddExtensionType(".svg", "image/svg+xml"); err != nil {
		logger.ErrorC("web", fmt.Sprintf("Warning: failed to register SVG MIME type: %v", err))
	}
}
