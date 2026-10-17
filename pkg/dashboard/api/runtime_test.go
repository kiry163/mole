package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRuntimeStatusReportsInProcessGateway(t *testing.T) {
	h := NewHandler("") // no configPath needed while only the status shape is under test
	h.SetRuntime(&Runtime{Address: "127.0.0.1:18790", Version: "test", PID: 4242, Channels: []string{"webchat"}})

	rec := httptest.NewRecorder()
	h.handleRuntimeStatus(rec, httptest.NewRequest(http.MethodGet, "/api/gateway/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "running" {
		t.Fatalf("unexpected status payload: %v", got)
	}
}

func TestRuntimeStatusPayloadKeepsSPAFieldNames(t *testing.T) {
	h := NewHandler("")
	h.SetRuntime(&Runtime{
		Address:  "127.0.0.1:18790",
		Version:  "1.2.3 (git: abc1234)",
		PID:      4242,
		Channels: []string{"webchat", "weixin"},
	})

	rec := httptest.NewRecorder()
	h.handleRuntimeStatus(rec, httptest.NewRequest(http.MethodGet, "/api/gateway/status", nil))

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"status", "pid", "version", "address", "channels",
	} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("payload %v misses the SPA field %q", raw, key)
		}
	}

	var got runtimeStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != runtimeStatusRunning {
		t.Fatalf("status = %q, want %q", got.Status, runtimeStatusRunning)
	}
	if got.PID != 4242 {
		t.Fatalf("pid = %d, want 4242", got.PID)
	}
	if got.Version != "1.2.3 (git: abc1234)" {
		t.Fatalf("version = %q", got.Version)
	}
	if got.Address != "127.0.0.1:18790" {
		t.Fatalf("address = %q", got.Address)
	}
	if !slices.Equal(got.Channels, []string{"webchat", "weixin"}) {
		t.Fatalf("channels = %v", got.Channels)
	}
}

func TestRuntimeStatusWithoutAttachedRuntimeReportsUnknown(t *testing.T) {
	h := NewHandler("")

	rec := httptest.NewRecorder()
	h.handleRuntimeStatus(rec, httptest.NewRequest(http.MethodGet, "/api/gateway/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["status"] != runtimeStatusUnknown {
		t.Fatalf("status = %#v, want %q", raw["status"], runtimeStatusUnknown)
	}
	if channels, ok := raw["channels"].([]any); !ok || len(channels) != 0 {
		t.Fatalf("channels = %#v, want an empty array", raw["channels"])
	}
}

func TestRuntimeRoutesAreRegistered(t *testing.T) {
	t.Setenv("MOLE_HOME", t.TempDir())

	h := NewHandler("")
	mux := h.Routes()

	for _, path := range []string{"/api/gateway/status", "/api/gateway/logs"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want %d", path, rec.Code, http.StatusOK)
		}
	}

	// The console no longer starts, stops or restarts a process it runs inside,
	// so those endpoints must not exist; the status path answers 405 instead.
	for _, path := range []string{"/api/gateway/start", "/api/gateway/stop", "/api/gateway/restart"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("POST %s status = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/gateway/status", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/gateway/status status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}

	// Every route lives under /api/, so a host mounts the mux on that prefix.
	for _, path := range []string{"/webchat/ws", "/webchat/media/attachment-1"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}

func TestHandleRuntimeLogsTailsGatewayLogFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MOLE_HOME", home)

	logPath := filepath.Join(home, "logs", "gateway.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	lines := make([]string, 0, 500)
	for i := range 500 {
		lines = append(lines, fmt.Sprintf("line-%03d", i))
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	h := NewHandler("")

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "default window is the newest 200 lines", query: "", want: lines[300:]},
		{name: "explicit limit", query: "?limit=3", want: lines[497:]},
		{name: "offset counts back from the tail", query: "?offset=2&limit=3", want: lines[495:498]},
		{name: "offset past the start of the file", query: "?offset=1000&limit=10", want: []string{}},
		{name: "malformed limit falls back to the default", query: "?limit=nope", want: lines[300:]},
		{name: "negative limit falls back to the default", query: "?limit=-5", want: lines[300:]},
		{name: "negative offset falls back to the newest lines", query: "?offset=-5&limit=2", want: lines[498:]},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.handleRuntimeLogs(rec, httptest.NewRequest(http.MethodGet, "/api/gateway/logs"+tt.query, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
			}

			var got runtimeLogsResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if !slices.Equal(got.Logs, tt.want) {
				t.Fatalf("logs = %v, want %v", got.Logs, tt.want)
			}
			if got.LogTotal != len(lines) {
				t.Fatalf("log_total = %d, want %d", got.LogTotal, len(lines))
			}
		})
	}
}

func TestHandleRuntimeLogsReportsEmptyMissingFile(t *testing.T) {
	t.Setenv("MOLE_HOME", t.TempDir())

	h := NewHandler("")
	rec := httptest.NewRecorder()
	h.handleRuntimeLogs(rec, httptest.NewRequest(http.MethodGet, "/api/gateway/logs", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"logs":[]`) {
		t.Fatalf("body = %s, want an empty logs array", body)
	}
}

func TestRequestReloadToleratesMissingRuntimeAndHook(t *testing.T) {
	h := NewHandler("")
	h.requestReload()

	h.SetRuntime(&Runtime{Address: "127.0.0.1:18790"})
	h.requestReload()
}
