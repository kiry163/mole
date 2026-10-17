// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package providers

import (
	"testing"

	"github.com/kiry163/mole/pkg/config"
)

func testConfig(models ...*config.ModelConfig) *config.Config {
	return &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{ModelName: models[0].ModelName},
		},
		ModelList: models,
	}
}

func TestCreateProviderUsesConfiguredModel(t *testing.T) {
	cfg := testConfig(&config.ModelConfig{
		ModelName: "primary",
		Provider:  "custom",
		Model:     "model-a",
		APIBase:   "https://example.com/v1",
	})
	cfg.ModelList[0].SetAPIKey("key")

	provider, modelID, err := CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider() error = %v", err)
	}
	if provider == nil {
		t.Fatal("CreateProvider() returned nil provider")
	}
	if modelID != "model-a" {
		t.Fatalf("modelID = %q, want model-a", modelID)
	}
}

func TestCreateProviderRequiresConfiguredModels(t *testing.T) {
	cfg := &config.Config{}
	if _, _, err := CreateProvider(cfg); err == nil {
		t.Fatal("expected error with empty model_list")
	}
}

func TestResolveModelConfigExactAlias(t *testing.T) {
	model := &config.ModelConfig{
		ModelName: "alias",
		Provider:  "custom",
		Model:     "upstream-id",
		APIBase:   "https://example.com/v1",
	}
	cfg := testConfig(model)

	resolved, err := ResolveModelConfig(cfg, "alias")
	if err != nil {
		t.Fatalf("ResolveModelConfig() error = %v", err)
	}
	if resolved.Model != "upstream-id" {
		t.Fatalf("resolved model = %q, want upstream-id", resolved.Model)
	}
	if resolved == model {
		t.Fatal("expected a clone, got the stored config entry")
	}
}

func TestResolveModelConfigRejectsProviderRefs(t *testing.T) {
	cfg := testConfig(&config.ModelConfig{
		ModelName: "alias",
		Provider:  "custom",
		Model:     "upstream-id",
		APIBase:   "https://example.com/v1",
	})

	if _, err := ResolveModelConfig(cfg, "custom/upstream-id"); err == nil {
		t.Fatal("expected provider-prefixed reference to be rejected")
	}
}
