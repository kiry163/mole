// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package providers

import (
	"fmt"
	"strings"

	"github.com/kiry163/mole/pkg/config"
)

// ExtractProtocol returns the configured provider label and the upstream model
// identifier. The model identifier is passed through verbatim; it is never
// split on "/" because OpenAI-compatible model IDs may contain slashes.
func ExtractProtocol(cfg *config.ModelConfig) (provider, modelID string) {
	if cfg == nil {
		return "", ""
	}
	provider = NormalizeProvider(cfg.Provider)
	if provider == "" {
		provider = "openai"
	}
	return provider, strings.TrimSpace(cfg.Model)
}

// ResolveAPIBase returns the configured API base with trailing slashes removed.
func ResolveAPIBase(cfg *config.ModelConfig) string {
	if cfg == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(cfg.APIBase), "/")
}

// CreateProviderFromConfig creates an OpenAI-compatible provider from a model
// configuration. Mole only speaks the OpenAI Chat Completions protocol.
func CreateProviderFromConfig(cfg *config.ModelConfig) (LLMProvider, string, error) {
	if cfg == nil {
		return nil, "", fmt.Errorf("config is nil")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, "", fmt.Errorf("model is required")
	}

	providerName, modelID := ExtractProtocol(cfg)
	apiBase := ResolveAPIBase(cfg)
	if cfg.APIKey() == "" && apiBase == "" {
		return nil, "", fmt.Errorf("api_key or api_base is required")
	}

	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = fmt.Sprintf("Mole/%s", config.Version)
	}

	provider := NewHTTPProvider(
		cfg.APIKey(),
		apiBase,
		cfg.MaxTokensField,
		userAgent,
		cfg.RequestTimeout,
		cfg.ExtraBody,
		cfg.CustomHeaders,
	)
	provider.SetProviderName(providerName)
	return finalizeProviderFromConfig(provider, modelID, cfg)
}

func finalizeProviderFromConfig(
	provider LLMProvider,
	modelID string,
	cfg *config.ModelConfig,
) (LLMProvider, string, error) {
	return provider, modelID, nil
}
