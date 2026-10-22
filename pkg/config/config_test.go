package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig_MCPMaxInlineTextChars(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Tools.MCP.GetMaxInlineTextChars() != DefaultMCPMaxInlineTextChars {
		t.Fatalf(
			"DefaultConfig().Tools.MCP.GetMaxInlineTextChars() = %d, want %d",
			cfg.Tools.MCP.GetMaxInlineTextChars(),
			DefaultMCPMaxInlineTextChars,
		)
	}
}

func TestLoadConfig_MCPMaxInlineTextChars(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	raw := `{
		"version": 4,
		"tools": {
			"mcp": {
				"enabled": true,
				"max_inline_text_chars": 2048
			}
		}
	}`
	if err := os.WriteFile(configPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("WriteFile(configPath): %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if got := cfg.Tools.MCP.GetMaxInlineTextChars(); got != 2048 {
		t.Fatalf("cfg.Tools.MCP.GetMaxInlineTextChars() = %d, want 2048", got)
	}
}

func TestDefaultConfig_WorkspacePath(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Agents.Defaults.Workspace == "" {
		t.Error("Workspace should not be empty")
	}
}

// TestDefaultConfig_MaxTokens verifies max tokens has default value
func TestDefaultConfig_MaxTokens(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Agents.Defaults.MaxTokens == 0 {
		t.Error("MaxTokens should not be zero")
	}
}

// TestDefaultConfig_MaxToolIterations verifies max tool iterations has default value
func TestDefaultConfig_MaxToolIterations(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Agents.Defaults.MaxToolIterations == 0 {
		t.Error("MaxToolIterations should not be zero")
	}
}

// TestDefaultConfig_Temperature verifies temperature has default value
func TestDefaultConfig_Temperature(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Agents.Defaults.Temperature != nil {
		t.Error("Temperature should be nil when not provided")
	}
}

// TestDefaultConfig_Gateway verifies gateway defaults
func TestDefaultConfig_Gateway(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Gateway.Host != "localhost" {
		t.Error("Gateway host should have default value")
	}
	if cfg.Gateway.Port == 0 {
		t.Error("Gateway port should have default value")
	}
	if cfg.Gateway.HotReload {
		t.Error("Gateway hot reload should be disabled by default")
	}
}

// TestDefaultConfig_Channels verifies channels are disabled by default
func TestDefaultConfig_Channels(t *testing.T) {
	cfg := DefaultConfig()

	for name, bc := range cfg.Channels {
		if bc.Enabled {
			t.Errorf("Channel %q should be disabled by default", name)
		}
	}
}

func TestDefaultConfig_ChannelStreamingDisabled(t *testing.T) {
	cfg := DefaultConfig()

	webchat := cfg.Channels.Get(ChannelWebChat)
	if webchat == nil {
		t.Fatal("DefaultConfig() missing webchat channel")
	}
	webchatDecoded, err := webchat.GetDecoded()
	if err != nil {
		t.Fatalf("webchat GetDecoded() error = %v", err)
	}
	webchatSettings, ok := webchatDecoded.(*WebChatSettings)
	if !ok {
		t.Fatalf("webchat settings type = %T, want *WebChatSettings", webchatDecoded)
	}
	if !webchatSettings.Streaming.Enabled {
		t.Fatal("DefaultConfig().webchat.settings.streaming.enabled should be true")
	}
}

func TestValidateSingletonChannels_RejectsMultipleInstances(t *testing.T) {
	channels := ChannelsConfig{
		"webchat1": &Channel{Enabled: true, Type: ChannelWebChat},
		"webchat2": &Channel{Enabled: true, Type: ChannelWebChat},
	}
	err := validateSingletonChannels(channels)
	if err == nil {
		t.Fatal("expected error for multiple webchat channels, got nil")
	}
	if !strings.Contains(err.Error(), "singleton") {
		t.Fatalf("expected singleton error, got: %v", err)
	}
}

func TestValidateSingletonChannels_AllowsSingleInstance(t *testing.T) {
	channels := ChannelsConfig{
		"webchat1": &Channel{Enabled: true, Type: ChannelWebChat},
	}
	err := validateSingletonChannels(channels)
	if err != nil {
		t.Fatalf("expected no error for single webchat channel, got: %v", err)
	}
}

func TestValidateSingletonChannels_IgnoresDisabledInstances(t *testing.T) {
	channels := ChannelsConfig{
		"webchat1": &Channel{Enabled: true, Type: ChannelWebChat},
		"webchat2": &Channel{Enabled: false, Type: ChannelWebChat},
	}
	err := validateSingletonChannels(channels)
	if err != nil {
		t.Fatalf("expected no error when only one webchat channel is enabled, got: %v", err)
	}
}

// TestDefaultConfig_WebTools verifies web tools config
func TestDefaultConfig_WebTools(t *testing.T) {
	cfg := DefaultConfig()

	// Verify web tools defaults
	if cfg.Tools.Web.Tavily.MaxResults != 5 {
		t.Error("Expected Tavily MaxResults 5, got ", cfg.Tools.Web.Tavily.MaxResults)
	}
	if len(cfg.Tools.Web.Tavily.APIKeys) != 0 {
		t.Error("Tavily API key should be empty by default")
	}
	if !cfg.Tools.Web.Sogou.Enabled {
		t.Error("Expected Sogou to be enabled by default")
	}
}

func TestSaveConfig_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not enforced on Windows")
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	cfg := DefaultConfig()
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("config file has permission %04o, want 0600", perm)
	}
}

func TestSaveConfig_IncludesEmptyLegacyModelField(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	cfg := DefaultConfig()
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if !strings.Contains(string(data), `"model_name": ""`) {
		t.Fatalf("saved config should include empty legacy model_name field, got: %s", string(data))
	}
}

func TestSaveConfig_PreservesExplicitDisabledWebChatStreaming(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	cfg := DefaultConfig()
	webchat := cfg.Channels.Get(ChannelWebChat)
	if webchat == nil {
		t.Fatal("DefaultConfig() missing webchat channel")
	}
	webchat.Settings = RawNode(`{"streaming":{"enabled":false}}`)

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !strings.Contains(string(data), `"streaming"`) || !strings.Contains(string(data), `"enabled": false`) {
		t.Fatalf("saved config should preserve explicit disabled webchat streaming, got:\n%s", string(data))
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	loadedWebChat := loaded.Channels.Get(ChannelWebChat)
	if loadedWebChat == nil {
		t.Fatal("loaded config missing webchat channel")
	}
	decoded, err := loadedWebChat.GetDecoded()
	if err != nil {
		t.Fatalf("webchat GetDecoded() error = %v", err)
	}
	settings, ok := decoded.(*WebChatSettings)
	if !ok {
		t.Fatalf("webchat settings type = %T, want *WebChatSettings", decoded)
	}
	if settings.Streaming.Enabled {
		t.Fatal("explicit disabled webchat streaming should remain disabled after SaveConfig/LoadConfig round-trip")
	}
}

// TestSaveConfig_FiltersVirtualModels verifies that SaveConfig does not write
// virtual models (generated by expandMultiKeyModels) to the config file.
func TestSaveConfig_FiltersVirtualModels(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	cfg := DefaultConfig()

	// Manually add a virtual model to ModelList (simulating what expandMultiKeyModels does)
	primaryModel := &ModelConfig{
		ModelName: "gpt-4",
		Model:     "openai/gpt-4o",
		APIKeys:   SimpleSecureStrings("key1"),
	}
	virtualModel := &ModelConfig{
		ModelName: "gpt-4__key_1",
		Model:     "openai/gpt-4o",
		APIKeys:   SimpleSecureStrings("key2"),
		isVirtual: true,
	}
	cfg.ModelList = []*ModelConfig{primaryModel, virtualModel}

	// SaveConfig should filter out virtual models
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Reload and verify
	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Should only have the primary model, not the virtual one
	if len(reloaded.ModelList) != 1 {
		t.Fatalf("expected 1 model after reload, got %d", len(reloaded.ModelList))
	}

	if reloaded.ModelList[0].ModelName != "gpt-4" {
		t.Errorf("expected model_name 'gpt-4', got %q", reloaded.ModelList[0].ModelName)
	}

	// Verify virtual model was not persisted
	for _, m := range reloaded.ModelList {
		if m.ModelName == "gpt-4__key_1" {
			t.Errorf("virtual model gpt-4__key_1 should not have been saved")
		}
	}

	// Verify the saved file does not contain the virtual model name
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if strings.Contains(string(data), "gpt-4__key_1") {
		t.Errorf("saved config should not contain virtual model name 'gpt-4__key_1'")
	}
}

// TestConfig_Complete verifies all config fields are set
func TestConfig_Complete(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Agents.Defaults.Workspace == "" {
		t.Error("Workspace should not be empty")
	}
	if cfg.Agents.Defaults.Temperature != nil {
		t.Error("Temperature should be nil when not provided")
	}
	if cfg.Agents.Defaults.MaxTokens == 0 {
		t.Error("MaxTokens should not be zero")
	}
	if cfg.Agents.Defaults.MaxToolIterations == 0 {
		t.Error("MaxToolIterations should not be zero")
	}
	if cfg.Gateway.Host != "localhost" {
		t.Error("Gateway host should have default value")
	}
	if cfg.Gateway.Port == 0 {
		t.Error("Gateway port should have default value")
	}
	if !cfg.Tools.Exec.AllowRemote {
		t.Error("Exec.AllowRemote should be true by default")
	}
}

func TestDefaultConfig_WebProviderIsAuto(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Tools.Web.Provider != "auto" {
		t.Fatalf("DefaultConfig().Tools.Web.Provider = %q, want auto", cfg.Tools.Web.Provider)
	}
}

// setExampleConfigEnv defines the variables that config/config.example.json
// references. SecureString resolves $VAR at decode time, so the example-config
// guard tests must provide them the way an instance .env would.
func setExampleConfigEnv(t *testing.T) {
	t.Helper()
	for name, value := range map[string]string{
		"DASHBOARD_TOKEN":   "example-dashboard-token",
		"DEEPSEEK_BASE_URL": "https://api.deepseek.com/v1",
		"DEEPSEEK_API_KEY":  "sk-example",
		"OPENAI_API_KEY":    "sk-openai-example",
		"TAVILY_API_KEY":    "tvly-example",
	} {
		t.Setenv(name, value)
	}
}

func TestConfigExample_WebProviderIsAuto(t *testing.T) {
	setExampleConfigEnv(t)
	data, err := os.ReadFile(filepath.Join("..", "..", "config", "config.example.json"))
	if err != nil {
		t.Fatalf("ReadFile(config.example.json) error: %v", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal(config.example.json) error: %v", err)
	}
	if cfg.Tools.Web.Provider != "auto" {
		t.Fatalf("config.example.json tools.web.provider = %q, want auto", cfg.Tools.Web.Provider)
	}
}

func TestDefaultConfig_ToolFeedbackDisabled(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Agents.Defaults.ToolFeedback.Enabled {
		t.Fatal("DefaultConfig().Agents.Defaults.ToolFeedback.Enabled should be false")
	}
	if cfg.Agents.Defaults.ToolFeedback.SeparateMessages {
		t.Fatal("DefaultConfig().Agents.Defaults.ToolFeedback.SeparateMessages should be false")
	}
}

func TestLoadConfig_ToolFeedbackDefaultsFalseWhenUnset(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(
		configPath,
		[]byte(`{"version": 4,"agents":{"defaults":{"workspace":"./workspace"}}}`),
		0o600,
	); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Agents.Defaults.ToolFeedback.Enabled {
		t.Fatal(
			"agents.defaults.tool_feedback.enabled should remain false when unset in config file",
		)
	}
	if cfg.Agents.Defaults.ToolFeedback.SeparateMessages {
		t.Fatal("agents.defaults.tool_feedback.separate_messages should remain false when unset in config file")
	}
}

func TestLoadConfig_SyntaxErrorReportsLineAndColumn(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	raw := "{\n  \"version\": 4,\n  \"tools\": {\n    \"web\": {\n      \"enabled\": true,,\n      \"format\": \"markdown\"\n    }\n  }\n}\n"
	if err := os.WriteFile(configPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("expected syntax error, got nil")
	}
	if !strings.Contains(err.Error(), "syntax error at line 5, column 23") {
		t.Fatalf("expected line/column diagnostic, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "\"enabled\": true,,") {
		t.Fatalf("expected source snippet in diagnostic, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "^") {
		t.Fatalf("expected caret marker in diagnostic, got %q", err.Error())
	}
}

func TestLoadConfig_TypeErrorReportsFieldPath(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	raw := "{\n  \"version\": 4,\n  \"tools\": {\n    \"web\": {\n      \"fetch_limit_bytes\": \"oops\"\n    }\n  }\n}\n"
	if err := os.WriteFile(configPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("expected type error, got nil")
	}
	if !strings.Contains(err.Error(), "type error at line 5, column 33") {
		t.Fatalf("expected line/column diagnostic, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "fetch_limit_bytes") {
		t.Fatalf("expected field name in diagnostic, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "\"fetch_limit_bytes\": \"oops\"") {
		t.Fatalf("expected source snippet in diagnostic, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "^") {
		t.Fatalf("expected caret marker in diagnostic, got %q", err.Error())
	}
}

func TestLoadConfig_UnknownFieldsReportsExactPaths(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	raw := "{\n  \"version\": 4,\n  \"tools\": {\n    \"weeb\": {\n      \"enabled\": true\n    },\n    \"web\": {\n      \"fatch_limit_bytes\": 123\n    }\n  }\n}\n"
	if err := os.WriteFile(configPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("expected unknown field error, got nil")
	}
	if !strings.Contains(err.Error(), "tools.weeb") || !strings.Contains(err.Error(), "tools.web.fatch_limit_bytes") {
		t.Fatalf("expected exact unknown field paths, got %q", err.Error())
	}
}

func TestDefaultConfig_ExecAllowRemoteEnabled(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Tools.Exec.AllowRemote {
		t.Fatal("DefaultConfig().Tools.Exec.AllowRemote should be true")
	}
}

func TestDefaultConfig_FilterSensitiveDataEnabled(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Tools.FilterSensitiveData {
		t.Fatal("DefaultConfig().Tools.FilterSensitiveData should be true")
	}
}

func TestDefaultConfig_FilterMinLength(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Tools.FilterMinLength != 8 {
		t.Fatalf("DefaultConfig().Tools.FilterMinLength = %d, want 8", cfg.Tools.FilterMinLength)
	}
}

func TestDefaultConfig_LoadImageEnabled(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Tools.LoadImage.Enabled {
		t.Fatal("DefaultConfig().Tools.LoadImage.Enabled should be true")
	}
	if !cfg.Tools.IsToolEnabled("load_image") {
		t.Fatal("DefaultConfig().Tools.IsToolEnabled(load_image) should be true")
	}
}

func TestLoadConfig_LoadImageCanBeDisabled(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	raw := "{\n  \"version\": 4,\n  \"tools\": {\n    \"load_image\": {\n      \"enabled\": false\n    }\n  }\n}\n"
	if err := os.WriteFile(configPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Tools.LoadImage.Enabled {
		t.Fatal("LoadConfig().Tools.LoadImage.Enabled should be false")
	}
	if cfg.Tools.IsToolEnabled("load_image") {
		t.Fatal("LoadConfig().Tools.IsToolEnabled(load_image) should be false")
	}
}

func TestDefaultConfig_MessageMediaDisabled(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Tools.Message.Enabled {
		t.Fatal("DefaultConfig().Tools.Message.Enabled should be true")
	}
	if cfg.Tools.Message.MediaEnabled {
		t.Fatal("DefaultConfig().Tools.Message.MediaEnabled should be false")
	}
}

func TestToolsConfig_GetFilterMinLength(t *testing.T) {
	tests := []struct {
		name     string
		minLen   int
		expected int
	}{
		{"zero returns default", 0, 8},
		{"negative returns default", -1, 8},
		{"positive returns value", 16, 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ToolsConfig{FilterMinLength: tt.minLen}
			if got := cfg.GetFilterMinLength(); got != tt.expected {
				t.Errorf("GetFilterMinLength() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultConfig_LogLevel(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Gateway.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want \"fatal\"", cfg.Gateway.LogLevel)
	}
}

func TestLoadConfig_ExecAllowRemoteDefaultsTrueWhenUnset(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"version": 4,"tools":{"exec":{"enable_deny_patterns":true}}}`),
		0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if !cfg.Tools.Exec.AllowRemote {
		t.Fatal("tools.exec.allow_remote should remain true when unset in config file")
	}
}

func TestLoadConfig_WebToolsProxy(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	configJSON := `{
  "version": 4,
  "agents": {"defaults":{"workspace":"./workspace","model_name":"gpt4","max_tokens":8192,"max_tool_iterations":20}},
  "model_list": [{"model_name":"gpt4","model":"openai/gpt-5.4","api_keys":["x"]}],
  "tools": {"web":{"proxy":"http://127.0.0.1:7890"}}
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.Tools.Web.Proxy != "http://127.0.0.1:7890" {
		t.Fatalf("Tools.Web.Proxy = %q, want %q", cfg.Tools.Web.Proxy, "http://127.0.0.1:7890")
	}
}

func TestDefaultConfig_SummarizationThresholds(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Agents.Defaults.SummarizeMessageThreshold != 20 {
		t.Errorf(
			"SummarizeMessageThreshold = %d, want 20",
			cfg.Agents.Defaults.SummarizeMessageThreshold,
		)
	}
	if cfg.Agents.Defaults.SummarizeTokenPercent != 75 {
		t.Errorf("SummarizeTokenPercent = %d, want 75", cfg.Agents.Defaults.SummarizeTokenPercent)
	}
}

func TestDefaultConfig_SessionDimensions(t *testing.T) {
	cfg := DefaultConfig()

	if len(cfg.Session.Dimensions) != 1 || cfg.Session.Dimensions[0] != "chat" {
		t.Errorf("Session.Dimensions = %v, want [chat]", cfg.Session.Dimensions)
	}
}

func TestSessionConfig_ApplyDmScope(t *testing.T) {
	tests := []struct {
		name       string
		dmScope    string
		dimensions []string
		want       []string
	}{
		{
			name:    "per-channel-peer",
			dmScope: "per-channel-peer",
			want:    []string{"chat", "sender"},
		},
		{
			name:    "per-channel",
			dmScope: "per-channel",
			want:    []string{"chat"},
		},
		{
			name:    "per-peer",
			dmScope: "per-peer",
			want:    []string{"sender"},
		},
		{
			name:    "global",
			dmScope: "global",
			want:    nil,
		},
		{
			name:       "explicit dimensions take precedence",
			dmScope:    "per-channel-peer",
			dimensions: []string{"sender"},
			want:       []string{"sender"},
		},
		{
			name:    "empty dm_scope is no-op",
			dmScope: "",
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SessionConfig{
				DmScope:    tt.dmScope,
				Dimensions: tt.dimensions,
			}
			s.ApplyDmScope()
			if len(s.Dimensions) != len(tt.want) {
				t.Fatalf("Dimensions = %v, want %v", s.Dimensions, tt.want)
			}
			for i, v := range tt.want {
				if s.Dimensions[i] != v {
					t.Errorf("Dimensions[%d] = %q, want %q", i, s.Dimensions[i], v)
				}
			}
		})
	}
}

func TestSessionConfig_ApplyDmScope_ClearsStaleDimensions(t *testing.T) {
	// Simulates the PATCH handler scenario: dm_scope changed but stale
	// dimensions remain from the old scope. After clearing dimensions,
	// ApplyDmScope should re-derive from the new dm_scope.
	tests := []struct {
		name    string
		dmScope string
		want    []string
	}{
		{
			name:    "per-channel-peer to per-channel",
			dmScope: "per-channel",
			want:    []string{"chat"},
		},
		{
			name:    "per-channel-peer to per-peer",
			dmScope: "per-peer",
			want:    []string{"sender"},
		},
		{
			name:    "per-channel-peer to global",
			dmScope: "global",
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SessionConfig{
				DmScope:    tt.dmScope,
				Dimensions: []string{"chat", "sender"}, // stale from per-channel-peer
			}
			// Simulate what the PATCH handler does: clear dimensions when dm_scope changes
			s.Dimensions = nil
			s.ApplyDmScope()
			if len(s.Dimensions) != len(tt.want) {
				t.Fatalf("Dimensions = %v, want %v", s.Dimensions, tt.want)
			}
			for i, v := range tt.want {
				if s.Dimensions[i] != v {
					t.Errorf("Dimensions[%d] = %q, want %q", i, s.Dimensions[i], v)
				}
			}
		})
	}
}

func TestDefaultConfig_WorkspacePath_Default(t *testing.T) {
	t.Setenv("MOLE_HOME", "")

	var fakeHome string
	if runtime.GOOS == "windows" {
		fakeHome = `C:\tmp\home`
		t.Setenv("USERPROFILE", fakeHome)
	} else {
		fakeHome = "/tmp/home"
		t.Setenv("HOME", fakeHome)
	}

	cfg := DefaultConfig()
	want := filepath.Join(fakeHome, ".mole", "workspace")

	if cfg.Agents.Defaults.Workspace != want {
		t.Errorf("Default workspace path = %q, want %q", cfg.Agents.Defaults.Workspace, want)
	}
}

func TestDefaultConfig_WorkspacePath_WithMoleHome(t *testing.T) {
	t.Setenv("MOLE_HOME", "/custom/mole/home")

	cfg := DefaultConfig()
	want := filepath.Join("/custom/mole/home", "workspace")

	if cfg.Agents.Defaults.Workspace != want {
		t.Errorf(
			"Workspace path with MOLE_HOME = %q, want %q",
			cfg.Agents.Defaults.Workspace,
			want,
		)
	}
}

// TestFlexibleStringSlice_UnmarshalText tests UnmarshalText with various comma separators
func TestFlexibleStringSlice_UnmarshalText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "English commas only",
			input:    "123,456,789",
			expected: []string{"123", "456", "789"},
		},
		{
			name:     "Chinese commas only",
			input:    "123，456，789",
			expected: []string{"123", "456", "789"},
		},
		{
			name:     "Mixed English and Chinese commas",
			input:    "123,456，789",
			expected: []string{"123", "456", "789"},
		},
		{
			name:     "Single value",
			input:    "123",
			expected: []string{"123"},
		},
		{
			name:     "Values with whitespace",
			input:    " 123 , 456 , 789 ",
			expected: []string{"123", "456", "789"},
		},
		{
			name:     "Empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "Only commas - English",
			input:    ",,",
			expected: []string{},
		},
		{
			name:     "Only commas - Chinese",
			input:    "，，",
			expected: []string{},
		},
		{
			name:     "Mixed commas with empty parts",
			input:    "123,,456，，789",
			expected: []string{"123", "456", "789"},
		},
		{
			name:     "Complex mixed values",
			input:    "user1@example.com，user2@test.com, admin@domain.org",
			expected: []string{"user1@example.com", "user2@test.com", "admin@domain.org"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f FlexibleStringSlice
			err := f.UnmarshalText([]byte(tt.input))
			if err != nil {
				t.Fatalf("UnmarshalText(%q) error = %v", tt.input, err)
			}

			if tt.expected == nil {
				if f != nil {
					t.Errorf("UnmarshalText(%q) = %v, want nil", tt.input, f)
				}
				return
			}

			if len(f) != len(tt.expected) {
				t.Errorf(
					"UnmarshalText(%q) length = %d, want %d",
					tt.input,
					len(f),
					len(tt.expected),
				)
				return
			}

			for i, v := range tt.expected {
				if f[i] != v {
					t.Errorf("UnmarshalText(%q)[%d] = %q, want %q", tt.input, i, f[i], v)
				}
			}
		})
	}
}

// TestFlexibleStringSlice_UnmarshalText_EmptySliceConsistency tests nil vs empty slice behavior
func TestFlexibleStringSlice_UnmarshalText_EmptySliceConsistency(t *testing.T) {
	t.Run("Empty string returns nil", func(t *testing.T) {
		var f FlexibleStringSlice
		err := f.UnmarshalText([]byte(""))
		if err != nil {
			t.Fatalf("UnmarshalText error = %v", err)
		}
		if f != nil {
			t.Errorf("Empty string should return nil, got %v", f)
		}
	})

	t.Run("Commas only returns empty slice", func(t *testing.T) {
		var f FlexibleStringSlice
		err := f.UnmarshalText([]byte(",,,"))
		if err != nil {
			t.Fatalf("UnmarshalText error = %v", err)
		}
		if f == nil {
			t.Error("Commas only should return empty slice, not nil")
		}
		if len(f) != 0 {
			t.Errorf("Expected empty slice, got %v", f)
		}
	})
}

func TestFlexibleStringSlice_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "null",
			input:    `null`,
			expected: nil,
		},
		{
			name:     "single string",
			input:    `"Thinking..."`,
			expected: []string{"Thinking..."},
		},
		{
			name:     "single number",
			input:    `123`,
			expected: []string{"123"},
		},
		{
			name:     "string array",
			input:    `["Thinking...", "Still working..."]`,
			expected: []string{"Thinking...", "Still working..."},
		},
		{
			name:     "mixed array",
			input:    `["123", 456]`,
			expected: []string{"123", "456"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f FlexibleStringSlice
			if err := json.Unmarshal([]byte(tt.input), &f); err != nil {
				t.Fatalf("json.Unmarshal(%s) error = %v", tt.input, err)
			}
			if tt.expected == nil {
				if f != nil {
					t.Fatalf("json.Unmarshal(%s) = %#v, want nil slice", tt.input, f)
				}
				return
			}
			if len(f) != len(tt.expected) {
				t.Fatalf("json.Unmarshal(%s) len = %d, want %d", tt.input, len(f), len(tt.expected))
			}
			for i, want := range tt.expected {
				if f[i] != want {
					t.Fatalf("json.Unmarshal(%s)[%d] = %q, want %q", tt.input, i, f[i], want)
				}
			}
		})
	}
}

// TestLoadConfig_FileRefNotSealed verifies that file:// api_key references are
// resolved from disk and preserved verbatim in config.json.
func TestLoadConfig_FileRefNotSealed(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	keyFile := filepath.Join(dir, "openai.key")
	if err := os.WriteFile(keyFile, []byte("sk-from-file"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	data := `{"version": 4,"model_list":[{"model_name":"test","model":"openai/gpt-4","api_keys":["file://openai.key"]}]}`
	if err := os.WriteFile(cfgPath, []byte(data), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := cfg.ModelList[0].APIKey(); got != "sk-from-file" {
		t.Fatalf("api_key = %q, want %q", got, "sk-from-file")
	}

	raw, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(raw), "file://openai.key") {
		t.Error("file:// reference should be preserved unchanged in the config file")
	}
	if strings.Contains(string(raw), "sk-from-file") {
		t.Error("resolved file:// value must not be written back to config.json")
	}
}

func TestConfigParsesLogLevel(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	data := `{"version": 4,"gateway":{"log_level":"debug"}}`
	if err := os.WriteFile(cfgPath, []byte(data), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Gateway.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want \"debug\"", cfg.Gateway.LogLevel)
	}
}

func TestConfigLogLevelEmpty(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	data := `{"version": 4}`
	if err := os.WriteFile(cfgPath, []byte(data), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	// When config omits log_level, the DefaultConfig value ("fatal") is preserved.
	if cfg.Gateway.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want \"fatal\"", cfg.Gateway.LogLevel)
	}
}

func TestResolveGatewayLogLevel(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	data := `{"version": 4,"gateway":{"log_level":"debug"}}`
	if err := os.WriteFile(cfgPath, []byte(data), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if got := ResolveGatewayLogLevel(cfgPath); got != "debug" {
		t.Fatalf("ResolveGatewayLogLevel() = %q, want %q", got, "debug")
	}
}

func TestResolveGatewayLogLevel_UsesConfigValue(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	data := `{"version": 4,"gateway":{"log_level":"warning"}}`
	if err := os.WriteFile(cfgPath, []byte(data), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if got := ResolveGatewayLogLevel(cfgPath); got != "warn" {
		t.Fatalf("ResolveGatewayLogLevel() = %q, want %q", got, "warn")
	}
}

func TestModelConfig_ExtraBodyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &Config{
		Version: CurrentVersion,
		ModelList: []*ModelConfig{
			{
				ModelName: "test-model",
				Model:     "openai/test",
				APIKeys:   SimpleSecureStrings("sk-test"),
				ExtraBody: map[string]any{"custom_field": "value", "num_field": 42},
			},
		},
	}

	if err := SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig error: %v", err)
	}

	loaded, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	if loaded.ModelList[0].ExtraBody == nil {
		t.Fatal("ExtraBody should not be nil after round-trip")
	}
	if got := loaded.ModelList[0].ExtraBody["custom_field"]; got != "value" {
		t.Errorf("ExtraBody[custom_field] = %v, want value", got)
	}
	if got := loaded.ModelList[0].ExtraBody["num_field"]; got != float64(42) {
		t.Errorf("ExtraBody[num_field] = %v, want 42", got)
	}
}

func TestModelConfig_CustomHeadersRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &Config{
		Version: CurrentVersion,
		ModelList: []*ModelConfig{
			{
				ModelName:     "test-model",
				Model:         "openai/test",
				APIKeys:       SimpleSecureStrings("sk-test"),
				CustomHeaders: map[string]string{"X-Source": "coding-plan", "X-Agent": "openclaw"},
			},
		},
	}

	if err := SaveConfig(cfgPath, cfg); err != nil {
		t.Fatalf("SaveConfig error: %v", err)
	}

	loaded, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	if loaded.ModelList[0].CustomHeaders == nil {
		t.Fatal("CustomHeaders should not be nil after round-trip")
	}
	if got := loaded.ModelList[0].CustomHeaders["X-Source"]; got != "coding-plan" {
		t.Errorf("CustomHeaders[X-Source] = %q, want coding-plan", got)
	}
	if got := loaded.ModelList[0].CustomHeaders["X-Agent"]; got != "openclaw" {
		t.Errorf("CustomHeaders[X-Agent] = %q, want openclaw", got)
	}
}

func TestFilterSensitiveData(t *testing.T) {
	// Test with nil security config
	cfg := &Config{}
	if got := cfg.FilterSensitiveData("hello sk-key123 world"); got != "hello sk-key123 world" {
		t.Errorf("nil security: got %q, want original", got)
	}

	// Test with empty content
	if got := cfg.FilterSensitiveData(""); got != "" {
		t.Errorf("empty content: got %q, want empty", got)
	}

	// Test short content (less than FilterMinLength=8, should skip filtering)
	cfg.ModelList = SecureModelList{
		&ModelConfig{
			ModelName: "test",
			APIKeys:   SimpleSecureStrings("sk-long-key-12345"),
		},
	}
	m, err := cfg.GetModelConfig("test")
	assert.NoError(t, err)
	m.APIKeys = SimpleSecureStrings("sk-long-key-12345")
	cfg.Tools.FilterSensitiveData = true
	cfg.Tools.FilterMinLength = 8

	// Debug: check if sensitive values are collected
	values := cfg.collectSensitiveValues()
	t.Logf("collected %d sensitive values: %v", len(values), values)

	if got := cfg.FilterSensitiveData("sk-key"); got != "sk-key" {
		t.Errorf("short content should not be filtered: got %q", got)
	}

	// Test filtering works
	content := "Your API key is sk-long-key-12345 and token abc123"
	// abc123 is not in sensitive values, only sk-long-key-12345 should be filtered
	expected := "Your API key is [FILTERED] and token abc123"
	if got := cfg.FilterSensitiveData(content); got != expected {
		t.Errorf("filtering failed: got %q, want %q", got, expected)
	}

	// Test disabled filtering
	cfg.Tools.FilterSensitiveData = false
	if got := cfg.FilterSensitiveData(content); got != content {
		t.Errorf("disabled filtering: got %q, want original %q", got, content)
	}
}

func TestFilterSensitiveData_MultipleKeys(t *testing.T) {
	cfg := &Config{
		Tools: ToolsConfig{
			FilterSensitiveData: true,
			FilterMinLength:     8,
		},
		ModelList: SecureModelList{
			&ModelConfig{
				ModelName: "model1",
				Model:     "openai/model1",
				APIKeys:   SecureStrings{NewSecureString("key-one"), NewSecureString("key-two")},
			},
			&ModelConfig{
				ModelName: "model2",
				Model:     "openai/model2",
				APIKeys:   SecureStrings{NewSecureString("key-three")},
			},
		},
	}

	content := "key-one and key-two and key-three should be filtered"
	expected := "[FILTERED] and [FILTERED] and [FILTERED] should be filtered"
	if got := cfg.FilterSensitiveData(content); got != expected {
		t.Errorf("multiple keys: got %q, want %q", got, expected)
	}
}

func TestFilterSensitiveData_AllTokenTypes(t *testing.T) {
	cfg := &Config{
		// Model API keys
		ModelList: SecureModelList{
			&ModelConfig{
				ModelName: "test-model",
				APIKeys:   SecureStrings{NewSecureString("sk-model-key-12345")},
			},
		},
		// Channel tokens
		Channels: testChannelsConfigWithTokens(),
		Tools: ToolsConfig{
			FilterSensitiveData: true,
			FilterMinLength:     8,
			// Web tool API keys
			Web: WebToolsConfig{
				Tavily: TavilyConfig{
					APIKeys: SecureStrings{NewSecureString("tavily-api-key")},
				},
			},
		},
	}

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "model_api_key",
			content: "Using model with key sk-model-key-12345",
			want:    "Using model with key [FILTERED]",
		},
		{
			name:    "qq_app_secret",
			content: "QQ app secret: qq-app-secret-abc",
			want:    "QQ app secret: [FILTERED]",
		},
		{
			name:    "tavily_api_key",
			content: "Tavily key: tavily-api-key",
			want:    "Tavily key: [FILTERED]",
		},
		{
			name:    "mixed_content",
			content: "Model key sk-model-key-12345 and QQ secret qq-app-secret-abc",
			want:    "Model key [FILTERED] and QQ secret [FILTERED]",
		},
		{
			name:    "short_key_not_filtered",
			content: "Key abc not filtered because length < 8",
			want:    "Key abc not filtered because length < 8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.FilterSensitiveData(tt.content); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MakeBackup tests
// ---------------------------------------------------------------------------

// TestMakeBackup_WithDateSuffix verifies backup files include a date suffix.
func TestMakeBackup_WithDateSuffix(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"version": 4}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := MakeBackup(configPath); err != nil {
		t.Fatalf("MakeBackup: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	var hasDatedBackup bool
	for _, e := range entries {
		if matched, _ := filepath.Match("config.json.20*.bak", e.Name()); matched {
			hasDatedBackup = true
			// Verify backup content matches original
			bakPath := filepath.Join(dir, e.Name())
			data, err := os.ReadFile(bakPath)
			if err != nil {
				t.Fatalf("ReadFile backup: %v", err)
			}
			if string(data) != `{"version": 4}` {
				t.Errorf("backup content = %q, want original content", string(data))
			}
			break
		}
	}
	if !hasDatedBackup {
		t.Error("expected backup file with date suffix pattern config.json.20*.bak")
	}
}

// TestMakeBackup_AlsoBacksEnvFile verifies that the instance .env file is also
// backed up with the same date suffix.
func TestMakeBackup_AlsoBacksEnvFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	envPath := EnvFilePath(configPath)

	os.WriteFile(configPath, []byte(`{"version": 4}`), 0o600)
	os.WriteFile(envPath, []byte("DASHBOARD_TOKEN=sk-test\n"), 0o600)

	if err := MakeBackup(configPath); err != nil {
		t.Fatalf("MakeBackup: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	configBackups := 0
	envBackups := 0
	for _, e := range entries {
		if matched, _ := filepath.Match("config.json.20*.bak", e.Name()); matched {
			configBackups++
		}
		if matched, _ := filepath.Match(".env.20*.bak", e.Name()); matched {
			envBackups++
		}
	}
	if configBackups != 1 {
		t.Errorf("expected 1 config backup, got %d", configBackups)
	}
	if envBackups != 1 {
		t.Errorf("expected 1 env backup, got %d", envBackups)
	}
}

// TestMakeBackup_NonexistentFileSkipsBackup verifies that MakeBackup returns nil
// when the config file does not exist (no error, no panic).
func TestMakeBackup_NonexistentFileSkipsBackup(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nonexistent.json")

	if err := MakeBackup(configPath); err != nil {
		t.Fatalf("MakeBackup on nonexistent file should return nil, got: %v", err)
	}
}

// TestMakeBackup_OnlyConfigNoEnv verifies backup succeeds when only
// the config file exists and no .env file.
func TestMakeBackup_OnlyConfigNoEnv(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	os.WriteFile(configPath, []byte(`{"version": 4}`), 0o600)

	if err := MakeBackup(configPath); err != nil {
		t.Fatalf("MakeBackup: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	configBackups := 0
	envBackups := 0
	for _, e := range entries {
		if matched, _ := filepath.Match("config.json.20*.bak", e.Name()); matched {
			configBackups++
		}
		if matched, _ := filepath.Match(".env.20*.bak", e.Name()); matched {
			envBackups++
		}
	}
	if configBackups != 1 {
		t.Errorf("expected 1 config backup, got %d", configBackups)
	}
	if envBackups != 0 {
		t.Errorf("expected 0 env backups when no env file exists, got %d", envBackups)
	}
}

// TestMakeBackup_SameDateSuffix verifies that config and .env backups
// share the same date suffix (they are created in the same MakeBackup call).
func TestMakeBackup_SameDateSuffix(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	envPath := EnvFilePath(configPath)

	os.WriteFile(configPath, []byte(`{"version": 4}`), 0o600)
	os.WriteFile(envPath, []byte("KEY=value\n"), 0o600)

	if err := MakeBackup(configPath); err != nil {
		t.Fatalf("MakeBackup: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	var configDate, envDate string
	for _, e := range entries {
		name := e.Name()
		// Extract date part: after the last . before .bak
		// e.g. config.json.20260330.bak → 20260330
		if strings.HasPrefix(name, "config.json.") && strings.HasSuffix(name, ".bak") {
			configDate = strings.TrimPrefix(name, "config.json.")
			configDate = strings.TrimSuffix(configDate, ".bak")
		}
		if strings.HasPrefix(name, ".env.") && strings.HasSuffix(name, ".bak") {
			envDate = strings.TrimPrefix(name, ".env.")
			envDate = strings.TrimSuffix(envDate, ".bak")
		}
	}
	if configDate == "" {
		t.Fatal("config backup file not found")
	}
	if envDate == "" {
		t.Fatal("env backup file not found")
	}
	if configDate != envDate {
		t.Errorf("config backup date = %q, env backup date = %q, should match", configDate, envDate)
	}
}

func testChannelsConfigWithTokens() ChannelsConfig {
	channels := make(ChannelsConfig)
	type chDef struct {
		name string
		cfg  any
	}
	defs := []chDef{
		{"qq", QQSettings{AppSecret: *NewSecureString("qq-app-secret-abc")}},
		{"webchat", WebChatSettings{Token: *NewSecureString("webchat-token-abc123")}},
	}
	for _, def := range defs {
		// Create Channel directly with settings to preserve SecureString values
		bc := &Channel{Type: def.name}
		bc.Decode(def.cfg)
		channels[def.name] = bc
	}
	return channels
}

// TestExampleConfigPassesStrictValidation 是配置字段删除的守卫：LoadConfig 拒绝未知键，
// 因此 config.example.json 必须始终能被严格解码器接受。
func TestExampleConfigPassesStrictValidation(t *testing.T) {
	setExampleConfigEnv(t)
	data, err := os.ReadFile(filepath.Join("..", "..", "config", "config.example.json"))
	if err != nil {
		t.Fatalf("ReadFile(config.example.json) error: %v", err)
	}
	var cfg Config
	if err := decodeJSONWithDiagnostics(data, &cfg, "config.example.json"); err != nil {
		t.Fatalf("config.example.json 被严格校验拒绝: %v", err)
	}
	if cfg.Version != CurrentVersion {
		t.Fatalf("config.example.json version = %d, want %d", cfg.Version, CurrentVersion)
	}
}

// TestLoadConfig_RejectsUnknownVersion pins the stale-config sentinel: the same
// config bytes load under the current schema version but are rejected with a
// readable error when they still carry an old version.
func TestLoadConfig_RejectsUnknownVersion(t *testing.T) {
	setExampleConfigEnv(t)
	data, err := os.ReadFile(filepath.Join("..", "..", "config", "config.example.json"))
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal example: %v", err)
	}

	dir := t.TempDir()
	write := func(version int) string {
		t.Helper()
		raw["version"] = version
		out, err := json.Marshal(raw)
		if err != nil {
			t.Fatalf("marshal version=%d: %v", version, err)
		}
		path := filepath.Join(dir, fmt.Sprintf("config-v%d.json", version))
		if err := os.WriteFile(path, out, 0o600); err != nil {
			t.Fatalf("write version=%d: %v", version, err)
		}
		return path
	}

	if _, err := LoadConfig(write(CurrentVersion)); err != nil {
		t.Fatalf("current version should load: %v", err)
	}

	_, err = LoadConfig(write(CurrentVersion - 1))
	if err == nil {
		t.Fatal("old version should be rejected")
	}
	if !strings.Contains(err.Error(), "unsupported config version") {
		t.Fatalf("error should name the version, got: %v", err)
	}
}

func TestLoadConfigExpandsEnvRefsFromProcessEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("TEST_API_BASE", "https://example.test/v1")
	data := `{
	  "version": 4,
	  "model_list": [{
	    "model_name": "m",
	    "provider": "openai",
	    "model": "gpt-test",
	    "api_base": "$TEST_API_BASE",
	    "api_keys": ["$TEST_API_KEY"]
	  }],
	  "agents": {"defaults": {"model_name": "m"}}
	}`
	t.Setenv("TEST_API_KEY", "sk-test")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := cfg.ModelList[0].APIBase; got != "https://example.test/v1" {
		t.Fatalf("APIBase = %q", got)
	}
	if got := cfg.ModelList[0].APIKey(); got != "sk-test" {
		t.Fatalf("APIKey = %q", got)
	}
}

func TestLoadConfigMissingFileStillRunsPostProcessing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MOLE_HOME", dir)
	cfg, err := LoadConfig(filepath.Join(dir, "missing.json"))
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Version != CurrentVersion {
		t.Fatalf("Version = %d", cfg.Version)
	}
}

func TestLoadConfigRejectsMissingEnvRef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{"version":4,"model_list":[{"model_name":"m","provider":"openai","model":"gpt-test","api_base":"$NO_SUCH_BASE","api_keys":["sk"]}],"agents":{"defaults":{"model_name":"m"}}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "NO_SUCH_BASE") {
		t.Fatalf("error = %v, want NO_SUCH_BASE", err)
	}
}

func TestDefaultConfigHasNoBuiltInModels(t *testing.T) {
	if got := len(DefaultConfig().ModelList); got != 0 {
		t.Fatalf("DefaultConfig().ModelList length = %d, want 0", got)
	}
}
