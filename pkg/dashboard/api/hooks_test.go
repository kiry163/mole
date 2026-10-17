package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPatchConfigRejectsUnmountableHooks(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	patch := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPatch, "/api/config", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(rec, req)
		return rec
	}

	rec := patch(`{"hooks": {"processes": {"p": {"enabled": true, "transport": "grpc", "command": ["x"], "observe": ["turn_start"]}}}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unmountable hooks = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unsupported transport") {
		t.Fatalf("body = %s, want the mount rule message", rec.Body.String())
	}

	rec = patch(`{"hooks": {"processes": {"review": {"enabled": true, "command": ["true"], "observe": ["turn_start"]}}}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("mountable hooks = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
}

func TestListBuiltinHooks(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/hooks/builtins", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Builtins []string `json:"builtins"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v body=%s", err, rec.Body.String())
	}
	if resp.Builtins == nil {
		t.Fatal("builtins must be an array, not null")
	}
}
