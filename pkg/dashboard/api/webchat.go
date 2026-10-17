package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kiry163/mole/pkg/config"
)

// registerWebChatRoutes binds WebChat Channel management endpoints to the ServeMux.
func (h *Handler) registerWebChatRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/webchat/info", h.handleGetWebChatInfo)
	mux.HandleFunc("GET /api/webchat/token", h.handleGetWebChatToken)
	mux.HandleFunc("POST /api/webchat/token", h.handleRegenWebChatToken)
	mux.HandleFunc("POST /api/webchat/setup", h.handleWebChatSetup)
}

// webChatWSURL returns the same-origin WebSocket URL of the WebChat Channel.
//
// The channel serves /webchat/ws on the same listener as the console, so the URL
// follows the host the request arrived on - including the host a reverse proxy or
// tunnel advertises - instead of the gateway bind address.
func webChatWSURL(r *http.Request) string {
	host := forwardedHostFirst(r)
	if host == "" {
		host = r.Host
	}

	return requestWSScheme(r) + "://" + host + "/webchat/ws"
}

// requestWSScheme returns ws or wss for the connection the client used.
func requestWSScheme(r *http.Request) string {
	if forwarded := forwardedProtoFirst(r); forwarded != "" {
		if strings.EqualFold(forwarded, "https") || strings.EqualFold(forwarded, "wss") {
			return "wss"
		}
		if strings.EqualFold(forwarded, "http") || strings.EqualFold(forwarded, "ws") {
			return "ws"
		}
	}

	if r.TLS != nil {
		return "wss"
	}

	return "ws"
}

// forwardedProtoFirst returns the first client-facing protocol from
// X-Forwarded-Proto, or from the RFC 7239 Forwarded header, lowercased.
func forwardedProtoFirst(r *http.Request) string {
	raw := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if raw == "" {
		raw = forwardedRFC7239Param(r, "proto")
	}
	if raw == "" {
		return ""
	}
	if i := strings.IndexByte(raw, ','); i >= 0 {
		raw = strings.TrimSpace(raw[:i])
	}

	return strings.ToLower(raw)
}

// forwardedHostFirst returns the client-visible host from reverse-proxy / tunnel
// headers (e.g. VS Code port forwarding, nginx), or the RFC 7239 Forwarded host.
// Empty when the request carries neither.
func forwardedHostFirst(r *http.Request) string {
	raw := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if raw == "" {
		raw = forwardedRFC7239Param(r, "host")
	}
	if raw == "" {
		return ""
	}
	if i := strings.IndexByte(raw, ','); i >= 0 {
		raw = strings.TrimSpace(raw[:i])
	}

	return raw
}

// forwardedRFC7239Param parses key= from the first element of the RFC 7239
// Forwarded header.
func forwardedRFC7239Param(r *http.Request, key string) string {
	value := strings.TrimSpace(r.Header.Get("Forwarded"))
	if value == "" {
		return ""
	}

	first := strings.TrimSpace(strings.Split(value, ",")[0])
	for _, part := range strings.Split(first, ";") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(strings.ToLower(part), key+"=") {
			continue
		}

		param := strings.TrimSpace(part[strings.IndexByte(part, '=')+1:])
		if len(param) >= 2 && param[0] == '"' && param[len(param)-1] == '"' {
			param = param[1 : len(param)-1]
		}
		return param
	}

	return ""
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

func (h *Handler) writeWebChatInfoResponse(
	w http.ResponseWriter,
	r *http.Request,
	cfg *config.Config,
	changed *bool,
) {
	webchatCfg, enabled := decodeWebChatSettings(cfg)

	resp := map[string]any{
		"ws_url":  webChatWSURL(r),
		"enabled": enabled,
	}
	if changed != nil {
		resp["changed"] = *changed
	}
	if webchatCfg.Token.String() != "" {
		resp["configured"] = true
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleGetWebChatInfo returns non-secret WebChat connection info for the console UI.
//
//	GET /api/webchat/info
func (h *Handler) handleGetWebChatInfo(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	h.writeWebChatInfoResponse(w, r, cfg, nil)
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

// handleRegenWebChatToken rotates the raw WebChat WebSocket token and returns
// non-secret connection info for the console UI.
//
//	POST /api/webchat/token
func (h *Handler) handleRegenWebChatToken(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	token := generateSecureToken()
	if bc := cfg.Channels.GetByType(config.ChannelWebChat); bc != nil {
		decoded, err := bc.GetDecoded()
		if err == nil && decoded != nil {
			if settings, ok := decoded.(*config.WebChatSettings); ok {
				settings.Token = *config.NewSecureString(token)
			}
		}
	}

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	// The running channel compares handshakes against the settings it loaded at
	// startup, so ask the host for a reload to make the rotated token effective.
	h.requestReload()

	h.writeWebChatInfoResponse(w, r, cfg, nil)
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

// handleWebChatSetup automatically configures everything needed for the WebChat Channel to work.
//
//	POST /api/webchat/setup
func (h *Handler) handleWebChatSetup(w http.ResponseWriter, r *http.Request) {
	changed, err := h.EnsureWebChatChannel()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Reload config (EnsureWebChatChannel may have modified it).
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	if changed {
		// A channel the console just configured is not running yet: ask the host
		// for a reload so it starts with the new settings.
		h.requestReload()
	}

	h.writeWebChatInfoResponse(w, r, cfg, &changed)
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
