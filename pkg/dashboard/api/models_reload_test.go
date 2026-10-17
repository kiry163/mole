package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The Models page mutates the configuration the running gateway already read:
// the console must ask the host to reload so the agent loop picks up the new
// model_list and default model without a Config-page save or a restart. Every
// model write handler therefore has to request exactly one reload after a
// successful save, and none when the request is rejected.
//
// The reload counter uses the same runtime-injection mechanism as the other
// package tests (see config_reset_test.go): SetRuntime attaches a fake gateway
// whose Reload increments a counter.

func serveModelRequest(t *testing.T, h *Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	var reader *bytes.Buffer
	if body == "" {
		reader = bytes.NewBufferString("")
	} else {
		reader = bytes.NewBufferString(body)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	return rec
}

func TestHandleAddModel_RequestsReload(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	reloads := 0
	h := NewHandler(configPath)
	h.SetRuntime(&Runtime{Reload: func() { reloads++ }})

	rec := serveModelRequest(t, h, http.MethodPost, "/api/models", `{
		"model_name":"added-model",
		"provider":"openai",
		"api_base":"https://api.example.test/v1",
		"model":"gpt-4o",
		"api_key":"sk-added"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d, want exactly 1 after a successful model add", reloads)
	}
}

func TestHandleAddModel_NoReloadOnValidationFailure(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	reloads := 0
	h := NewHandler(configPath)
	h.SetRuntime(&Runtime{Reload: func() { reloads++ }})

	rec := serveModelRequest(t, h, http.MethodPost, "/api/models", `{
		"model_name":"bad-provider",
		"provider":"not-supported",
		"model":"gpt-4o-mini"
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if reloads != 0 {
		t.Fatalf("reloads = %d, want 0 when the add is rejected", reloads)
	}
}

func TestHandleUpdateModel_RequestsReload(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	reloads := 0
	h := NewHandler(configPath)
	h.SetRuntime(&Runtime{Reload: func() { reloads++ }})

	rec := serveModelRequest(t, h, http.MethodPut, "/api/models/0", `{
		"model_name":"custom-default",
		"provider":"openai",
		"api_base":"https://api.example.test/v1",
		"model":"gpt-4o-mini",
		"api_key":"sk-updated"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d, want exactly 1 after a successful model update", reloads)
	}
}

func TestHandleUpdateModel_NoReloadOnValidationFailure(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	reloads := 0
	h := NewHandler(configPath)
	h.SetRuntime(&Runtime{Reload: func() { reloads++ }})

	rec := serveModelRequest(t, h, http.MethodPut, "/api/models/0", `{
		"model_name":"custom-default",
		"provider":"not-supported",
		"model":"gpt-4o-mini"
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if reloads != 0 {
		t.Fatalf("reloads = %d, want 0 when the update is rejected", reloads)
	}
}

func TestHandleDeleteModel_RequestsReload(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	reloads := 0
	h := NewHandler(configPath)
	h.SetRuntime(&Runtime{Reload: func() { reloads++ }})

	rec := serveModelRequest(t, h, http.MethodDelete, "/api/models/0", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d, want exactly 1 after a successful model delete", reloads)
	}
}

func TestHandleDeleteModel_NoReloadOnValidationFailure(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	reloads := 0
	h := NewHandler(configPath)
	h.SetRuntime(&Runtime{Reload: func() { reloads++ }})

	rec := serveModelRequest(t, h, http.MethodDelete, "/api/models/99", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if reloads != 0 {
		t.Fatalf("reloads = %d, want 0 when the delete is rejected", reloads)
	}
}

func TestHandleSetDefaultModel_RequestsReload(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	reloads := 0
	h := NewHandler(configPath)
	h.SetRuntime(&Runtime{Reload: func() { reloads++ }})

	rec := serveModelRequest(t, h, http.MethodPost, "/api/models/default", `{
		"model_name":"custom-default"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d, want exactly 1 after a successful default-model change", reloads)
	}
}

func TestHandleSetDefaultModel_NoReloadOnValidationFailure(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	reloads := 0
	h := NewHandler(configPath)
	h.SetRuntime(&Runtime{Reload: func() { reloads++ }})

	rec := serveModelRequest(t, h, http.MethodPost, "/api/models/default", `{
		"model_name":"unknown-model"
	}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if reloads != 0 {
		t.Fatalf("reloads = %d, want 0 when the default-model change is rejected", reloads)
	}
}

func TestHandleUpdateDefaultChain_RequestsReload(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	reloads := 0
	h := NewHandler(configPath)
	h.SetRuntime(&Runtime{Reload: func() { reloads++ }})

	rec := serveModelRequest(t, h, http.MethodPut, "/api/models/default-chain", `{
		"default_model":"custom-default",
		"fallback_chain":[]
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d, want exactly 1 after a successful default-chain update", reloads)
	}
}

func TestHandleUpdateDefaultChain_NoReloadOnValidationFailure(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	reloads := 0
	h := NewHandler(configPath)
	h.SetRuntime(&Runtime{Reload: func() { reloads++ }})

	// An ASR-only provider cannot serve as a chat fallback, so the handler
	// rejects the chain before saving anything.
	rec := serveModelRequest(t, h, http.MethodPut, "/api/models/default-chain", `{
		"default_model":"custom-default",
		"fallback_chain":["elevenlabs/scribe_v1"]
	}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if reloads != 0 {
		t.Fatalf("reloads = %d, want 0 when the default-chain update is rejected", reloads)
	}
}
