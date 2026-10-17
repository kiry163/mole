// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package providers

import (
	"fmt"
	"strings"

	"github.com/kiry163/mole/pkg/config"
	anthropicmessages "github.com/kiry163/mole/pkg/providers/anthropic_messages"
	"github.com/kiry163/mole/pkg/providers/common"
)

// ExtractProtocol extracts the effective protocol and model identifier from a
// model configuration.
//
// The explicit Provider field takes precedence. When Provider is empty, the
// protocol is inferred from Model. Plain model names default to "openai".
// Provider-prefixed models strip the first slash-separated segment from the
// returned model ID.
//
// The returned protocol is normalized to the provider's canonical spelling.
// Examples:
//   - Model "openai/gpt-4o" -> ("openai", "gpt-4o")
//   - Model "nvidia/z-ai/glm-5.1" -> ("nvidia", "z-ai/glm-5.1")
//   - Provider "nvidia", Model "z-ai/glm-5.1" -> ("nvidia", "z-ai/glm-5.1")
//   - Provider "openai", Model "openai/gpt-4o" -> ("openai", "openai/gpt-4o")
//   - Model "gpt-4o" -> ("openai", "gpt-4o")
func ExtractProtocol(cfg *config.ModelConfig) (protocol, modelID string) {
	if cfg == nil {
		return "", ""
	}

	model := strings.TrimSpace(cfg.Model)
	if provider := strings.TrimSpace(cfg.Provider); provider != "" {
		return NormalizeProvider(provider), model
	}
	return SplitModelProviderAndID(model, "openai")
}

// ResolveAPIBase returns the configured API base, or the protocol default when
// the model uses an HTTP-based provider family with a known default endpoint.
func ResolveAPIBase(cfg *config.ModelConfig) string {
	if cfg == nil {
		return ""
	}
	if apiBase := strings.TrimSpace(cfg.APIBase); apiBase != "" {
		return strings.TrimRight(apiBase, "/")
	}
	protocol, _ := ExtractProtocol(cfg)
	return strings.TrimRight(getDefaultAPIBase(protocol), "/")
}

// CreateProviderFromConfig creates a provider based on the ModelConfig.
// It uses ExtractProtocol to determine which provider to create.
// Supported protocol families include OpenAI-compatible prefixes (e.g., openai, openrouter, groq)
// and Anthropic (including anthropic-messages).
// See the switch on protocol in this function for the authoritative list.
// Returns the provider, the effective model ID from ExtractProtocol, and any error.
func CreateProviderFromConfig(cfg *config.ModelConfig) (LLMProvider, string, error) {
	if cfg == nil {
		return nil, "", fmt.Errorf("config is nil")
	}

	if cfg.Model == "" {
		return nil, "", fmt.Errorf("model is required")
	}

	protocol, modelID := ExtractProtocol(cfg)

	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = fmt.Sprintf("Mole/%s", config.Version)
	}

	switch protocol {
	case "openai":
		// OpenAI with API key
		if cfg.APIKey() == "" && cfg.APIBase == "" {
			return nil, "", fmt.Errorf("api_key or api_base is required for HTTP-based protocol %q", protocol)
		}
		apiBase := cfg.APIBase
		if apiBase == "" {
			apiBase = getDefaultAPIBase(protocol)
		}
		provider := NewHTTPProviderWithMaxTokensFieldAndRequestTimeout(
			cfg.APIKey(),
			apiBase,
			cfg.Proxy,
			cfg.MaxTokensField,
			userAgent,
			cfg.RequestTimeout,
			cfg.ExtraBody,
			cfg.CustomHeaders,
		)
		provider.SetProviderName(protocol)
		return finalizeProviderFromConfig(provider, modelID, cfg)

	case "litellm", "lmstudio", "gpt4free", "openrouter", "groq", "zhipu", "nvidia", "venice",
		"nearai", "ollama", "moonshot", "shengsuanyun", "siliconflow", "deepseek", "cerebras",
		"vivgrid", "volcengine", "vllm", "qwen-portal", "qwen-intl", "qwen-us", "mistral",
		"avian", "longcat", "modelscope", "novita", "alibaba-coding", "zai", "mimo":
		// All other OpenAI-compatible HTTP providers
		if cfg.APIKey() == "" && cfg.APIBase == "" && !isEmptyAPIKeyAllowed(protocol) {
			return nil, "", fmt.Errorf("api_key or api_base is required for HTTP-based protocol %q", protocol)
		}
		apiBase := cfg.APIBase
		if apiBase == "" {
			apiBase = getDefaultAPIBase(protocol)
		}
		provider := NewHTTPProviderWithMaxTokensFieldAndRequestTimeout(
			cfg.APIKey(),
			apiBase,
			cfg.Proxy,
			cfg.MaxTokensField,
			userAgent,
			cfg.RequestTimeout,
			cfg.ExtraBody,
			cfg.CustomHeaders,
		)
		provider.SetProviderName(protocol)
		return finalizeProviderFromConfig(provider, modelID, cfg)

	case "anthropic":
		// Use API key with HTTP API
		apiBase := common.NormalizeBaseURL(cfg.APIBase, "https://api.anthropic.com/v1", true)
		if cfg.APIKey() == "" {
			return nil, "", fmt.Errorf("api_key is required for anthropic protocol (model: %s)", cfg.Model)
		}
		provider := NewHTTPProviderWithMaxTokensFieldAndRequestTimeout(
			cfg.APIKey(),
			apiBase,
			cfg.Proxy,
			cfg.MaxTokensField,
			userAgent,
			cfg.RequestTimeout,
			cfg.ExtraBody,
			cfg.CustomHeaders,
		)
		provider.SetProviderName(protocol)
		return finalizeProviderFromConfig(provider, modelID, cfg)

	case "anthropic-messages":
		// Anthropic Messages API with native format (HTTP-based, no SDK)
		apiBase := cfg.APIBase
		if apiBase == "" {
			apiBase = "https://api.anthropic.com/v1"
		}
		if cfg.APIKey() == "" {
			return nil, "", fmt.Errorf("api_key is required for anthropic-messages protocol (model: %s)", cfg.Model)
		}
		return finalizeProviderFromConfig(anthropicmessages.NewProviderWithTimeout(
			cfg.APIKey(),
			apiBase,
			userAgent,
			cfg.RequestTimeout,
		), modelID, cfg)

	case "alibaba-coding-anthropic":
		// Alibaba Coding Plan with Anthropic-compatible API
		apiBase := cfg.APIBase
		if apiBase == "" {
			apiBase = getDefaultAPIBase(protocol)
		}
		if cfg.APIKey() == "" {
			return nil, "", fmt.Errorf("api_key is required for %q protocol (model: %s)", protocol, cfg.Model)
		}
		return finalizeProviderFromConfig(anthropicmessages.NewProviderWithTimeout(
			cfg.APIKey(),
			apiBase,
			userAgent,
			cfg.RequestTimeout,
		), modelID, cfg)

	default:
		return nil, "", fmt.Errorf("unknown protocol %q in model %q", protocol, cfg.Model)
	}
}

func finalizeProviderFromConfig(
	provider LLMProvider,
	modelID string,
	cfg *config.ModelConfig,
) (LLMProvider, string, error) {
	wrapped, err := wrapProviderWithToolSchemaTransform(provider, cfg.ToolSchemaTransform)
	if err != nil {
		return nil, "", err
	}
	return wrapped, modelID, nil
}

func isEmptyAPIKeyAllowed(protocol string) bool {
	option, ok := modelProviderOptionForName(protocol)
	return ok && option.EmptyAPIKeyAllowed
}

// IsEmptyAPIKeyAllowedForProtocol reports whether a protocol allows requests
// without api_key when using its default local endpoint.
func IsEmptyAPIKeyAllowedForProtocol(protocol string) bool {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	return isEmptyAPIKeyAllowed(protocol)
}

// IsHTTPAPIProtocol reports whether a provider uses an HTTP API base in the
// model configuration path. This excludes providers that are not plain HTTP
// API endpoints even if they do not require an explicit api_key field.
func IsHTTPAPIProtocol(protocol string) bool {
	protocol = NormalizeProvider(protocol)
	option, ok := modelProviderOptionsByName[protocol]
	return ok && option.httpAPI
}

// DefaultAPIBaseForProtocol returns the configured default API base for a protocol.
// It returns empty string if the protocol has no default base.
func DefaultAPIBaseForProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	return getDefaultAPIBase(protocol)
}

// getDefaultAPIBase returns the default API base URL for a given protocol.
func getDefaultAPIBase(protocol string) string {
	option, ok := modelProviderOptionForName(protocol)
	if !ok {
		return ""
	}
	return option.DefaultAPIBase
}
