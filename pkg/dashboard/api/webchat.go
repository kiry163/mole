package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/kiry163/mole/pkg/config"
)

// registerWebChatRoutes binds WebChat Channel management endpoints to the ServeMux.
func (h *Handler) registerWebChatRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/webchat/token", h.handleGetWebChatToken)
}

func decodeWebChatSettings(cfg *config.Config) (config.WebChatSettings, bool) {
	if cfg == nil {
		return config.WebChatSettings{}, false
	}

	bc := cfg.Channels.GetByType(config.ChannelWebChat)
	if bc == nil {
		return config.WebChatSettings{}, false
	}

	var webchatCfg config.WebChatSettings
	if err := bc.Decode(&webchatCfg); err != nil {
		return config.WebChatSettings{}, false
	}

	return webchatCfg, bc.Enabled
}

// webChatTokenResponse carries the raw channel token to the same-origin SPA.
type webChatTokenResponse struct {
	Token string `json:"token"`
}

// handleGetWebChatToken returns the raw WebChat token. The SPA runs on the same
// origin and passes it to the channel's WebSocket handshake as the
// "token.<value>" subprotocol.
//
//	GET /api/webchat/token
func (h *Handler) handleGetWebChatToken(w http.ResponseWriter, _ *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	webchatCfg, enabled := decodeWebChatSettings(cfg)
	if token := webchatCfg.Token.String(); enabled && token != "" {
		writeJSONResponse(w, webChatTokenResponse{Token: token})
		return
	}

	http.Error(w, "WebChat channel not configured", http.StatusServiceUnavailable)
}

// EnsureWebChatChannel enables the WebChat channel with sane defaults if it isn't
// already configured. Returns true when the config was modified.
func (h *Handler) EnsureWebChatChannel() (bool, error) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		return false, fmt.Errorf("failed to load config: %w", err)
	}

	changed := false

	bc := cfg.Channels.GetByType(config.ChannelWebChat)
	if bc == nil {
		bc = &config.Channel{Type: config.ChannelWebChat}
		cfg.Channels["webchat"] = bc
	}

	if !bc.Enabled {
		bc.Enabled = true
		changed = true
	}

	// webchat is the console's own channel and is enabled automatically, so its
	// audience has to be acknowledged automatically too: an empty allow_from
	// accepts everyone (pkg/channels only logs a warning), and the console
	// refuses to save a channel in that state.
	if len(bc.AllowFrom) == 0 {
		bc.AllowFrom = config.FlexibleStringSlice{"*"}
		changed = true
	}

	if decoded, err := bc.GetDecoded(); err == nil && decoded != nil {
		if webchatCfg, ok := decoded.(*config.WebChatSettings); ok {
			if webchatCfg.Token.String() == "" {
				webchatCfg.Token = *config.NewSecureString(generateSecureToken())
				changed = true
			}
		}
	}

	if changed {
		if err := config.SaveConfig(h.configPath, cfg); err != nil {
			return false, fmt.Errorf("failed to save config: %w", err)
		}
	}

	return changed, nil
}

// generateSecureToken creates a random 32-character hex string.
func generateSecureToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to something pseudo-random if crypto/rand fails
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
