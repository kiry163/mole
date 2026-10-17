package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/logger"
	"github.com/kiry163/mole/pkg/providers"
)

// registerModelRoutes binds model list management endpoints to the ServeMux.
func (h *Handler) registerModelRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/models", h.handleListModels)
	mux.HandleFunc("GET /api/models/default-chain", h.handleGetDefaultChain)
	mux.HandleFunc("POST /api/models/fetch", h.handleFetchModels)
	mux.HandleFunc("GET /api/models/catalog", h.handleListCatalogs)
	mux.HandleFunc("DELETE /api/models/catalog/{id}", h.handleDeleteCatalog)
	mux.HandleFunc("POST /api/models", h.handleAddModel)
	mux.HandleFunc("POST /api/models/default", h.handleSetDefaultModel)
	mux.HandleFunc("PUT /api/models/default-chain", h.handleUpdateDefaultChain)
	mux.HandleFunc("PUT /api/models/{index}", h.handleUpdateModel)
	mux.HandleFunc("DELETE /api/models/{index}", h.handleDeleteModel)
	mux.HandleFunc("POST /api/models/{index}/test", h.handleTestModel)
	mux.HandleFunc("POST /api/models/test-inline", h.handleTestInlineModel)
}

// modelResponse is the JSON structure returned for each model in the list.
// All ModelConfig fields are included so the frontend can display and edit them.
type modelResponse struct {
	Index     int    `json:"index"`
	ModelName string `json:"model_name"`
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model"`
	APIBase   string `json:"api_base,omitempty"`
	APIKey    string `json:"api_key"`
	// Advanced fields
	RPM                 int                         `json:"rpm,omitempty"`
	MaxTokensField      string                      `json:"max_tokens_field,omitempty"`
	RequestTimeout      int                         `json:"request_timeout,omitempty"`
	ThinkingLevel       string                      `json:"thinking_level,omitempty"`
	ToolSchemaTransform string                      `json:"tool_schema_transform,omitempty"`
	Streaming           config.ModelStreamingConfig `json:"streaming,omitempty"`
	ExtraBody           map[string]any              `json:"extra_body,omitempty"`
	CustomHeaders       map[string]string           `json:"custom_headers,omitempty"`
	// Meta
	Enabled   bool   `json:"enabled"`
	Available bool   `json:"available"`
	Status    string `json:"status"`
	IsDefault bool   `json:"is_default"`
	IsVirtual bool   `json:"is_virtual"`
}

type defaultChainResponse struct {
	DefaultModel  string   `json:"default_model"`
	FallbackChain []string `json:"fallback_chain"`
}

type defaultChainRequest struct {
	DefaultModel  string   `json:"default_model"`
	FallbackChain []string `json:"fallback_chain"`
}

func normalizeStoredModelConfig(mc *config.ModelConfig) bool {
	if mc == nil {
		return false
	}

	changed := false
	modelName := strings.TrimSpace(mc.ModelName)
	if modelName != mc.ModelName {
		mc.ModelName = modelName
		changed = true
	}
	model := strings.TrimSpace(mc.Model)
	if model != mc.Model {
		mc.Model = model
		changed = true
	}
	provider := providers.NormalizeProvider(mc.Provider)
	if provider != mc.Provider {
		mc.Provider = provider
		changed = true
	}
	return changed
}

func normalizeIncomingModelConfig(mc *config.ModelConfig) {
	if mc == nil {
		return
	}
	mc.ModelName = strings.TrimSpace(mc.ModelName)
	mc.Model = strings.TrimSpace(mc.Model)
	mc.Provider = providers.NormalizeProvider(mc.Provider)
	mc.APIBase = strings.TrimRight(strings.TrimSpace(mc.APIBase), "/")
}

func validateIncomingModelConfig(mc *config.ModelConfig) error {
	if mc == nil {
		return fmt.Errorf("model config is required")
	}
	if err := mc.Validate(); err != nil {
		return err
	}
	if _, err := url.ParseRequestURI(mc.APIBase); err != nil {
		return fmt.Errorf("api_base is invalid: %w", err)
	}
	parsed, err := url.Parse(mc.APIBase)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("api_base must be an absolute http(s) URL")
	}
	return nil
}

func normalizeStoredModels(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}

	changed := false
	for _, model := range cfg.ModelList {
		if normalizeStoredModelConfig(model) {
			changed = true
		}
	}
	return changed
}

func normalizeStoredModelNames(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	changed := false
	for _, model := range cfg.ModelList {
		if model == nil {
			continue
		}
		modelName := strings.TrimSpace(model.ModelName)
		if modelName != model.ModelName {
			model.ModelName = modelName
			changed = true
		}
	}
	return changed
}

func trimModelNameList(names []string) []string {
	trimmed := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		trimmed = append(trimmed, name)
	}
	return trimmed
}

func dedupeModelNameList(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(names))
	deduped := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		deduped = append(deduped, name)
	}
	return deduped
}

func modelConfigsByName(cfg *config.Config) map[string][]*config.ModelConfig {
	models := make(map[string][]*config.ModelConfig, len(cfg.ModelList))
	for _, model := range cfg.ModelList {
		if model == nil {
			continue
		}
		name := strings.TrimSpace(model.ModelName)
		if name == "" {
			continue
		}
		models[name] = append(models[name], model)
	}
	return models
}

func modelConfigsByNameExceptIndex(
	cfg *config.Config,
	modelName string,
	exceptIndex int,
) []*config.ModelConfig {
	modelName = strings.TrimSpace(modelName)
	if cfg == nil || modelName == "" {
		return nil
	}
	models := make([]*config.ModelConfig, 0, 1)
	for i, model := range cfg.ModelList {
		if i == exceptIndex || model == nil {
			continue
		}
		if strings.TrimSpace(model.ModelName) == modelName {
			models = append(models, model)
		}
	}
	return models
}

func validateDefaultChainModelSet(
	modelName string,
	models []*config.ModelConfig,
	defaultContext bool,
) error {
	if len(models) == 0 {
		if defaultContext {
			return fmt.Errorf("default model %q not found in model_list", modelName)
		}
		return fmt.Errorf("fallback model %q not found in model_list", modelName)
	}
	for _, modelCfg := range models {
		if modelCfg.IsVirtual() {
			if defaultContext {
				return fmt.Errorf("cannot set virtual model %q as default", modelName)
			}
			return fmt.Errorf("cannot add virtual model %q to fallback_chain", modelName)
		}
		if !hasModelConfiguration(modelCfg) {
			if defaultContext {
				return fmt.Errorf("default model %q has no api_base configured", modelName)
			}
			return fmt.Errorf("fallback model %q has no api_base configured", modelName)
		}
	}
	return nil
}

func validateDefaultChainReference(
	cfg *config.Config,
	modelName string,
	models []*config.ModelConfig,
	defaultContext bool,
) error {
	if len(models) == 0 {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			return fmt.Errorf("model is required")
		}
		if defaultContext {
			return fmt.Errorf("default model %q not found in model_list", modelName)
		}
		return fmt.Errorf("fallback model %q not found in model_list", modelName)
	}
	return validateDefaultChainModelSet(modelName, models, defaultContext)
}

func defaultChainReferenceIdentity(modelName string) string {
	return strings.TrimSpace(modelName)
}

func validateDefaultModelChain(
	cfg *config.Config,
	defaultModel string,
	fallbacks []string,
) ([]string, error) {
	defaultModel = strings.TrimSpace(defaultModel)
	fallbacks = dedupeModelNameList(trimModelNameList(fallbacks))

	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if defaultModel == "" && len(fallbacks) > 0 {
		return nil, fmt.Errorf("default_model is required when fallback_chain is not empty")
	}

	modelsByName := modelConfigsByName(cfg)
	defaultIdentity := ""
	if defaultModel != "" {
		if err := validateDefaultChainReference(
			cfg,
			defaultModel,
			modelsByName[defaultModel],
			true,
		); err != nil {
			return nil, err
		}
		defaultIdentity = defaultChainReferenceIdentity(defaultModel)
	}

	seenIdentities := make(map[string]struct{}, len(fallbacks))
	validated := make([]string, 0, len(fallbacks))
	for _, fallback := range fallbacks {
		if fallback == defaultModel {
			return nil, fmt.Errorf("default model %q cannot also appear in fallback_chain", defaultModel)
		}
		if err := validateDefaultChainReference(cfg, fallback, modelsByName[fallback], false); err != nil {
			return nil, err
		}
		identity := defaultChainReferenceIdentity(fallback)
		if identity != "" && identity == defaultIdentity {
			return nil, fmt.Errorf(
				"fallback model %q resolves to the same runtime model as default model %q",
				fallback,
				defaultModel,
			)
		}
		if _, exists := seenIdentities[identity]; identity != "" && exists {
			continue
		}
		if identity != "" {
			seenIdentities[identity] = struct{}{}
		}
		validated = append(validated, fallback)
	}

	return validated, nil
}

func normalizeDefaultChainReferences(cfg *config.Config) {
	if cfg == nil {
		return
	}

	defaultModel := strings.TrimSpace(cfg.Agents.Defaults.ModelName)
	fallbacks := dedupeModelNameList(trimModelNameList(cfg.Agents.Defaults.ModelFallbacks))
	modelsByName := modelConfigsByName(cfg)
	defaultIdentity := ""

	if defaultModel != "" {
		if err := validateDefaultChainReference(
			cfg,
			defaultModel,
			modelsByName[defaultModel],
			true,
		); err != nil {
			defaultModel = ""
		}
		defaultIdentity = defaultChainReferenceIdentity(defaultModel)
	}
	if defaultModel == "" {
		cfg.Agents.Defaults.ModelName = ""
		cfg.Agents.Defaults.ModelFallbacks = nil
		return
	}

	seenIdentities := make(map[string]struct{}, len(fallbacks))
	validated := make([]string, 0, len(fallbacks))
	for _, fallback := range fallbacks {
		if fallback == defaultModel {
			continue
		}
		if err := validateDefaultChainReference(cfg, fallback, modelsByName[fallback], false); err != nil {
			continue
		}
		identity := defaultChainReferenceIdentity(fallback)
		if identity != "" && identity == defaultIdentity {
			continue
		}
		if _, exists := seenIdentities[identity]; identity != "" && exists {
			continue
		}
		if identity != "" {
			seenIdentities[identity] = struct{}{}
		}
		validated = append(validated, fallback)
	}

	cfg.Agents.Defaults.ModelName = defaultModel
	cfg.Agents.Defaults.ModelFallbacks = validated
}

// handleListModels returns all model_list entries with masked API keys.
//
//	GET /api/models
func (h *Handler) handleListModels(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	// Normalize stored provider/model fields in memory so GET can round-trip
	// through the current API shape without mutating the on-disk config.
	normalizeStoredModels(cfg)
	normalizeDefaultChainReferences(cfg)

	defaultModel := cfg.Agents.Defaults.GetModelName()
	fallbackChain := cfg.Agents.Defaults.ModelFallbacks
	modelStatuses := make([]modelConfigurationSummary, len(cfg.ModelList))

	var wg sync.WaitGroup
	wg.Add(len(cfg.ModelList))
	for i, m := range cfg.ModelList {
		go func(i int, m *config.ModelConfig) {
			defer wg.Done()
			modelStatuses[i] = modelConfigurationStatus(m)
		}(i, m)
	}
	wg.Wait()

	models := make([]modelResponse, 0, len(cfg.ModelList))
	for i, m := range cfg.ModelList {
		provider, modelID := providers.ExtractProtocol(m)
		models = append(models, modelResponse{
			Index:               i,
			ModelName:           m.ModelName,
			Provider:            provider,
			Model:               modelID,
			APIBase:             m.APIBase,
			APIKey:              maskAPIKey(m.APIKey()),
			RPM:                 m.RPM,
			MaxTokensField:      m.MaxTokensField,
			RequestTimeout:      m.RequestTimeout,
			ThinkingLevel:       m.ThinkingLevel,
			ToolSchemaTransform: m.ToolSchemaTransform,
			Streaming:           m.Streaming,
			ExtraBody:           m.ExtraBody,
			CustomHeaders:       m.CustomHeaders,
			Enabled:             m.Enabled,
			Available:           modelStatuses[i].Available,
			Status:              modelStatuses[i].Status,
			IsDefault:           m.ModelName == defaultModel,
			IsVirtual:           m.IsVirtual(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"models":         models,
		"total":          len(models),
		"default_model":  defaultModel,
		"fallback_chain": fallbackChain,
	})
}

// handleGetDefaultChain returns the persisted default model and fallback chain.
//
//	GET /api/models/default-chain
func (h *Handler) handleGetDefaultChain(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}
	normalizeStoredModels(cfg)
	normalizeDefaultChainReferences(cfg)

	resp := defaultChainResponse{
		DefaultModel:  strings.TrimSpace(cfg.Agents.Defaults.GetModelName()),
		FallbackChain: trimModelNameList(cfg.Agents.Defaults.ModelFallbacks),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleAddModel appends a new model configuration entry.
//
//	POST /api/models
func (h *Handler) handleAddModel(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	type custom struct {
		config.ModelConfig
		APIKey string `json:"api_key"`
	}

	var mc custom
	if err = json.Unmarshal(body, &mc); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	if mc.APIKey != "" {
		mc.ModelConfig.SetAPIKey(mc.APIKey)
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}
	normalizeIncomingModelConfig(&mc.ModelConfig)
	if err = validateIncomingModelConfig(&mc.ModelConfig); err != nil {
		http.Error(w, fmt.Sprintf("Validation error: %v", err), http.StatusBadRequest)
		return
	}

	cfg.ModelList = append(cfg.ModelList, &mc.ModelConfig)
	normalizeStoredModels(cfg)
	normalizeDefaultChainReferences(cfg)

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	// The gateway already read the model list at startup: ask it to reload so
	// the change takes effect without a restart.
	h.requestReload()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"index":  len(cfg.ModelList) - 1,
	})
}

// handleUpdateModel replaces a model configuration entry at the given index.
// If the request body omits api_key (or sends an empty string), the existing
// stored key is preserved so callers can update only api_base without
// exposing or clearing the secret.
//
//	PUT /api/models/{index}
func (h *Handler) handleUpdateModel(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		http.Error(w, "Invalid index", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var rawFields map[string]json.RawMessage
	if err = json.Unmarshal(body, &rawFields); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	type custom struct {
		config.ModelConfig
		APIKey string `json:"api_key"`
	}

	var mc custom
	if err = json.Unmarshal(body, &mc); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}
	normalizeStoredModels(cfg)

	if idx < 0 || idx >= len(cfg.ModelList) {
		http.Error(w, fmt.Sprintf("Index %d out of range (0-%d)", idx, len(cfg.ModelList)-1), http.StatusNotFound)
		return
	}
	oldModelName := strings.TrimSpace(cfg.ModelList[idx].ModelName)

	// Preserve the existing API key when the caller omits it (empty string).
	// This lets the UI update api_base without clearing the stored secret.
	if mc.APIKey == "" {
		mc.ModelConfig.SetAPIKey(cfg.ModelList[idx].APIKey())
	} else {
		mc.ModelConfig.SetAPIKey(mc.APIKey)
	}
	// Preserve existing ExtraBody when omitted (nil), but clear it when
	// the frontend sends an empty object {} to indicate the field should
	// be removed.
	if mc.ExtraBody == nil {
		mc.ExtraBody = cfg.ModelList[idx].ExtraBody
	} else if len(mc.ExtraBody) == 0 {
		mc.ExtraBody = nil
	}
	// Preserve existing CustomHeaders when omitted (nil), but clear it when
	// the frontend sends an empty object {} to indicate the field should
	// be removed.
	if mc.CustomHeaders == nil {
		mc.CustomHeaders = cfg.ModelList[idx].CustomHeaders
	} else if len(mc.CustomHeaders) == 0 {
		mc.CustomHeaders = nil
	}
	if _, ok := rawFields["tool_schema_transform"]; !ok {
		mc.ToolSchemaTransform = cfg.ModelList[idx].ToolSchemaTransform
	}
	if _, ok := rawFields["streaming"]; !ok {
		mc.Streaming = cfg.ModelList[idx].Streaming
	}
	// Preserve the existing Provider when the caller omits it, while still
	// allowing explicit clearing via "".
	if _, ok := rawFields["provider"]; !ok {
		mc.Provider = cfg.ModelList[idx].Provider
	}

	normalizeIncomingModelConfig(&mc.ModelConfig)
	if err = validateIncomingModelConfig(&mc.ModelConfig); err != nil {
		http.Error(w, fmt.Sprintf("Validation error: %v", err), http.StatusBadRequest)
		return
	}
	newModelName := strings.TrimSpace(mc.ModelConfig.ModelName)
	defaultReferenceRenamed := false
	fallbackReferenceRenamed := false
	if oldModelName != "" && newModelName != "" && oldModelName != newModelName {
		remainingOldNameModels := modelConfigsByNameExceptIndex(cfg, oldModelName, idx)
		if err := validateDefaultChainModelSet(
			oldModelName,
			remainingOldNameModels,
			true,
		); err != nil {
			defaultReferenceRenamed = true
			if strings.TrimSpace(cfg.Agents.Defaults.ModelName) == oldModelName {
				cfg.Agents.Defaults.ModelName = newModelName
			}
		}
		if err := validateDefaultChainModelSet(
			oldModelName,
			remainingOldNameModels,
			false,
		); err != nil {
			fallbackReferenceRenamed = true
			cfg.Agents.Defaults.ModelFallbacks = replaceModelName(
				cfg.Agents.Defaults.ModelFallbacks,
				oldModelName,
				newModelName,
			)
		}
	}
	cfg.ModelList[idx] = &mc.ModelConfig
	normalizeStoredModels(cfg)
	normalizeDefaultChainReferences(cfg)

	logger.Debugf("update model config: %#v", mc.ModelConfig)

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	// The gateway already read the model list at startup: ask it to reload so
	// the change takes effect without a restart.
	h.requestReload()

	w.Header().Set("Content-Type", "application/json")
	response := map[string]any{"status": "ok"}
	if oldModelName != "" && newModelName != "" && oldModelName != newModelName {
		response["reference_rename"] = map[string]any{
			"from":     oldModelName,
			"to":       newModelName,
			"default":  defaultReferenceRenamed,
			"fallback": fallbackReferenceRenamed,
		}
	}
	json.NewEncoder(w).Encode(response)
}

// handleDeleteModel removes a model configuration entry at the given index.
//
//	DELETE /api/models/{index}
func (h *Handler) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		http.Error(w, "Invalid index", http.StatusBadRequest)
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}
	// Restore plain-string refs before the slice shift: envRefs paths are
	// positional, so deleting an entry would make later paths point elsewhere.
	cfg.RestoreEnvRefs()
	normalizeStoredModelNames(cfg)

	if idx < 0 || idx >= len(cfg.ModelList) {
		http.Error(w, fmt.Sprintf("Index %d out of range (0-%d)", idx, len(cfg.ModelList)-1), http.StatusNotFound)
		return
	}

	deletedModelName := strings.TrimSpace(cfg.ModelList[idx].ModelName)

	cfg.ModelList = append(cfg.ModelList[:idx], cfg.ModelList[idx+1:]...)

	if deletedModelName != "" {
		remainingModels := modelConfigsByName(cfg)[deletedModelName]
		if strings.TrimSpace(cfg.Agents.Defaults.ModelName) == deletedModelName {
			if err := validateDefaultChainModelSet(
				deletedModelName,
				remainingModels,
				true,
			); err != nil {
				cfg.Agents.Defaults.ModelName = ""
			}
		}
		if err := validateDefaultChainModelSet(
			deletedModelName,
			remainingModels,
			false,
		); err != nil {
			cfg.Agents.Defaults.ModelFallbacks = removeModelName(
				cfg.Agents.Defaults.ModelFallbacks,
				deletedModelName,
			)
		}
	}
	normalizeDefaultChainReferences(cfg)

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	// The gateway already read the model list at startup: ask it to reload so
	// the change takes effect without a restart.
	h.requestReload()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func removeModelName(names []string, target string) []string {
	target = strings.TrimSpace(target)
	if target == "" || len(names) == 0 {
		return names
	}
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == target {
			continue
		}
		filtered = append(filtered, name)
	}
	return filtered
}

func replaceModelName(names []string, oldName string, newName string) []string {
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || oldName == newName || len(names) == 0 {
		return names
	}
	replaced := make([]string, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == oldName {
			replaced = append(replaced, newName)
			continue
		}
		replaced = append(replaced, name)
	}
	return replaced
}

// handleSetDefaultModel sets the default model for all agents.
//
//	POST /api/models/default
func (h *Handler) handleSetDefaultModel(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req struct {
		ModelName string `json:"model_name"`
	}
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	modelName := strings.TrimSpace(req.ModelName)
	if modelName == "" {
		http.Error(w, "model_name is required", http.StatusBadRequest)
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}
	normalizeStoredModelNames(cfg)

	// This endpoint only sets the default model alias. Raw references are
	// managed by the default-chain endpoint and may already exist in persisted
	// configuration.
	if err := validateDefaultChainModelSet(
		modelName,
		modelConfigsByName(cfg)[modelName],
		true,
	); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "not found in model_list") {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}

	cfg.Agents.Defaults.ModelName = modelName
	cfg.Agents.Defaults.ModelFallbacks = removeModelName(cfg.Agents.Defaults.ModelFallbacks, modelName)
	normalizeDefaultChainReferences(cfg)

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	// The gateway already read the model list at startup: ask it to reload so
	// the change takes effect without a restart.
	h.requestReload()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":        "ok",
		"default_model": modelName,
	})
}

// handleUpdateDefaultChain updates the persisted default model and fallback chain.
//
//	PUT /api/models/default-chain
func (h *Handler) handleUpdateDefaultChain(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req defaultChainRequest
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}
	normalizeStoredModelNames(cfg)

	defaultModel := strings.TrimSpace(req.DefaultModel)
	fallbacks, err := validateDefaultModelChain(cfg, defaultModel, req.FallbackChain)
	if err != nil {
		http.Error(w, fmt.Sprintf("Validation error: %v", err), http.StatusBadRequest)
		return
	}

	cfg.Agents.Defaults.ModelName = defaultModel
	cfg.Agents.Defaults.ModelFallbacks = fallbacks

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	// The gateway already read the model list at startup: ask it to reload so
	// the change takes effect without a restart.
	h.requestReload()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(defaultChainResponse{
		DefaultModel:  defaultModel,
		FallbackChain: fallbacks,
	})
}

// maskAPIKey returns a masked version of an API key for safe display.
// Keys longer than 12 chars show prefix + last 4 chars: "sk-****abcd".
// Keys 9-12 chars show prefix + last 2 chars: "sk-****cd".
// Shorter keys are fully masked as "****".
// Empty keys return empty string.
// Ensure at least 40% of the key will not be displayed.
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}

	if len(key) <= 8 {
		return "****"
	}

	// Show first 3 chars and last 2 chars
	if len(key) <= 12 {
		return key[:3] + "****" + key[len(key)-2:]
	}

	// Show first 3 chars and last 4 chars
	return key[:3] + "****" + key[len(key)-4:]
}

// handleFetchModels lists models from an OpenAI-compatible endpoint.
//
//	POST /api/models/fetch
func (h *Handler) handleFetchModels(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req struct {
		Provider   string `json:"provider"`
		APIKey     string `json:"api_key"`
		APIBase    string `json:"api_base"`
		ModelIndex *int   `json:"model_index,omitempty"`
	}
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	apiKey := strings.TrimSpace(req.APIKey)
	apiBase := strings.TrimRight(strings.TrimSpace(req.APIBase), "/")
	if apiBase == "" {
		http.Error(w, "api_base is required", http.StatusBadRequest)
		return
	}
	if req.ModelIndex != nil && apiKey == "" {
		apiKey = h.lookupStoredAPIKey(*req.ModelIndex, apiBase)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	models, err := fetchUpstreamModels(ctx, apiBase, apiKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch models: %v", err), http.StatusBadGateway)
		return
	}

	catalogModels := make([]CatalogModel, len(models))
	for i, m := range models {
		catalogModels[i] = CatalogModel{ID: m.ID, OwnedBy: m.OwnedBy}
	}
	provider := providers.NormalizeProvider(req.Provider)
	if provider == "" {
		provider = "custom"
	}
	if saveErr := SaveCatalog(provider, apiBase, apiKey, catalogModels); saveErr != nil {
		logger.Warnf("Failed to save model catalog: %v", saveErr)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"models": models,
		"total":  len(models),
	})
}

func (h *Handler) lookupStoredAPIKey(index int, reqAPIBase string) string {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil || index < 0 || index >= len(cfg.ModelList) {
		return ""
	}
	stored := cfg.ModelList[index]
	if normalizeAPIBaseForCompare(stored.APIBase) != normalizeAPIBaseForCompare(reqAPIBase) {
		return ""
	}
	return stored.APIKey()
}

type upstreamModel struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by,omitempty"`
}

func fetchUpstreamModels(ctx context.Context, apiBase, apiKey string) ([]upstreamModel, error) {
	fetchURL := strings.TrimRight(strings.TrimSpace(apiBase), "/") + "/models"
	return fetchOpenAICompatibleModels(ctx, fetchURL, apiKey)
}

func fetchOpenAICompatibleModels(ctx context.Context, fetchURL, apiKey string) ([]upstreamModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fetchURL, nil)
	if err != nil {
		return nil, err
	}
	if apiKey = strings.TrimSpace(apiKey); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var envelope struct {
		Data *[]upstreamModel `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Data != nil {
		return normalizeUpstreamModels(*envelope.Data), nil
	}

	var list []upstreamModel
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("unrecognized model list response: %w", err)
	}
	return normalizeUpstreamModels(list), nil
}

func normalizeUpstreamModels(models []upstreamModel) []upstreamModel {
	out := make([]upstreamModel, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, upstreamModel{ID: id, OwnedBy: strings.TrimSpace(model.OwnedBy)})
	}
	return out
}

func normalizeAPIBaseForCompare(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.TrimRight(raw, "/")
	u, err := url.Parse(raw)
	if err != nil {
		return strings.ToLower(raw)
	}
	if u.Host == "" {
		u, err = url.Parse("//" + raw)
		if err != nil {
			return strings.ToLower(raw)
		}
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.Path, "/")
}

// handleTestModel tests connectivity to a model endpoint.
//
//	POST /api/models/{index}/test
func (h *Handler) handleTestModel(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		http.Error(w, "Invalid index", http.StatusBadRequest)
		return
	}

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	if idx < 0 || idx >= len(cfg.ModelList) {
		http.Error(w, fmt.Sprintf("Index %d out of range (0-%d)", idx, len(cfg.ModelList)-1), http.StatusNotFound)
		return
	}

	m := cfg.ModelList[idx]
	start := time.Now()
	summary := modelConfigurationStatus(m)
	latency := time.Since(start).Milliseconds()

	result := map[string]any{
		"success":    summary.Available,
		"latency_ms": latency,
		"status":     summary.Status,
	}

	if !summary.Available {
		if summary.Status == modelStatusUnconfigured {
			result["error"] = "API key not configured"
		} else {
			result["error"] = "Endpoint unreachable"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleTestInlineModel tests connectivity using inline (unsaved) parameters.
// Unlike handleTestModel which only checks saved config, this endpoint performs
// a real network probe (e.g. GET /models) to verify the endpoint is reachable.
//
//	POST /api/models/test-inline
func (h *Handler) handleTestInlineModel(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var req struct {
		Provider   string `json:"provider"`
		Model      string `json:"model"`
		APIBase    string `json:"api_base"`
		APIKey     string `json:"api_key"`
		ModelIndex *int   `json:"model_index"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	m := &config.ModelConfig{
		Provider: strings.TrimSpace(req.Provider),
		Model:    strings.TrimSpace(req.Model),
		APIBase:  strings.TrimSpace(req.APIBase),
	}
	if req.APIKey != "" {
		m.SetAPIKey(req.APIKey)
	}

	// When api_key is empty and model_index is provided, fall back to stored
	// credentials. Only reuse the stored key when the effective API base
	// matches, to prevent attaching a credential to a different endpoint.
	if req.APIKey == "" && req.ModelIndex != nil {
		cfg, err := config.LoadConfig(h.configPath)
		if err == nil && *req.ModelIndex >= 0 && *req.ModelIndex < len(cfg.ModelList) {
			stored := cfg.ModelList[*req.ModelIndex]
			effectiveReqBase := strings.TrimSpace(m.APIBase)
			if effectiveReqBase == "" {
				effectiveReqBase = stored.APIBase
				m.APIBase = stored.APIBase
			}
			if normalizeAPIBaseForCompare(effectiveReqBase) == normalizeAPIBaseForCompare(stored.APIBase) {
				if stored.APIKey() != "" {
					m.SetAPIKey(stored.APIKey())
				}
			}
		}
	}

	// Check if configuration exists
	if !hasModelConfiguration(m) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"success":    false,
			"latency_ms": 0,
			"status":     modelStatusUnconfigured,
			"error":      "API key not configured",
		})
		return
	}

	// Perform a real network probe
	start := time.Now()
	available := probeModelConnectivity(m)
	latency := time.Since(start).Milliseconds()

	result := map[string]any{
		"success":    available,
		"latency_ms": latency,
	}
	if available {
		result["status"] = modelStatusAvailable
	} else {
		result["status"] = modelStatusUnreachable
		result["error"] = "Endpoint unreachable"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// probeModelConnectivity performs a real network probe to verify model endpoint reachability.
func probeModelConnectivity(m *config.ModelConfig) bool {
	apiBase := modelProbeAPIBase(m)
	_, modelID := providers.ExtractProtocol(m)
	if apiBase == "" {
		return false
	}
	return probeOpenAICompatibleModel(apiBase, modelID, m.APIKey())
}
