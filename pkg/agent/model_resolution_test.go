package agent

import (
	"testing"

	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/providers"
)

func TestModelAliasFromCandidateIdentityKey(t *testing.T) {
	if got := modelAliasFromCandidateIdentityKey("model_name:primary"); got != "primary" {
		t.Fatalf("modelAliasFromCandidateIdentityKey() = %q, want %q", got, "primary")
	}
	if got := modelAliasFromCandidateIdentityKey("openai/gpt-5.4"); got != "" {
		t.Fatalf("modelAliasFromCandidateIdentityKey() = %q, want empty", got)
	}
}

func TestResolvedCandidateModelName_PrefersIdentityAlias(t *testing.T) {
	got := resolvedCandidateModelName([]providers.FallbackCandidate{
		{Provider: "openai", Model: "gpt-5.4", IdentityKey: "model_name:primary"},
	}, "fallback-model")
	if got != "primary" {
		t.Fatalf("resolvedCandidateModelName() = %q, want %q", got, "primary")
	}
}

func TestResolvedCandidateModelName_DoesNotScanFallbackAliases(t *testing.T) {
	got := resolvedCandidateModelName([]providers.FallbackCandidate{
		{Provider: "openai", Model: "gpt-5.4"},
		{Provider: "openai", Model: "gpt-5.4-mini", IdentityKey: "model_name:fallback"},
	}, "primary-model")
	if got != "primary-model" {
		t.Fatalf("resolvedCandidateModelName() = %q, want %q", got, "primary-model")
	}
}

func TestResolvedCandidateModelName_UsesCandidateDisplayName(t *testing.T) {
	got := resolvedCandidateModelName([]providers.FallbackCandidate{
		{Provider: "openai", Model: "gpt-5.4", DisplayName: "gpt-5.4-display"},
	}, "fallback-model")
	if got != "gpt-5.4-display" {
		t.Fatalf("resolvedCandidateModelName() = %q, want %q", got, "gpt-5.4-display")
	}
}

func TestResolveActiveModelConfig_PrefersCandidateConfigKey(t *testing.T) {
	second := &config.ModelConfig{
		ModelName: "suanneng-glm-4.7",
		Provider:  "zhipu",
		Model:     "glm-4.7",
		Streaming: config.ModelStreamingConfig{Enabled: true},
	}
	cfg := &config.Config{
		ModelList: []*config.ModelConfig{
			{
				ModelName: "glm-4.7",
				Provider:  "zhipu",
				Model:     "glm-4.7",
				Streaming: config.ModelStreamingConfig{Enabled: false},
			},
			second,
		},
	}

	got := resolveActiveModelConfig(
		cfg,
		[]providers.FallbackCandidate{{
			Provider:    "zhipu",
			Model:       "glm-4.7",
			IdentityKey: "model_name:suanneng-glm-4.7",
			ConfigIndex: 2,
			ConfigKey:   modelConfigResolutionKey(second),
		}},
		"glm-4.7",
	)

	if got == nil {
		t.Fatal("resolveActiveModelConfig() = nil, want model config")
	}
	if got.ModelName != "suanneng-glm-4.7" {
		t.Fatalf("model_name = %q, want %q", got.ModelName, "suanneng-glm-4.7")
	}
	if !got.Streaming.Enabled {
		t.Fatal("streaming.enabled = false, want true from selected load-balanced entry")
	}
}

func TestResolveActiveModelConfig_UsesAliasWhenNoCandidates(t *testing.T) {
	cfg := &config.Config{
		ModelList: []*config.ModelConfig{{
			ModelName: "primary",
			Provider:  "openai",
			Model:     "gpt-4o",
		}},
	}

	got := resolveActiveModelConfig(cfg, nil, "primary")
	if got == nil || got.ModelName != "primary" {
		t.Fatalf("resolveActiveModelConfig() = %#v, want primary", got)
	}
}

func TestResolveActiveModelConfig_ReturnsNilForUnconfiguredCandidate(t *testing.T) {
	cfg := &config.Config{
		ModelList: []*config.ModelConfig{{
			ModelName: "openai-gpt",
			Provider:  "openai",
			Model:     "gpt-4o",
		}},
	}

	got := resolveActiveModelConfig(
		cfg,
		[]providers.FallbackCandidate{{
			Provider: "nvidia",
			Model:    "gpt-4o",
		}},
		"gpt-4o",
	)
	if got != nil {
		t.Fatalf("resolveActiveModelConfig() = %#v, want nil for unconfigured candidate", got)
	}
}
