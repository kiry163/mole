package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/providers"
)

func modelConfigIdentityKey(mc *config.ModelConfig) string {
	if mc == nil {
		return ""
	}
	if name := strings.TrimSpace(mc.ModelName); name != "" {
		return "model_name:" + name
	}
	return ""
}

func modelConfigResolutionKey(mc *config.ModelConfig) string {
	if mc == nil {
		return ""
	}
	apiKeys := make([]string, len(mc.APIKeys))
	for i := range mc.APIKeys {
		apiKeys[i] = mc.APIKeys[i].String()
	}
	payload, err := json.Marshal(struct {
		ModelName           string                      `json:"model_name"`
		Provider            string                      `json:"provider"`
		Model               string                      `json:"model"`
		APIBase             string                      `json:"api_base"`
		APIKeys             []string                    `json:"api_keys"`
		Enabled             bool                        `json:"enabled"`
		UserAgent           string                      `json:"user_agent"`
		RPM                 int                         `json:"rpm"`
		RequestTimeout      int                         `json:"request_timeout"`
		MaxTokensField      string                      `json:"max_tokens_field"`
		ThinkingLevel       string                      `json:"thinking_level"`
		ToolSchemaTransform string                      `json:"tool_schema_transform"`
		Streaming           config.ModelStreamingConfig `json:"streaming"`
		ExtraBody           map[string]any              `json:"extra_body"`
		CustomHeaders       map[string]string           `json:"custom_headers"`
	}{
		ModelName: mc.ModelName, Provider: providers.NormalizeProvider(mc.Provider), Model: mc.Model,
		APIBase: mc.APIBase, APIKeys: apiKeys,
		Enabled: mc.Enabled, UserAgent: mc.UserAgent, RPM: mc.RPM,
		RequestTimeout: mc.RequestTimeout, MaxTokensField: mc.MaxTokensField,
		ThinkingLevel: mc.ThinkingLevel, ToolSchemaTransform: mc.ToolSchemaTransform,
		Streaming: mc.Streaming, ExtraBody: mc.ExtraBody, CustomHeaders: mc.CustomHeaders,
	})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("config:%x", sum)
}

func candidateFromModelConfig(mc *config.ModelConfig) (providers.FallbackCandidate, bool) {
	if mc == nil || strings.TrimSpace(mc.Model) == "" {
		return providers.FallbackCandidate{}, false
	}

	provider, modelID := providers.ExtractProtocol(mc)
	return providers.FallbackCandidate{
		Provider:    provider,
		Model:       modelID,
		DisplayName: strings.TrimSpace(mc.ModelName),
		RPM:         mc.RPM,
		IdentityKey: modelConfigIdentityKey(mc),
		ConfigKey:   modelConfigResolutionKey(mc),
	}, true
}

func lookupModelConfigByRef(cfg *config.Config, raw string) *config.ModelConfig {
	raw = strings.TrimSpace(raw)
	if raw == "" || cfg == nil {
		return nil
	}
	mc, err := cfg.GetModelConfig(raw)
	if err != nil {
		return nil
	}
	return mc
}

func resolveModelCandidate(
	cfg *config.Config,
	raw string,
) (providers.FallbackCandidate, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return providers.FallbackCandidate{}, false
	}

	mc := lookupModelConfigByRef(cfg, raw)
	if mc == nil {
		return providers.FallbackCandidate{}, false
	}
	candidate, ok := candidateFromModelConfig(mc)
	if !ok {
		return providers.FallbackCandidate{}, false
	}
	for index, modelCfg := range cfg.ModelList {
		if modelCfg == mc {
			candidate.ConfigIndex = index + 1
			candidate.ConfigKey = modelConfigResolutionKey(modelCfg)
			break
		}
	}
	return candidate, true
}

func resolveModelCandidates(
	cfg *config.Config,
	primary string,
	fallbacks []string,
) []providers.FallbackCandidate {
	seen := make(map[string]bool)
	candidates := make([]providers.FallbackCandidate, 0, 1+len(fallbacks))

	addCandidate := func(raw string) {
		candidate, ok := resolveModelCandidate(cfg, raw)
		if !ok {
			return
		}

		key := candidate.StableKey()
		if seen[key] {
			return
		}
		seen[key] = true
		candidates = append(candidates, candidate)
	}

	addCandidate(primary)
	for _, fallback := range fallbacks {
		addCandidate(fallback)
	}

	return candidates
}

func resolvedCandidateModel(candidates []providers.FallbackCandidate, fallback string) string {
	if len(candidates) > 0 && strings.TrimSpace(candidates[0].Model) != "" {
		return candidates[0].Model
	}
	return fallback
}

func resolvedCandidateProvider(candidates []providers.FallbackCandidate, fallback string) string {
	if len(candidates) > 0 && strings.TrimSpace(candidates[0].Provider) != "" {
		return candidates[0].Provider
	}
	return fallback
}

func resolvedCandidateModelName(candidates []providers.FallbackCandidate, fallback string) string {
	if len(candidates) > 0 {
		if name := modelAliasFromCandidateIdentityKey(candidates[0].IdentityKey); strings.TrimSpace(name) != "" {
			return name
		}
		if displayName := strings.TrimSpace(candidates[0].DisplayName); displayName != "" {
			return displayName
		}
	}
	return strings.TrimSpace(fallback)
}

func resolvedRuntimeModelConfig(cfg *config.Config, modelName string) (*config.ModelConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	return providers.ResolveModelConfig(cfg, strings.TrimSpace(modelName))
}

func resolveActiveModelConfig(
	cfg *config.Config,
	candidates []providers.FallbackCandidate,
	activeModel string,
) *config.ModelConfig {
	if cfg == nil {
		return nil
	}

	if len(candidates) > 0 {
		if modelCfg, err := resolvedCandidateModelConfig(cfg, candidates[0]); err == nil {
			return modelCfg
		}
		return nil
	}

	if mc := lookupModelConfigByRef(cfg, activeModel); mc != nil {
		clone := *mc
		return &clone
	}

	return nil
}
