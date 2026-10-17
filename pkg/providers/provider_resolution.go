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

// ResolveModelConfig resolves a configured model alias to its model entry.
// Only exact model_name matches are supported.
func ResolveModelConfig(cfg *config.Config, model string) (*config.ModelConfig, error) {
	model = strings.TrimSpace(model)
	if cfg == nil || model == "" {
		return nil, fmt.Errorf("model is required")
	}
	modelCfg, err := cfg.GetModelConfig(model)
	if err != nil || modelCfg == nil {
		return nil, fmt.Errorf("model %q is not configured", model)
	}
	clone := *modelCfg
	return &clone, nil
}

// CreateProvider creates a provider for the configured default model.
func CreateProvider(cfg *config.Config) (LLMProvider, string, error) {
	model := cfg.Agents.Defaults.GetModelName()

	if len(cfg.ModelList) == 0 {
		return nil, "", fmt.Errorf("no models configured. Please add a model in the console")
	}

	modelCfg, err := ResolveModelConfig(cfg, model)
	if err != nil {
		return nil, "", fmt.Errorf("model %q not found in model_list: %w", model, err)
	}

	provider, modelID, err := CreateProviderFromConfig(modelCfg)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create provider for model %q: %w", model, err)
	}

	return provider, modelID, nil
}
