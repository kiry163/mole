package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kiry163/mole/pkg/logger"
)

// The Config page and the raw-config editor write the configuration the running
// gateway has already read. Like every other config-mutating handler, a
// successful PUT/PATCH /api/config must ask the host to reload so the channel
// manager and provider pick up the new config in place;
// a rejected request must not.
//
// The reload counter uses the same runtime-injection mechanism as the other
// package tests (see config_reset_test.go): SetRuntime attaches a fake gateway
// whose Reload increments a counter.

func serveConfigRequest(t *testing.T, h *Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	return rec
}

// isolateLogLevel keeps the reload tests from leaking a log level into the rest
// of the package: the handlers under test call applyRuntimeLogLevel().
func isolateLogLevel(t *testing.T) {
	t.Helper()

	initial := logger.GetLevel()
	logger.SetLevel(logger.INFO)
	t.Cleanup(func() { logger.SetLevel(initial) })
}

const validPutConfigBody = `{
	"version": 1,
	"agents": {
		"defaults": {
			"workspace": "~/.mole/workspace",
			"model_name": "custom-default"
		}
	},
	"model_list": [
		{
			"model_name": "custom-default",
			"model": "openai/gpt-4o",
			"api_keys": ["sk-default"]
		}
	]
}`

func TestHandleUpdateConfig_RequestsReload(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()
	isolateLogLevel(t)

	reloads := 0
	h := NewHandler(configPath)
	h.SetRuntime(&Runtime{Reload: func() { reloads++ }})

	rec := serveConfigRequest(t, h, http.MethodPut, "/api/config", validPutConfigBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d, want exactly 1 after a successful config update", reloads)
	}
}

func TestHandleUpdateConfig_NoReloadOnRejectedSave(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()
	isolateLogLevel(t)

	reloads := 0
	h := NewHandler(configPath)
	h.SetRuntime(&Runtime{Reload: func() { reloads++ }})

	rec := serveConfigRequest(t, h, http.MethodPut, "/api/config", `{"version": 1,`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if reloads != 0 {
		t.Fatalf("reloads = %d, want 0 after a rejected config update", reloads)
	}
}

func TestHandlePatchConfig_RequestsReload(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()
	isolateLogLevel(t)

	reloads := 0
	h := NewHandler(configPath)
	h.SetRuntime(&Runtime{Reload: func() { reloads++ }})

	rec := serveConfigRequest(t, h, http.MethodPatch, "/api/config", `{"gateway":{"log_level":"info"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d, want exactly 1 after a successful config patch", reloads)
	}
}

func TestHandlePatchConfig_NoReloadOnRejectedSave(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()
	isolateLogLevel(t)

	reloads := 0
	h := NewHandler(configPath)
	h.SetRuntime(&Runtime{Reload: func() { reloads++ }})

	rec := serveConfigRequest(t, h, http.MethodPatch, "/api/config", `{"gateway":{"log_level":"info"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if reloads != 0 {
		t.Fatalf("reloads = %d, want 0 after a rejected config patch", reloads)
	}
}
