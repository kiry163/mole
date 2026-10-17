package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kiry163/mole/pkg/config"
)

func TestEnsureWebChatChannel_FreshConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	changed, err := h.EnsureWebChatChannel()
	if err != nil {
		t.Fatalf("EnsureWebChatChannel() error = %v", err)
	}
	if !changed {
		t.Fatal("EnsureWebChatChannel() should report changed on a fresh config")
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	bc := cfg.Channels["webchat"]
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	webchatCfg := decoded.(*config.WebChatSettings)
	if !bc.Enabled {
		t.Error("expected WebChat to be enabled after setup")
	}
	if webchatCfg.Token.String() == "" {
		t.Error("expected a non-empty token after setup")
	}
}

func TestEnsureWebChatChannel_DoesNotEnableTokenQuery(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	if _, err := h.EnsureWebChatChannel(); err != nil {
		t.Fatalf("EnsureWebChatChannel() error = %v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	bc := cfg.Channels["webchat"]
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	webchatCfg := decoded.(*config.WebChatSettings)
	if webchatCfg.AllowTokenQuery {
		t.Error("setup must not enable allow_token_query by default")
	}
}

func TestEnsureWebChatChannel_LeavesAllowOriginsEmptyByDefault(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	if _, err := h.EnsureWebChatChannel(); err != nil {
		t.Fatalf("EnsureWebChatChannel() error = %v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	bc := cfg.Channels["webchat"]
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	webchatCfg := decoded.(*config.WebChatSettings)
	if len(webchatCfg.AllowOrigins) != 0 {
		t.Errorf("allow_origins = %v, want empty", webchatCfg.AllowOrigins)
	}
}

func TestEnsureWebChatChannel_NoOriginConfigurationRequired(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	if _, err := h.EnsureWebChatChannel(); err != nil {
		t.Fatalf("EnsureWebChatChannel() error = %v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	bc := cfg.Channels["webchat"]
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	webchatCfg := decoded.(*config.WebChatSettings)
	if len(webchatCfg.AllowOrigins) != 0 {
		t.Errorf("allow_origins = %v, want empty", webchatCfg.AllowOrigins)
	}
}

func TestEnsureWebChatChannel_PreservesUserSettings(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")

	// Pre-configure with custom user settings
	cfg := config.DefaultConfig()
	bc := cfg.Channels["webchat"]
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	webchatCfg := decoded.(*config.WebChatSettings)
	bc.Enabled = true
	bc.AllowFrom = config.FlexibleStringSlice{"*"}
	webchatCfg.SetToken("user-custom-token")
	webchatCfg.AllowTokenQuery = true
	webchatCfg.AllowOrigins = []string{"https://myapp.example.com"}
	if err = config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)

	changed, err := h.EnsureWebChatChannel()
	if err != nil {
		t.Fatalf("EnsureWebChatChannel() error = %v", err)
	}
	if changed {
		t.Error("EnsureWebChatChannel() should not change a fully configured config")
	}

	cfg, err = config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	bc = cfg.Channels["webchat"]
	decoded, err = bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	webchatCfg = decoded.(*config.WebChatSettings)
	if webchatCfg.Token.String() != "user-custom-token" {
		t.Errorf("token = %q, want %q", webchatCfg.Token.String(), "user-custom-token")
	}
	if !webchatCfg.AllowTokenQuery {
		t.Error("user's allow_token_query=true must be preserved")
	}
	if len(webchatCfg.AllowOrigins) != 1 || webchatCfg.AllowOrigins[0] != "https://myapp.example.com" {
		t.Errorf("allow_origins = %v, want [https://myapp.example.com]", webchatCfg.AllowOrigins)
	}
}

func TestEnsureWebChatChannel_ExistingConfigWithoutEnvFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")

	cfg := config.DefaultConfig()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err = os.WriteFile(configPath, raw, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	h := NewHandler(configPath)

	changed, err := h.EnsureWebChatChannel()
	if err != nil {
		t.Fatalf("EnsureWebChatChannel() error = %v", err)
	}
	if !changed {
		t.Fatal("EnsureWebChatChannel() should report changed when webchat is missing")
	}

	cfg, err = config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	bc := cfg.Channels["webchat"]
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	webchatCfg := decoded.(*config.WebChatSettings)
	if !bc.Enabled {
		t.Error("expected WebChat to be enabled after setup")
	}
	if webchatCfg.Token.String() == "" {
		t.Error("expected a non-empty token after setup")
	}
	rawEnv, err := os.ReadFile(config.EnvFilePath(configPath))
	if err != nil {
		t.Fatalf("expected .env to be created: %v", err)
	}
	if !strings.Contains(string(rawEnv), "WEBCHAT_TOKEN="+webchatCfg.Token.String()) {
		t.Fatalf(".env should hold the webchat token, got:\n%s", rawEnv)
	}
}

func TestEnsureWebChatChannel_ConfiguresWebChatWithoutGateway(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")

	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.ModelName = ""
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	if _, err := h.EnsureWebChatChannel(); err != nil {
		t.Fatalf("EnsureWebChatChannel() error = %v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	bc := cfg.Channels["webchat"]
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	webchatCfg := decoded.(*config.WebChatSettings)
	if !bc.Enabled {
		t.Error("expected WebChat to be enabled after startup setup")
	}
	if webchatCfg.Token.String() == "" {
		t.Error("expected a non-empty token after startup setup")
	}
}

func TestEnsureWebChatChannel_Idempotent(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	// First call sets things up
	if _, err := h.EnsureWebChatChannel(); err != nil {
		t.Fatalf("first EnsureWebChatChannel() error = %v", err)
	}

	cfg1, _ := config.LoadConfig(configPath)
	bc := cfg1.Channels["webchat"]
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	webchatCfg := decoded.(*config.WebChatSettings)
	token1 := webchatCfg.Token.String()

	// Second call should be a no-op
	changed, err := h.EnsureWebChatChannel()
	if err != nil {
		t.Fatalf("second EnsureWebChatChannel() error = %v", err)
	}
	if changed {
		t.Error("second EnsureWebChatChannel() should not report changed")
	}

	cfg2, _ := config.LoadConfig(configPath)
	bc = cfg2.Channels["webchat"]
	decoded, err = bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	webchatCfg = decoded.(*config.WebChatSettings)
	if webchatCfg.Token.String() != token1 {
		t.Error("token should not change on subsequent calls")
	}
}

func TestHandleWebChatSetup_DoesNotPersistRequestOrigin(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	req := httptest.NewRequest("POST", "/api/webchat/setup", nil)
	req.Header.Set("Origin", "http://10.0.0.5:3000")
	rec := httptest.NewRecorder()

	h.handleWebChatSetup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	bc := cfg.Channels["webchat"]
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	webchatCfg := decoded.(*config.WebChatSettings)
	if len(webchatCfg.AllowOrigins) != 0 {
		t.Errorf("allow_origins = %v, want empty", webchatCfg.AllowOrigins)
	}
}

func TestHandleWebChatSetup_Response(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	req := httptest.NewRequest("POST", "/api/webchat/setup", nil)
	rec := httptest.NewRecorder()

	h.handleWebChatSetup(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, ok := resp["token"]; ok {
		t.Error("response must not expose the raw webchat token")
	}
	if resp["ws_url"] == nil || resp["ws_url"] == "" {
		t.Error("response should contain ws_url")
	}
	if resp["enabled"] != true {
		t.Error("response should have enabled=true")
	}
	if resp["changed"] != true {
		t.Error("response should have changed=true on first setup")
	}
	if resp["configured"] != true {
		t.Error("response should have configured=true")
	}
}

func TestHandleGetWebChatInfo_OmitsToken(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	if _, err := h.EnsureWebChatChannel(); err != nil {
		t.Fatalf("EnsureWebChatChannel() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://console.local/api/webchat/info", nil)
	rec := httptest.NewRecorder()

	h.handleGetWebChatInfo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, ok := resp["token"]; ok {
		t.Fatal("info response must not expose the raw webchat token")
	}
	if resp["enabled"] != true {
		t.Fatalf("enabled = %#v, want true", resp["enabled"])
	}
	if resp["configured"] != true {
		t.Fatalf("configured = %#v, want true", resp["configured"])
	}
	if resp["ws_url"] == nil || resp["ws_url"] == "" {
		t.Fatal("response should contain ws_url")
	}
}

func TestHandleRegenWebChatTokenPersistsTokenAndRequestsReload(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	if _, err := h.EnsureWebChatChannel(); err != nil {
		t.Fatalf("EnsureWebChatChannel() error = %v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	bc := cfg.Channels["webchat"]
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	previousToken := decoded.(*config.WebChatSettings).Token.String()

	reloads := 0
	h.SetRuntime(&Runtime{Address: "127.0.0.1:18790", Reload: func() { reloads++ }})

	req := httptest.NewRequest(http.MethodPost, "http://localhost/api/webchat/token", nil)
	rec := httptest.NewRecorder()
	h.handleRegenWebChatToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	cfg, err = config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	decoded, err = cfg.Channels["webchat"].GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	token := decoded.(*config.WebChatSettings).Token.String()
	if token == "" {
		t.Fatal("expected regenerated webchat token to be persisted")
	}
	if token == previousToken {
		t.Fatal("expected regenerated webchat token to differ from the previous one")
	}
	if reloads != 1 {
		t.Fatalf("reload requests = %d, want 1: the running channel must pick the new token up", reloads)
	}
}

func TestHandleGetWebChatTokenReturnsRawChannelToken(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	if _, err := h.EnsureWebChatChannel(); err != nil {
		t.Fatalf("EnsureWebChatChannel() error = %v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	decoded, err := cfg.Channels["webchat"].GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	want := decoded.(*config.WebChatSettings).Token.String()

	rec := httptest.NewRecorder()
	h.handleGetWebChatToken(rec, httptest.NewRequest(http.MethodGet, "/api/webchat/token", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp webChatTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Token != want {
		t.Fatalf("token = %q, want %q", resp.Token, want)
	}
}

func TestHandleGetWebChatTokenReportsUnconfiguredChannel(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Channels["webchat"].Enabled = false
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)

	rec := httptest.NewRecorder()
	h.handleGetWebChatToken(rec, httptest.NewRequest(http.MethodGet, "/api/webchat/token", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestWriteWebChatInfoResponseUsesRequestHost(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		headers map[string]string
		want    string
	}{
		{
			name:   "direct host",
			target: "http://192.168.1.9:18790/api/webchat/info",
			want:   "ws://192.168.1.9:18790/webchat/ws",
		},
		{
			name:   "secure host",
			target: "https://console.example.com/api/webchat/info",
			want:   "wss://console.example.com/webchat/ws",
		},
		{
			name:   "forwarded host and protocol",
			target: "http://127.0.0.1:18790/api/webchat/info",
			headers: map[string]string{
				"X-Forwarded-Host":  "tunnel.example.com",
				"X-Forwarded-Proto": "https",
			},
			want: "wss://tunnel.example.com/webchat/ws",
		},
		{
			name:   "rfc7239 forwarded header",
			target: "http://127.0.0.1:18790/api/webchat/info",
			headers: map[string]string{
				"Forwarded": `for=192.0.2.60;host=code.example.com;proto=https`,
			},
			want: "wss://code.example.com/webchat/ws",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.json")
			cfg := config.DefaultConfig()
			h := NewHandler(configPath)

			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			rec := httptest.NewRecorder()
			h.writeWebChatInfoResponse(rec, req, cfg, nil)

			var resp map[string]any
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if resp["ws_url"] != tt.want {
				t.Fatalf("ws_url = %#v, want %q", resp["ws_url"], tt.want)
			}
		})
	}
}

func TestWebChatRoutesAreRegistered(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// The console configures the channel when it starts (pkg/dashboard.New does
	// the same), so the token endpoints answer instead of reporting a missing
	// channel.
	if _, err := h.EnsureWebChatChannel(); err != nil {
		t.Fatalf("EnsureWebChatChannel() error = %v", err)
	}

	apiRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/webchat/info"},
		{http.MethodGet, "/api/webchat/token"},
		{http.MethodPost, "/api/webchat/token"},
		{http.MethodPost, "/api/webchat/setup"},
	}
	for _, route := range apiRoutes {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(route.method, route.path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s status = %d, want %d, body=%s",
				route.method, route.path, rec.Code, http.StatusOK, rec.Body.String())
		}
	}

	// The channel serves /webchat/ws and /webchat/media/{id} on its own routes;
	// the console must not register them, or a host mounting the console mux
	// would shadow the channel.
	for _, path := range []string{"/webchat/ws", "/webchat/media/attachment-1"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want %d: the console must not serve channel paths",
				path, rec.Code, http.StatusNotFound)
		}
	}
}

func TestEnsureWebChatChannelAcknowledgesAllowAll(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	if _, err := h.EnsureWebChatChannel(); err != nil {
		t.Fatalf("EnsureWebChatChannel() error = %v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	bc := cfg.Channels.GetByType(config.ChannelWebChat)
	if bc == nil {
		t.Fatal("webchat channel missing after EnsureWebChatChannel()")
	}
	if len(bc.AllowFrom) != 1 || bc.AllowFrom[0] != "*" {
		t.Fatalf("webchat allow_from = %#v, want [\"*\"]: enabling a channel without a whitelist "+
			"would silently accept everyone, and the console rejects saving that state", bc.AllowFrom)
	}
}
