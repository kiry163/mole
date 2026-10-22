package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kiry163/mole/pkg/config"
)

func TestHandleListTools_ReportsWebSearchEnabledWhenToolIsOn(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.Tools.Web.Provider = "tavily"
	cfg.Tools.Web.Sogou.Enabled = false
	cfg.Tools.Web.Tavily.Enabled = true
	cfg.Tools.Web.Tavily.SetAPIKeys(nil)
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tools", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp toolSupportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	for _, tool := range resp.Tools {
		if tool.Name != "web_search" {
			continue
		}
		if tool.Status != "enabled" || tool.ReasonCode != "" {
			t.Fatalf("web_search = %#v, want enabled with no reason code", tool)
		}
		return
	}

	t.Fatal("expected web_search in response")
}

func TestHandleGetWebSearchConfig(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.Tools.Web.Provider = "sogou"
	cfg.Tools.Web.Sogou.Enabled = true
	cfg.Tools.Web.Sogou.MaxResults = 6
	cfg.Tools.Web.Tavily.Enabled = true
	cfg.Tools.Web.Tavily.SetAPIKey("tavily-test-key")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tools/web-search-config", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp webSearchConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp.Provider != "sogou" {
		t.Fatalf("provider = %q, want sogou", resp.Provider)
	}
	if resp.CurrentService != "sogou" {
		t.Fatalf("current_service = %q, want sogou", resp.CurrentService)
	}
	if !resp.Settings["tavily"].APIKeySet {
		t.Fatalf("tavily api_key_set should be true: %#v", resp.Settings["tavily"])
	}
}

func TestHandleUpdateWebSearchConfig(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.Tools.Web.Tavily.SetAPIKeys([]string{"tavily-old-1", "tavily-old-2"})
	if saveErr := config.SaveConfig(configPath, cfg); saveErr != nil {
		t.Fatalf("SaveConfig() error = %v", saveErr)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/tools/web-search-config",
		bytes.NewBufferString(`{
			"provider":"tavily",
			"proxy":"http://127.0.0.1:7890",
			"settings":{
				"sogou":{"enabled":true,"max_results":4},
				"tavily":{"enabled":true,"max_results":7,"api_key":"tavily-new-key"}
			}
		}`),
	)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if updated.Tools.Web.Provider != "tavily" {
		t.Fatalf("provider = %q, want tavily", updated.Tools.Web.Provider)
	}
	if updated.Tools.Web.Proxy != "http://127.0.0.1:7890" {
		t.Fatalf("proxy = %q", updated.Tools.Web.Proxy)
	}
	if !updated.Tools.Web.Sogou.Enabled || updated.Tools.Web.Sogou.MaxResults != 4 {
		t.Fatalf("sogou config not updated: %#v", updated.Tools.Web.Sogou)
	}
	if !updated.Tools.Web.Tavily.Enabled || updated.Tools.Web.Tavily.MaxResults != 7 {
		t.Fatalf("tavily config not updated: %#v", updated.Tools.Web.Tavily)
	}
	if updated.Tools.Web.Tavily.APIKey() != "tavily-new-key" {
		t.Fatalf("tavily api key not updated")
	}
}

func TestHandleUpdateWebSearchConfig_PreservesAndReplacesMultiKeys(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.Tools.Web.Tavily.SetAPIKeys([]string{"tavily-old-1", "tavily-old-2"})
	if saveErr := config.SaveConfig(configPath, cfg); saveErr != nil {
		t.Fatalf("SaveConfig() error = %v", saveErr)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/tools/web-search-config",
		bytes.NewBufferString(`{
			"provider":"auto",
			"proxy":"",
			"settings":{
				"tavily":{"enabled":true,"max_results":7}
			}
		}`),
	)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := updated.Tools.Web.Tavily.APIKeys.Values(); len(got) != 2 ||
		got[0] != "tavily-old-1" || got[1] != "tavily-old-2" {
		t.Fatalf("tavily api keys should be preserved, got %#v", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(
		http.MethodPut,
		"/api/tools/web-search-config",
		bytes.NewBufferString(`{
			"provider":"auto",
			"proxy":"",
			"settings":{
				"tavily":{"enabled":true,"max_results":7,"api_keys":["tavily-new-1","tavily-new-2","tavily-new-1"]}
			}
		}`),
	)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	updated, err = config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := updated.Tools.Web.Tavily.APIKeys.Values(); len(got) != 2 ||
		got[0] != "tavily-new-1" || got[1] != "tavily-new-2" {
		t.Fatalf("tavily api keys should be replaced by api_keys, got %#v", got)
	}
}

func TestResolveCurrentWebSearchProvider_PrefersConfiguredProvidersInAutoMode(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Tools.Web.Provider = "auto"
	cfg.Tools.Web.Sogou.Enabled = true
	cfg.Tools.Web.Tavily.Enabled = true
	cfg.Tools.Web.Tavily.SetAPIKey("tavily-test-key")

	if got := resolveCurrentWebSearchProvider(cfg); got != "tavily" {
		t.Fatalf("resolveCurrentWebSearchProvider() = %q, want tavily", got)
	}
}

func TestResolveCurrentWebSearchProvider_FallsBackWhenExplicitProviderUnavailable(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Tools.Web.Provider = "tavily"
	cfg.Tools.Web.Tavily.Enabled = true
	cfg.Tools.Web.Sogou.Enabled = true

	if got := resolveCurrentWebSearchProvider(cfg); got != "sogou" {
		t.Fatalf("resolveCurrentWebSearchProvider() = %q, want sogou", got)
	}
}

func TestResolveCurrentWebSearchProvider_FallsBackWhenProviderIsUnknown(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Tools.Web.Provider = "totally_unknown"
	cfg.Tools.Web.Sogou.Enabled = true

	if got := resolveCurrentWebSearchProvider(cfg); got != "sogou" {
		t.Fatalf("resolveCurrentWebSearchProvider() = %q, want sogou", got)
	}
}
