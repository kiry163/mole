package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/kiry163/mole/pkg/config"
	moleTools "github.com/kiry163/mole/pkg/tools"
)

type toolCatalogEntry struct {
	Name        string
	Description string
	Category    string
	ConfigKey   string
}

type toolSupportItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	ConfigKey   string `json:"config_key"`
	Status      string `json:"status"`
	ReasonCode  string `json:"reason_code,omitempty"`
}

type toolSupportResponse struct {
	Tools []toolSupportItem `json:"tools"`
}

type toolStateRequest struct {
	Enabled bool `json:"enabled"`
}

type webSearchProviderOption struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Configured   bool   `json:"configured"`
	Current      bool   `json:"current"`
	RequiresAuth bool   `json:"requires_auth"`
}

type webSearchProviderConfig struct {
	Enabled    bool     `json:"enabled"`
	MaxResults int      `json:"max_results"`
	BaseURL    string   `json:"base_url,omitempty"`
	APIKey     string   `json:"api_key,omitempty"`
	APIKeys    []string `json:"api_keys,omitempty"`
	Model      string   `json:"model,omitempty"`
	APIKeySet  bool     `json:"api_key_set,omitempty"`
}

type webSearchConfigResponse struct {
	Provider       string                             `json:"provider"`
	CurrentService string                             `json:"current_service"`
	Proxy          string                             `json:"proxy,omitempty"`
	Providers      []webSearchProviderOption          `json:"providers"`
	Settings       map[string]webSearchProviderConfig `json:"settings"`
}

type webSearchConfigRequest struct {
	Provider string                             `json:"provider"`
	Proxy    string                             `json:"proxy"`
	Settings map[string]webSearchProviderConfig `json:"settings"`
}

var toolCatalog = []toolCatalogEntry{
	{
		Name:        "read_file",
		Description: "Read file content from the workspace or explicitly allowed paths.",
		Category:    "filesystem",
		ConfigKey:   "read_file",
	},
	{
		Name:        "write_file",
		Description: "Create or overwrite files within the writable workspace scope.",
		Category:    "filesystem",
		ConfigKey:   "write_file",
	},
	{
		Name:        "list_dir",
		Description: "Inspect directories and enumerate files available to the agent.",
		Category:    "filesystem",
		ConfigKey:   "list_dir",
	},
	{
		Name:        "edit_file",
		Description: "Apply targeted edits to existing files without rewriting everything.",
		Category:    "filesystem",
		ConfigKey:   "edit_file",
	},
	{
		Name:        "append_file",
		Description: "Append content to the end of an existing file.",
		Category:    "filesystem",
		ConfigKey:   "append_file",
	},
	{
		Name:        "exec",
		Description: "Run shell commands inside the configured workspace sandbox.",
		Category:    "filesystem",
		ConfigKey:   "exec",
	},
	{
		Name:        "web_search",
		Description: "Search the web using the configured providers.",
		Category:    "web",
		ConfigKey:   "web",
	},
	{
		Name:        "web_fetch",
		Description: "Fetch and summarize the contents of a webpage.",
		Category:    "web",
		ConfigKey:   "web_fetch",
	},
	{
		Name:        "message",
		Description: "Send a follow-up message back to the active user or chat.",
		Category:    "communication",
		ConfigKey:   "message",
	},
	{
		Name:        "send_file",
		Description: "Send an outbound file or media attachment to the active chat.",
		Category:    "communication",
		ConfigKey:   "send_file",
	},
	{
		Name:        "tool_search_tool_regex",
		Description: "Discover hidden MCP tools by regex search when tool discovery is enabled.",
		Category:    "discovery",
		ConfigKey:   "mcp.discovery.use_regex",
	},
	{
		Name:        "tool_search_tool_bm25",
		Description: "Discover hidden MCP tools by semantic ranking when tool discovery is enabled.",
		Category:    "discovery",
		ConfigKey:   "mcp.discovery.use_bm25",
	},
}

func (h *Handler) registerToolRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tools", h.handleListTools)
	mux.HandleFunc("PUT /api/tools/{name}/state", h.handleUpdateToolState)
	mux.HandleFunc("GET /api/tools/web-search-config", h.handleGetWebSearchConfig)
	mux.HandleFunc("PUT /api/tools/web-search-config", h.handleUpdateWebSearchConfig)
}

func (h *Handler) handleListTools(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toolSupportResponse{
		Tools: buildToolSupport(cfg),
	})
}

func (h *Handler) handleUpdateToolState(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	var req toolStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	if err := applyToolState(cfg, r.PathValue("name"), req.Enabled); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func buildToolSupport(cfg *config.Config) []toolSupportItem {
	items := make([]toolSupportItem, 0, len(toolCatalog))
	for _, entry := range toolCatalog {
		status := "disabled"
		reasonCode := ""

		switch entry.Name {
		case "tool_search_tool_regex":
			status, reasonCode = resolveDiscoveryToolSupport(cfg, cfg.Tools.MCP.Discovery.UseRegex)
		case "tool_search_tool_bm25":
			status, reasonCode = resolveDiscoveryToolSupport(cfg, cfg.Tools.MCP.Discovery.UseBM25)
		case "web_search":
			status, reasonCode = resolveWebSearchToolSupport(cfg)
		default:
			if cfg.Tools.IsToolEnabled(entry.ConfigKey) {
				status = "enabled"
			}
		}

		items = append(items, toolSupportItem{
			Name:        entry.Name,
			Description: entry.Description,
			Category:    entry.Category,
			ConfigKey:   entry.ConfigKey,
			Status:      status,
			ReasonCode:  reasonCode,
		})
	}
	return items
}

func resolveDiscoveryToolSupport(cfg *config.Config, methodEnabled bool) (string, string) {
	if !cfg.Tools.IsToolEnabled("mcp") {
		return "disabled", ""
	}
	if !cfg.Tools.MCP.Discovery.Enabled {
		return "blocked", "requires_mcp_discovery"
	}
	if !methodEnabled {
		return "disabled", ""
	}
	return "enabled", ""
}

func resolveWebSearchToolSupport(cfg *config.Config) (string, string) {
	if !cfg.Tools.IsToolEnabled("web") {
		return "disabled", ""
	}
	return "enabled", ""
}

func applyToolState(cfg *config.Config, toolName string, enabled bool) error {
	switch toolName {
	case "read_file":
		cfg.Tools.ReadFile.Enabled = enabled
	case "write_file":
		cfg.Tools.WriteFile.Enabled = enabled
	case "list_dir":
		cfg.Tools.ListDir.Enabled = enabled
	case "edit_file":
		cfg.Tools.EditFile.Enabled = enabled
	case "append_file":
		cfg.Tools.AppendFile.Enabled = enabled
	case "exec":
		cfg.Tools.Exec.Enabled = enabled
	case "web_search":
		cfg.Tools.Web.Enabled = enabled
	case "web_fetch":
		cfg.Tools.WebFetch.Enabled = enabled
	case "message":
		cfg.Tools.Message.Enabled = enabled
	case "send_file":
		cfg.Tools.SendFile.Enabled = enabled
	case "tool_search_tool_regex":
		cfg.Tools.MCP.Discovery.UseRegex = enabled
		if enabled {
			cfg.Tools.MCP.Enabled = true
			cfg.Tools.MCP.Discovery.Enabled = true
		}
	case "tool_search_tool_bm25":
		cfg.Tools.MCP.Discovery.UseBM25 = enabled
		if enabled {
			cfg.Tools.MCP.Enabled = true
			cfg.Tools.MCP.Discovery.Enabled = true
		}
	default:
		return fmt.Errorf("tool %q cannot be updated", toolName)
	}
	return nil
}

func (h *Handler) handleGetWebSearchConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(buildWebSearchConfigResponse(cfg)); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (h *Handler) handleUpdateWebSearchConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	var req webSearchConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	provider := normalizeWebSearchProvider(req.Provider)
	if provider == "" {
		http.Error(w, "invalid web search provider", http.StatusBadRequest)
		return
	}

	cfg.Tools.Web.Provider = provider
	cfg.Tools.Web.Proxy = strings.TrimSpace(req.Proxy)

	if settings, ok := req.Settings["sogou"]; ok {
		cfg.Tools.Web.Sogou.Enabled = settings.Enabled
		cfg.Tools.Web.Sogou.MaxResults = settings.MaxResults
	}
	if settings, ok := req.Settings["tavily"]; ok {
		cfg.Tools.Web.Tavily.Enabled = settings.Enabled
		cfg.Tools.Web.Tavily.MaxResults = settings.MaxResults
		cfg.Tools.Web.Tavily.BaseURL = strings.TrimSpace(settings.BaseURL)
		if keys, ok := normalizeWebSearchAPIKeys(settings.APIKeys, settings.APIKey); ok {
			cfg.Tools.Web.Tavily.SetAPIKeys(keys)
		}
	}

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(buildWebSearchConfigResponse(cfg)); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func normalizeWebSearchProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "auto":
		return "auto"
	case "sogou",
		"tavily":
		return strings.ToLower(strings.TrimSpace(provider))
	default:
		return ""
	}
}

func normalizeWebSearchAPIKeys(apiKeys []string, apiKey string) ([]string, bool) {
	if apiKeys != nil {
		keys := make([]string, 0, len(apiKeys))
		seen := make(map[string]struct{}, len(apiKeys))
		for _, key := range apiKeys {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			keys = append(keys, trimmed)
		}
		return keys, true
	}

	if trimmed := strings.TrimSpace(apiKey); trimmed != "" {
		return []string{trimmed}, true
	}

	return nil, false
}

func buildWebSearchConfigResponse(cfg *config.Config) webSearchConfigResponse {
	opts := moleTools.WebSearchToolOptionsFromConfig(cfg)
	current := resolveCurrentWebSearchProvider(cfg)
	settings := map[string]webSearchProviderConfig{
		"sogou": {
			Enabled:    cfg.Tools.Web.Sogou.Enabled,
			MaxResults: cfg.Tools.Web.Sogou.MaxResults,
		},
		"tavily": {
			Enabled:    cfg.Tools.Web.Tavily.Enabled,
			MaxResults: cfg.Tools.Web.Tavily.MaxResults,
			BaseURL:    cfg.Tools.Web.Tavily.BaseURL,
			APIKeySet:  len(cfg.Tools.Web.Tavily.APIKeys.Values()) > 0,
		},
	}

	providers := []webSearchProviderOption{
		{
			ID:         "auto",
			Label:      "Auto",
			Configured: current != "",
			Current: cfg.Tools.Web.Provider == "" ||
				cfg.Tools.Web.Provider == "auto",
		},
		{
			ID:         "sogou",
			Label:      "Sogou",
			Configured: moleTools.WebSearchProviderReady(opts, "sogou"),
			Current:    current == "sogou",
		},
		{
			ID:           "tavily",
			Label:        "Tavily",
			Configured:   moleTools.WebSearchProviderReady(opts, "tavily"),
			Current:      current == "tavily",
			RequiresAuth: true,
		},
	}

	provider := cfg.Tools.Web.Provider
	if provider == "" {
		provider = "auto"
	}

	return webSearchConfigResponse{
		Provider:       provider,
		CurrentService: current,
		Proxy:          cfg.Tools.Web.Proxy,
		Providers:      providers,
		Settings:       settings,
	}
}

func resolveCurrentWebSearchProvider(cfg *config.Config) string {
	if cfg == nil || !cfg.Tools.IsToolEnabled("web") {
		return ""
	}
	selected, err := moleTools.ResolveWebSearchProviderName(moleTools.WebSearchToolOptionsFromConfig(cfg), "")
	if err != nil {
		return ""
	}
	return selected
}
