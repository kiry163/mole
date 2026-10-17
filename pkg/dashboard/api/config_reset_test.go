package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kiry163/mole/pkg/config"
)

// The console owns the WebChat channel it serves, so a factory reset must not
// leave the reload about to follow with the channel switched off: the console
// would stop serving /webchat/ws until the process is restarted.
func TestHandleResetConfig_KeepsWebChatChannelEnabled(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)

	// Startup path: the console enables the channel it serves.
	if _, err := h.EnsureWebChatChannel(); err != nil {
		t.Fatalf("EnsureWebChatChannel() error = %v", err)
	}

	reloads := 0
	h.SetRuntime(&Runtime{Reload: func() { reloads++ }})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/config/reset", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("body = %q, want status ok", body)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	bc := cfg.Channels.GetByType(config.ChannelWebChat)
	if bc == nil {
		t.Fatal("webchat channel missing after factory reset")
	}
	if !bc.Enabled {
		t.Error("expected webchat channel to stay enabled after factory reset")
	}
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("GetDecoded() error = %v", err)
	}
	if webchatCfg, ok := decoded.(*config.WebChatSettings); ok && webchatCfg.Token.String() == "" {
		t.Error("expected a non-empty webchat token after factory reset")
	}

	if reloads == 0 {
		t.Error("expected the reset to request a gateway reload")
	}
}
