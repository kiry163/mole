package config

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kiry163/mole/pkg"
	"github.com/kiry163/mole/pkg/fileutil"
	"github.com/kiry163/mole/pkg/logger"
)

// rrCounter is a global counter for round-robin load balancing across models.
var rrCounter atomic.Uint64

// CurrentVersion is the latest config schema version
const CurrentVersion = 4

func init() {
	initChannel()
}

// Config is the current config structure with version support.
type Config struct {
	// Config schema version for migration.
	Version   int             `json:"version"             yaml:"-"`
	Agents    AgentsConfig    `json:"agents"              yaml:"-"`
	Session   SessionConfig   `json:"session,omitempty"   yaml:"-"`
	Channels  ChannelsConfig  `json:"channel_list"        yaml:"channel_list"`
	ModelList SecureModelList `json:"model_list"          yaml:"model_list"` // New model-centric provider configuration
	Dashboard DashboardConfig `json:"dashboard,omitempty" yaml:"dashboard,omitempty"`
	Gateway   GatewayConfig   `json:"gateway"             yaml:"-"`
	Events    EventsConfig    `json:"events,omitempty"    yaml:"-"`
	Tools     ToolsConfig     `json:"tools"               yaml:",inline"`

	// cache for sensitive values and compiled regex (computed once)
	sensitiveCache *SensitiveDataCache
	// envRefs remembers plain strings expanded from $VAR references so saves
	// can persist the reference rather than the resolved value.
	envRefs map[string]envRef
}

// FilterSensitiveData filters sensitive values from content before sending to LLM.
// This prevents the LLM from seeing its own credentials.
// Uses strings.Replacer for O(n+m) performance (computed once per SecurityConfig).
// Short content (below FilterMinLength) is returned unchanged for performance.
func (c *Config) FilterSensitiveData(content string) string {
	if c == nil {
		return content
	}
	// Check if filtering is enabled (default: true)
	if !c.Tools.IsFilterSensitiveDataEnabled() {
		return content
	}
	// Fast path: skip filtering for short content
	if len(content) < c.Tools.GetFilterMinLength() {
		return content
	}
	return c.SensitiveDataReplacer().Replace(content)
}

// MarshalJSON implements custom JSON marshaling for Config
// to omit providers section when empty and session when empty.
func (c *Config) MarshalJSON() ([]byte, error) {
	type Alias Config
	aux := &struct {
		Session *SessionConfig `json:"session,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}

	if len(c.Session.Dimensions) > 0 || len(c.Session.IdentityLinks) > 0 || c.Session.DmScope != "" {
		sessionCfg := c.Session
		aux.Session = &sessionCfg
	}

	return json.Marshal(aux)
}

type AgentsConfig struct {
	Defaults AgentDefaults `json:"defaults"`
}

type SessionConfig struct {
	Dimensions    []string            `json:"dimensions,omitempty"`
	IdentityLinks map[string][]string `json:"identity_links,omitempty"`
	DmScope       string              `json:"dm_scope,omitempty"`
}

// ApplyDmScope translates the user-facing dm_scope value into the internal
// dimensions array that the routing layer consumes. It is a no-op when
// DmScope is empty or when Dimensions is already set (explicit Dimensions
// take precedence over the derived value).
func (s *SessionConfig) ApplyDmScope() {
	if s.DmScope == "" || len(s.Dimensions) > 0 {
		return
	}
	switch s.DmScope {
	case "per-channel-peer":
		s.Dimensions = []string{"chat", "sender"}
	case "per-channel":
		s.Dimensions = []string{"chat"}
	case "per-peer":
		s.Dimensions = []string{"sender"}
	case "global":
		s.Dimensions = nil
	}
}

type ToolFeedbackConfig struct {
	Enabled          bool `json:"enabled"`
	MaxArgsLength    int  `json:"max_args_length"`
	SeparateMessages bool `json:"separate_messages"`
}

type AgentDefaults struct {
	Workspace                 string             `json:"workspace"`
	RestrictToWorkspace       bool               `json:"restrict_to_workspace"`
	AllowReadOutsideWorkspace bool               `json:"allow_read_outside_workspace"`
	ModelName                 string             `json:"model_name"`
	ModelFallbacks            []string           `json:"model_fallbacks,omitempty"`
	ImageModel                string             `json:"image_model,omitempty"`
	ImageModelFallbacks       []string           `json:"image_model_fallbacks,omitempty"`
	MaxTokens                 int                `json:"max_tokens"`
	ContextWindow             int                `json:"context_window,omitempty"`
	Temperature               *float64           `json:"temperature,omitempty"`
	MaxToolIterations         int                `json:"max_tool_iterations"`
	SummarizeMessageThreshold int                `json:"summarize_message_threshold"`
	SummarizeTokenPercent     int                `json:"summarize_token_percent"`
	MaxMediaSize              int                `json:"max_media_size,omitempty"`
	SteeringMode              string             `json:"steering_mode,omitempty"`      // "one-at-a-time" (default) or "all"
	MaxParallelTurns          int                `json:"max_parallel_turns,omitempty"` // Max concurrent turns (0 or 1 = sequential)
	ToolFeedback              ToolFeedbackConfig `json:"tool_feedback,omitempty"`
	SplitOnMarker             bool               `json:"split_on_marker"` // split messages on <|[SPLIT]|> marker
	MaxLLMRetries             int                `json:"max_llm_retries,omitempty"`
	LLMRetryBackoffSecs       int                `json:"llm_retry_backoff_secs,omitempty"`
}

const DefaultMaxMediaSize = 20 * 1024 * 1024 // 20 MB

func (d *AgentDefaults) GetMaxMediaSize() int {
	if d.MaxMediaSize > 0 {
		return d.MaxMediaSize
	}
	return DefaultMaxMediaSize
}

// GetToolFeedbackMaxArgsLength returns the max visible text length for tool argument previews.
func (d *AgentDefaults) GetToolFeedbackMaxArgsLength() int {
	if d.ToolFeedback.MaxArgsLength > 0 {
		return d.ToolFeedback.MaxArgsLength
	}
	return 300
}

// IsToolFeedbackEnabled returns true when tool feedback messages should be sent to the chat.
func (d *AgentDefaults) IsToolFeedbackEnabled() bool {
	return d.ToolFeedback.Enabled
}

// IsToolFeedbackSeparateMessagesEnabled returns true when each tool feedback
// update should be sent as its own chat message instead of editing a single
// in-place progress message.
func (d *AgentDefaults) IsToolFeedbackSeparateMessagesEnabled() bool {
	return d.ToolFeedback.SeparateMessages
}

// GetModelName returns the configured default model name.
func (d *AgentDefaults) GetModelName() string {
	return d.ModelName
}

// GroupTriggerConfig controls when the bot responds in group chats.
type GroupTriggerConfig struct {
	MentionOnly bool     `json:"mention_only,omitempty"`
	Prefixes    []string `json:"prefixes,omitempty"`
}

// TypingConfig controls typing indicator behavior (Phase 10).
type TypingConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

// PlaceholderConfig controls placeholder message behavior (Phase 10).
type PlaceholderConfig struct {
	Enabled bool                `json:"enabled"`
	Text    FlexibleStringSlice `json:"text,omitempty"`
}

// GetRandomText returns a random placeholder text, or default if none set.
func (p *PlaceholderConfig) GetRandomText() string {
	if len(p.Text) == 0 {
		return "Thinking..."
	}
	if len(p.Text) == 1 {
		return p.Text[0]
	}
	idx := rand.Intn(len(p.Text))
	return p.Text[idx]
}

type StreamingConfig struct {
	Enabled         bool `json:"enabled,omitempty"`
	ThrottleSeconds int  `json:"throttle_seconds,omitempty"`
	MinGrowthChars  int  `json:"min_growth_chars,omitempty"`
}

func (c StreamingConfig) IsZero() bool {
	return !c.Enabled && c.ThrottleSeconds == 0 && c.MinGrowthChars == 0
}

func (c StreamingConfig) WithDefaults(throttleSeconds, minGrowthChars int) StreamingConfig {
	if c.Enabled {
		if c.ThrottleSeconds == 0 {
			c.ThrottleSeconds = throttleSeconds
		}
		if c.MinGrowthChars == 0 {
			c.MinGrowthChars = minGrowthChars
		}
	}
	return c
}

type QQSettings struct {
	AppID                string       `json:"app_id"                   yaml:"-"`
	AppSecret            SecureString `json:"app_secret,omitzero"      yaml:"app_secret,omitempty"`
	MaxMessageLength     int          `json:"max_message_length"       yaml:"-"`
	MaxBase64FileSizeMiB int64        `json:"max_base64_file_size_mib" yaml:"-"`
	SendMarkdown         bool         `json:"send_markdown"            yaml:"-"`
}

type WeixinSettings struct {
	Token      SecureString `json:"token,omitzero"       yaml:"token,omitempty"`
	AccountID  string       `json:"account_id,omitempty" yaml:"-"`
	BaseURL    string       `json:"base_url"             yaml:"-"`
	CDNBaseURL string       `json:"cdn_base_url"         yaml:"-"`
	Proxy      string       `json:"proxy"                yaml:"-"`
}

// SetToken sets the Weixin token and marks it as dirty for security saving
func (c *WeixinSettings) SetToken(token string) {
	c.Token = *NewSecureString(token)
}

type WebChatSettings struct {
	Token           SecureString    `json:"token,omitzero"              yaml:"token,omitempty"`
	AllowTokenQuery bool            `json:"allow_token_query,omitempty" yaml:"-"`
	AllowOrigins    []string        `json:"allow_origins,omitempty"     yaml:"-"`
	Streaming       StreamingConfig `json:"streaming,omitzero"          yaml:"-"`
	PingInterval    int             `json:"ping_interval,omitempty"     yaml:"-"`
	ReadTimeout     int             `json:"read_timeout,omitempty"      yaml:"-"`
	MaxConnections  int             `json:"max_connections,omitempty"   yaml:"-"`
}

// SetToken sets the WebChat token and marks it as dirty for security saving
func (c *WebChatSettings) SetToken(token string) {
	c.Token = *NewSecureString(token)
}

type ModelStreamingConfig struct {
	Enabled bool `json:"enabled,omitempty"`
}

func (c ModelStreamingConfig) IsZero() bool {
	return !c.Enabled
}

// ModelConfig represents one OpenAI-compatible model endpoint.
// Provider is a free-form label used for grouping and diagnostics; Model is the
// upstream model identifier and is sent verbatim.
type ModelConfig struct {
	// Required fields
	ModelName string `json:"model_name"` // User-facing alias for the model
	Provider  string `json:"provider"`   // Free-form provider label used for grouping and diagnostics.
	Model     string `json:"model"`      // Upstream model identifier, passed through verbatim.

	// OpenAI-compatible endpoint
	APIBase   string   `json:"api_base,omitempty"`  // API endpoint URL
	Fallbacks []string `json:"fallbacks,omitempty"` // Fallback model names for failover

	// Optional optimizations
	RPM            int                  `json:"rpm,omitempty"`              // Requests per minute limit
	MaxTokensField string               `json:"max_tokens_field,omitempty"` // Field name for max tokens (e.g., "max_completion_tokens")
	RequestTimeout int                  `json:"request_timeout,omitempty"`
	ThinkingLevel  string               `json:"thinking_level,omitempty"` // Extended thinking: off|low|medium|high|xhigh|adaptive
	Streaming      ModelStreamingConfig `json:"streaming,omitzero"`       // Opt-in for provider streaming on this model entry
	ExtraBody      map[string]any       `json:"extra_body,omitempty"`     // Additional fields to inject into request body
	CustomHeaders  map[string]string    `json:"custom_headers,omitempty"` // Additional headers to inject into every HTTP request

	APIKeys SecureStrings `json:"api_keys,omitzero" yaml:"api_keys,omitempty"` // API authentication keys (multiple keys for failover)

	// Enabled indicates whether this model entry is active. When omitted in
	// existing configs, the field is inferred during load: models with API keys
	// or the reserved "local-model" name are auto-enabled.
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	// UserAgent is the user agent string to use for HTTP requests.
	UserAgent string `json:"user_agent,omitempty" yaml:"-"`

	// isVirtual marks this model as a virtual model generated from multi-key expansion.
	// Virtual models should not be persisted to config files.
	isVirtual bool
}

// APIKey returns the first API key from apiKeys
func (c *ModelConfig) APIKey() string {
	if len(c.APIKeys) > 0 {
		return c.APIKeys[0].String()
	}
	return ""
}

// IsVirtual returns true if this model was generated from multi-key expansion.
func (c *ModelConfig) IsVirtual() bool {
	return c.isVirtual
}

// Validate checks if the ModelConfig has all required fields.
func (c *ModelConfig) Validate() error {
	if c.ModelName == "" {
		return fmt.Errorf("model_name is required")
	}
	if c.Model == "" {
		return fmt.Errorf("model is required")
	}
	// Reject whitespace in model identifier
	if strings.ContainsAny(c.Model, " \t\n\r") {
		return fmt.Errorf("model identifier contains whitespace")
	}

	// Reject leading slash
	if strings.HasPrefix(c.Model, "/") {
		return fmt.Errorf("model identifier must not start with /")
	}

	// Reject consecutive slashes
	if strings.Contains(c.Model, "//") {
		return fmt.Errorf("model identifier must not contain //")
	}
	return nil
}

func (c *ModelConfig) SetAPIKey(value string) {
	if len(c.APIKeys) > 0 {
		c.APIKeys[0].Set(value)
	} else {
		c.APIKeys = append(c.APIKeys, NewSecureString(value))
	}
}

type ToolDiscoveryConfig struct {
	Enabled          bool `json:"enabled"`
	TTL              int  `json:"ttl"`
	MaxSearchResults int  `json:"max_search_results"`
	UseBM25          bool `json:"use_bm25"`
	UseRegex         bool `json:"use_regex"`
}

type ToolConfig struct {
	Enabled bool `json:"enabled" yaml:"-"`
}

type MessageToolsConfig struct {
	ToolConfig `yaml:"-"`

	MediaEnabled bool `json:"media_enabled" yaml:"-"`
}

type TavilyConfig struct {
	Enabled    bool          `json:"enabled"           yaml:"-"`
	APIKeys    SecureStrings `json:"api_keys,omitzero" yaml:"api_keys,omitempty"`
	BaseURL    string        `json:"base_url"          yaml:"-"`
	MaxResults int           `json:"max_results"       yaml:"-"`
}

// APIKey returns the Tavily API key
func (c *TavilyConfig) APIKey() string {
	if len(c.APIKeys) == 0 {
		return ""
	}
	return c.APIKeys[0].String()
}

// SetAPIKey sets the Tavily API key
func (c *TavilyConfig) SetAPIKey(key string) {
	c.APIKeys = SimpleSecureStrings(key)
}

// SetAPIKeys sets the Tavily API keys
func (c *TavilyConfig) SetAPIKeys(keys []string) {
	c.APIKeys = make(SecureStrings, len(keys))
	for i, k := range keys {
		c.APIKeys[i] = NewSecureString(k)
	}
}

type SogouConfig struct {
	Enabled    bool `json:"enabled"`
	MaxResults int  `json:"max_results"`
}

type WebToolsConfig struct {
	ToolConfig `yaml:"-"`
	Tavily     TavilyConfig `yaml:"tavily,omitempty" json:"tavily"`
	Sogou      SogouConfig  `yaml:"-"                json:"sogou"`
	Provider   string       `yaml:"-"                json:"provider,omitempty"`
	// Proxy is an optional proxy URL for web tools (http/https/socks5/socks5h).
	// For authenticated proxies, prefer HTTP_PROXY/HTTPS_PROXY env vars instead of embedding credentials in config.
	Proxy                string              `yaml:"-" json:"proxy,omitempty"`
	FetchLimitBytes      int64               `yaml:"-" json:"fetch_limit_bytes,omitempty"`
	Format               string              `yaml:"-" json:"format,omitempty"`
	PrivateHostWhitelist FlexibleStringSlice `yaml:"-" json:"private_host_whitelist,omitempty"`
}

type ExecConfig struct {
	ToolConfig
	EnableDenyPatterns  bool     `                                 json:"enable_deny_patterns"`
	AllowRemote         bool     `                                 json:"allow_remote"`
	CustomDenyPatterns  []string `                                 json:"custom_deny_patterns"`
	CustomAllowPatterns []string `                                 json:"custom_allow_patterns"`
	TimeoutSeconds      int      `                                 json:"timeout_seconds"` // 0 means use default (60s)
}

type MediaCleanupConfig struct {
	ToolConfig
	MaxAge   int `                                    json:"max_age_minutes"`
	Interval int `                                    json:"interval_minutes"`
}

type ReadFileToolConfig struct {
	Enabled         bool   `json:"enabled"`
	Mode            string `json:"mode"`
	MaxReadFileSize int    `json:"max_read_file_size"`
}

const (
	ReadFileModeBytes = "bytes"
	ReadFileModeLines = "lines"
)

func (c ReadFileToolConfig) EffectiveMode() string {
	switch strings.ToLower(strings.TrimSpace(c.Mode)) {
	case ReadFileModeLines:
		return ReadFileModeLines
	case "", ReadFileModeBytes:
		return ReadFileModeBytes
	default:
		return ReadFileModeBytes
	}
}

type ToolsConfig struct {
	AllowReadPaths  []string `json:"allow_read_paths"  yaml:"-"`
	AllowWritePaths []string `json:"allow_write_paths" yaml:"-"`
	// FilterSensitiveData controls whether to filter sensitive values (API keys,
	// tokens, secrets) from tool results before sending to the LLM.
	// Default: true (enabled)
	FilterSensitiveData bool `json:"filter_sensitive_data" yaml:"-"`
	// FilterMinLength is the minimum content length required for filtering.
	// Content shorter than this will be returned unchanged for performance.
	// Default: 8
	FilterMinLength int                `json:"filter_min_length" yaml:"-"`
	Web             WebToolsConfig     `json:"web"               yaml:"web,omitempty"`
	Exec            ExecConfig         `json:"exec"              yaml:"-"`
	MediaCleanup    MediaCleanupConfig `json:"media_cleanup"     yaml:"-"`
	MCP             MCPConfig          `json:"mcp"               yaml:"-"`
	AppendFile      ToolConfig         `json:"append_file"       yaml:"-"`
	EditFile        ToolConfig         `json:"edit_file"         yaml:"-"`
	ListDir         ToolConfig         `json:"list_dir"          yaml:"-"`
	LoadImage       ToolConfig         `json:"load_image"        yaml:"-"`
	Message         MessageToolsConfig `json:"message"           yaml:"-"`
	ReadFile        ReadFileToolConfig `json:"read_file"         yaml:"-"`
	SendFile        ToolConfig         `json:"send_file"         yaml:"-"`
	WebFetch        ToolConfig         `json:"web_fetch"         yaml:"-"`
	WriteFile       ToolConfig         `json:"write_file"        yaml:"-"`
}

// IsFilterSensitiveDataEnabled returns true if sensitive data filtering is enabled
func (c *ToolsConfig) IsFilterSensitiveDataEnabled() bool {
	return c.FilterSensitiveData
}

// GetFilterMinLength returns the minimum content length for filtering (default: 8)
func (c *ToolsConfig) GetFilterMinLength() int {
	if c.FilterMinLength <= 0 {
		return 8
	}
	return c.FilterMinLength
}

// MCPServerConfig defines configuration for a single MCP server.
type MCPServerConfig struct {
	Enabled  bool              `json:"enabled"`
	Deferred *bool             `json:"deferred,omitempty"`
	Command  string            `json:"command"`
	Args     []string          `json:"args,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	EnvFile  string            `json:"env_file,omitempty"`
	Type     string            `json:"type,omitempty"`
	URL      string            `json:"url,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
}

type MCPConfig struct {
	ToolConfig
	Discovery ToolDiscoveryConfig `                                json:"discovery"`
	// MaxInlineTextChars controls how much MCP text stays inline before it is saved as an artifact.
	MaxInlineTextChars int `json:"max_inline_text_chars,omitempty"`
	// Servers is a map of server name to server configuration
	Servers map[string]MCPServerConfig `json:"servers,omitempty"`
}

const DefaultMCPMaxInlineTextChars = 16 * 1024

func (c *MCPConfig) GetMaxInlineTextChars() int {
	if c.MaxInlineTextChars > 0 {
		return c.MaxInlineTextChars
	}
	return DefaultMCPMaxInlineTextChars
}

// loadConfig decodes a config file written in the current schema version.
func loadConfig(data []byte) (*Config, error) {
	cfg := DefaultConfig()

	// Pre-scan the JSON to check how many model_list entries the user provided.
	// Go's JSON decoder reuses existing slice backing-array elements rather than
	// zero-initializing them, so fields absent from the user's JSON (e.g. api_base)
	// would silently inherit values from the DefaultConfig template at the same
	// index position. We only reset cfg.ModelList when the user actually provides
	// entries; when count is 0 we keep DefaultConfig's built-in list as fallback.
	var tmp Config
	if err := decodeJSONWithDiagnostics(data, &tmp, "config.json"); err != nil {
		return nil, err
	}
	if len(tmp.ModelList) > 0 {
		cfg.ModelList = nil
	}

	if err := decodeJSONWithDiagnostics(data, cfg, "config.json"); err != nil {
		return nil, err
	}
	return cfg, nil
}

func LoadConfig(path string) (*Config, error) {
	updateResolver(filepath.Dir(path))
	cfg, err := loadOrInitConfig(path)
	if err != nil {
		return nil, err
	}
	if err := InitChannelList(cfg.Channels); err != nil {
		return nil, err
	}
	// Expand multi-key models before recording plain-string $VAR paths: the
	// expansion rebuilds ModelList, so paths recorded on the original slice
	// would point at the wrong entries when the config is saved.
	cfg.ModelList = expandMultiKeyModels(cfg.ModelList)
	if err := ExpandEnvRefs(cfg); err != nil {
		return nil, err
	}
	cfg.Gateway.Host, err = normalizeGatewayHostInput(cfg.Gateway.Host)
	if err != nil {
		return nil, fmt.Errorf("invalid gateway host: %w", err)
	}
	if err := cfg.ValidateModelList(); err != nil {
		return nil, err
	}
	if cfg.Agents.Defaults.Workspace == "" {
		cfg.Agents.Defaults.Workspace = filepath.Join(GetHome(), pkg.WorkspaceName)
	}
	cfg.Session.ApplyDmScope()
	return cfg, nil
}

func loadOrInitConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			logger.WarnF("config file not found, using default config",
				map[string]any{"path": path})
			return DefaultConfig(), nil
		}
		return nil, err
	}
	if len(data) <= 10 {
		logger.Warn(fmt.Sprintf("content is [%s]", string(data)))
		return DefaultConfig(), nil
	}
	var versionInfo struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &versionInfo); err != nil {
		err = wrapJSONError(data, err, "config.json")
		logger.ErrorCF("config", formatDiagnosticLogMessage("Malformed config file", err),
			map[string]any{"path": path})
		return nil, err
	}
	if versionInfo.Version != CurrentVersion {
		return nil, fmt.Errorf("unsupported config version: %d", versionInfo.Version)
	}
	cfg, err := loadConfig(data)
	if err != nil {
		logger.ErrorCF("config", formatDiagnosticLogMessage("Failed to load config", err),
			map[string]any{"path": path})
		return nil, err
	}
	return cfg, nil
}

func MakeBackup(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	dateSuffix := time.Now().Format(".20060102.bak")
	// Backup config file
	bakPath := path + dateSuffix
	if err := fileutil.CopyFile(path, bakPath, 0o600); err != nil {
		logger.ErrorF("failed to create config backup", map[string]any{"error": err})
		return fmt.Errorf("failed to create config backup: %w", err)
	}
	// Backup the instance .env holding promoted secrets
	envPath := EnvFilePath(path)
	if _, err := os.Stat(envPath); err == nil {
		if envErr := fileutil.CopyFile(envPath, envPath+dateSuffix, 0o600); envErr != nil {
			logger.ErrorF("failed to create env backup", map[string]any{"error": envErr})
			return fmt.Errorf("failed to create env backup: %w", envErr)
		}
	}
	return nil
}

func SaveConfig(path string, cfg *Config) error {
	if cfg.Version < CurrentVersion {
		cfg.Version = CurrentVersion
	}
	// Persist $VAR references rather than the values they resolved to, and
	// reject references that would fail the next load.
	cfg.RestoreEnvRefs()
	if err := ValidateEnvRefs(cfg); err != nil {
		return err
	}
	// Filter out virtual models before serializing to config file
	nonVirtualModels := make([]*ModelConfig, 0, len(cfg.ModelList))
	for _, m := range cfg.ModelList {
		if !m.isVirtual {
			nonVirtualModels = append(nonVirtualModels, m)
		}
	}
	// Temporarily replace ModelList with filtered version for serialization
	originalModelList := cfg.ModelList
	defer func() {
		// Restore original ModelList after serialization
		cfg.ModelList = originalModelList
	}()
	cfg.ModelList = nonVirtualModels

	if err := promoteSecrets(path, cfg); err != nil {
		logger.ErrorCF("config", "cannot promote secrets to .env", map[string]any{"error": err})
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	// Callers keep working with resolved runtime values. Restore the full list
	// first so envRefs are recorded against the same slice LoadConfig saw.
	cfg.ModelList = originalModelList
	_ = ExpandEnvRefs(cfg)
	return fileutil.WriteFileAtomic(path, data, 0o600)
}

func (c *Config) WorkspacePath() string {
	return expandHome(c.Agents.Defaults.Workspace)
}

func expandHome(path string) string {
	if path == "" {
		return path
	}
	if path[0] == '~' {
		home, _ := os.UserHomeDir()
		if len(path) > 1 && path[1] == '/' {
			return home + path[1:]
		}
		return home
	}
	return path
}

// GetModelConfig returns the ModelConfig for the given model name.
// If multiple configs exist with the same model_name, it uses round-robin
// selection for load balancing. Returns an error if the model is not found.
func (c *Config) GetModelConfig(modelName string) (*ModelConfig, error) {
	matches := c.findMatches(modelName)
	if len(matches) == 0 {
		return nil, fmt.Errorf("model %q not found in model_list or providers", modelName)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}

	// Multiple configs - use round-robin for load balancing
	idx := (rrCounter.Add(1) - 1) % uint64(len(matches))
	return matches[idx], nil
}

// findMatches finds all ModelConfig entries with the given model_name.
func (c *Config) findMatches(modelName string) []*ModelConfig {
	var matches []*ModelConfig
	for i := range c.ModelList {
		if c.ModelList[i].ModelName == modelName {
			matches = append(matches, c.ModelList[i])
		}
	}
	return matches
}

// ValidateModelList validates all ModelConfig entries in the model_list.
// It checks that each model config is valid.
// Note: Multiple entries with the same model_name are allowed for load balancing.
func (c *Config) ValidateModelList() error {
	for i := range c.ModelList {
		if err := c.ModelList[i].Validate(); err != nil {
			return fmt.Errorf("model_list[%d]: %w", i, err)
		}
	}
	return nil
}

// ResetToDefaults backs up the current config, creates a default config,
// preserves security credentials from the existing config, and saves it.
func ResetToDefaults(configPath string) error {
	if err := MakeBackup(configPath); err != nil {
		return fmt.Errorf("backup before reset: %w", err)
	}
	cfg := DefaultConfig()
	cfg.Session.ApplyDmScope()
	if err := cfg.SecurityCopyFrom(configPath); err != nil {
		logger.WarnF("could not preserve security config", map[string]any{"error": err})
	}
	return SaveConfig(configPath, cfg)
}

func expandMultiKeyModels(models []*ModelConfig) []*ModelConfig {
	var expanded []*ModelConfig

	for _, m := range models {
		keys := m.APIKeys.Values()

		// Single key or no keys: keep as-is
		if len(keys) <= 1 {
			expanded = append(expanded, m)
			continue
		}

		// Multiple keys: expand
		originalName := m.ModelName

		// Create entries for additional keys (key_1, key_2, ...)
		var fallbackNames []string
		for i := 1; i < len(keys); i++ {
			suffix := fmt.Sprintf("__key_%d", i)
			expandedName := originalName + suffix

			// Create a copy for the additional key
			additionalEntry := &ModelConfig{
				ModelName:      expandedName,
				Provider:       m.Provider,
				Model:          m.Model,
				APIBase:        m.APIBase,
				APIKeys:        SimpleSecureStrings(keys[i]),
				RPM:            m.RPM,
				MaxTokensField: m.MaxTokensField,
				RequestTimeout: m.RequestTimeout,
				ThinkingLevel:  m.ThinkingLevel,
				Streaming:      m.Streaming,
				ExtraBody:      m.ExtraBody,
				CustomHeaders:  m.CustomHeaders,
				UserAgent:      m.UserAgent,
				isVirtual:      true,
			}
			expanded = append(expanded, additionalEntry)
			fallbackNames = append(fallbackNames, expandedName)
		}

		// Create the primary entry with first key and fallbacks
		primaryEntry := &ModelConfig{
			ModelName:      originalName,
			Provider:       m.Provider,
			Model:          m.Model,
			APIBase:        m.APIBase,
			RPM:            m.RPM,
			MaxTokensField: m.MaxTokensField,
			RequestTimeout: m.RequestTimeout,
			ThinkingLevel:  m.ThinkingLevel,
			Streaming:      m.Streaming,
			ExtraBody:      m.ExtraBody,
			CustomHeaders:  m.CustomHeaders,
			UserAgent:      m.UserAgent,
			APIKeys:        SimpleSecureStrings(keys[0]),
		}

		// Prepend new fallbacks to existing ones
		if len(fallbackNames) > 0 {
			primaryEntry.Fallbacks = append(fallbackNames, m.Fallbacks...)
		} else if len(m.Fallbacks) > 0 {
			primaryEntry.Fallbacks = m.Fallbacks
		}

		expanded = append(expanded, primaryEntry)
	}

	return expanded
}

func (t *ToolsConfig) IsToolEnabled(name string) bool {
	switch name {
	case "web":
		return t.Web.Enabled
	case "exec":
		return t.Exec.Enabled
	case "media_cleanup":
		return t.MediaCleanup.Enabled
	case "append_file":
		return t.AppendFile.Enabled
	case "edit_file":
		return t.EditFile.Enabled
	case "list_dir":
		return t.ListDir.Enabled
	case "load_image":
		return t.LoadImage.Enabled
	case "message":
		return t.Message.Enabled
	case "read_file":
		return t.ReadFile.Enabled
	case "web_fetch":
		return t.WebFetch.Enabled
	case "send_file":
		return t.SendFile.Enabled
	case "write_file":
		return t.WriteFile.Enabled
	case "mcp":
		return t.MCP.Enabled
	default:
		return true
	}
}
