// Mole - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 Mole contributors

package providers

import (
	"testing"

	"github.com/kiry163/mole/pkg/config"
)

func TestExtractProtocol(t *testing.T) {
	tests := []struct {
		name         string
		config       *config.ModelConfig
		wantProvider string
		wantModelID  string
	}{
		{
			name:         "explicit provider and slash model",
			config:       &config.ModelConfig{Provider: "openrouter", Model: "openai/gpt-4o"},
			wantProvider: "openrouter",
			wantModelID:  "openai/gpt-4o",
		},
		{
			name:         "provider normalized",
			config:       &config.ModelConfig{Provider: "  My-Provider ", Model: "model"},
			wantProvider: "my-provider",
			wantModelID:  "model",
		},
		{
			name:         "empty provider defaults to openai",
			config:       &config.ModelConfig{Model: "model"},
			wantProvider: "openai",
			wantModelID:  "model",
		},
		{
			name:         "model trimmed and passed through",
			config:       &config.ModelConfig{Provider: "custom", Model: "  meta/llama-3 "},
			wantProvider: "custom",
			wantModelID:  "meta/llama-3",
		},
		{
			name:         "nil config",
			wantProvider: "",
			wantModelID:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, modelID := ExtractProtocol(tt.config)
			if provider != tt.wantProvider {
				t.Errorf("provider = %q, want %q", provider, tt.wantProvider)
			}
			if modelID != tt.wantModelID {
				t.Errorf("modelID = %q, want %q", modelID, tt.wantModelID)
			}
		})
	}
}

func TestCreateProviderFromConfig_OpenAI(t *testing.T) {
	cfg := &config.ModelConfig{
		ModelName: "test-openai",
		Provider:  "openai",
		Model:     "gpt-4o",
		APIBase:   "https://api.example.com/v1",
	}
	cfg.SetAPIKey("test-key")

	provider, modelID, err := CreateProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
	if provider == nil {
		t.Fatal("CreateProviderFromConfig() returned nil provider")
	}
	if modelID != "gpt-4o" {
		t.Errorf("modelID = %q, want %q", modelID, "gpt-4o")
	}
}

func TestCreateProviderFromConfig_AllowsEmptyKeyWithBase(t *testing.T) {
	cfg := &config.ModelConfig{
		Provider: "local",
		Model:    "model",
		APIBase:  "http://localhost:11434/v1",
	}
	if _, _, err := CreateProviderFromConfig(cfg); err != nil {
		t.Fatalf("CreateProviderFromConfig() error = %v", err)
	}
}

func TestCreateProviderFromConfig_RequiresKeyOrBase(t *testing.T) {
	cfg := &config.ModelConfig{Provider: "custom", Model: "model"}
	if _, _, err := CreateProviderFromConfig(cfg); err == nil {
		t.Fatal("expected error when neither api_key nor api_base is set")
	}
}

func TestCreateProviderFromConfig_NilConfig(t *testing.T) {
	if _, _, err := CreateProviderFromConfig(nil); err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestCreateProviderFromConfig_EmptyModel(t *testing.T) {
	cfg := &config.ModelConfig{Provider: "custom", APIBase: "https://example.com/v1"}
	if _, _, err := CreateProviderFromConfig(cfg); err == nil {
		t.Fatal("expected error for empty model")
	}
}
