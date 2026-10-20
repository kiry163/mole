package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kiry163/mole/pkg/config"
)

func resetModelProbeHooks(t *testing.T) {
	t.Helper()

	origOpenAIProbe := probeOpenAICompatibleModelFunc
	origNow := modelProbeNowFunc
	resetModelProbeCache()
	t.Cleanup(func() {
		probeOpenAICompatibleModelFunc = origOpenAIProbe
		modelProbeNowFunc = origNow
		resetModelProbeCache()
	})
}

func writeModelChainConfig(
	t *testing.T,
	configPath string,
	models []*config.ModelConfig,
	defaultModel string,
	fallbacks []string,
) {
	t.Helper()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = models
	cfg.Agents.Defaults.ModelName = defaultModel
	cfg.Agents.Defaults.ModelFallbacks = fallbacks
	if err = config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
}

func updateModelAndLoadConfig(
	t *testing.T,
	configPath string,
	index int,
	body string,
) *config.Config {
	t.Helper()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPut,
		fmt.Sprintf("/api/models/%d", index),
		bytes.NewBufferString(body),
	)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	return cfg
}

func deleteModelAndLoadConfig(t *testing.T, configPath string, index int) *config.Config {
	t.Helper()

	h := NewHandler(configPath)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/models/%d", index), nil)
	req.SetPathValue("index", fmt.Sprintf("%d", index))
	h.handleDeleteModel(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	return cfg
}

func setDefaultModelAndLoadConfig(t *testing.T, configPath string, modelName string) *config.Config {
	t.Helper()

	h := NewHandler(configPath)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/models/default",
		bytes.NewBufferString(fmt.Sprintf(`{"model_name":%q}`, modelName)),
	)
	req.Header.Set("Content-Type", "application/json")
	h.handleSetDefaultModel(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	return cfg
}

func assertDefaultChain(
	t *testing.T,
	cfg *config.Config,
	wantDefault string,
	wantFallbacks []string,
) {
	t.Helper()

	if got := cfg.Agents.Defaults.ModelName; got != wantDefault {
		t.Fatalf("default model = %q, want %q", got, wantDefault)
	}
	if !slices.Equal(cfg.Agents.Defaults.ModelFallbacks, wantFallbacks) {
		t.Fatalf("fallback_chain = %#v, want %#v", cfg.Agents.Defaults.ModelFallbacks, wantFallbacks)
	}
}

func openAIModelConfig(alias string) *config.ModelConfig {
	return &config.ModelConfig{
		ModelName: alias,
		Provider:  "openai",
		Model:     "gpt-4o",
		APIBase:   "https://api.openai.com/v1",
		APIKeys:   config.SimpleSecureStrings("sk-openai"),
	}
}

func anthropicModelConfig(alias string) *config.ModelConfig {
	return &config.ModelConfig{
		ModelName: alias,
		Provider:  "anthropic",
		Model:     "claude-sonnet-4.6",
		APIBase:   "https://api.example.com/v1",
		APIKeys:   config.SimpleSecureStrings("sk-anthropic"),
	}
}

func deepseekModelConfig(alias string) *config.ModelConfig {
	return &config.ModelConfig{
		ModelName: alias,
		Provider:  "deepseek",
		Model:     "deepseek-chat",
		APIBase:   "https://api.deepseek.com/v1",
		APIKeys:   config.SimpleSecureStrings("sk-deepseek"),
	}
}

func elevenLabsModelConfig(alias string) *config.ModelConfig {
	return &config.ModelConfig{
		ModelName: alias,
		Provider:  "elevenlabs",
		Model:     "scribe_v1",
		APIBase:   "https://api.elevenlabs.io",
		APIKeys:   config.SimpleSecureStrings("sk-elevenlabs"),
	}
}

func resetModelProbeCache() {
	modelProbeState.resetForTest()
}

func TestHandleListModels_AvailabilityUsesRuntimeProbesForLocalModels(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()
	resetModelProbeHooks(t)

	var mu sync.Mutex
	var openAIProbes []string

	probeOpenAICompatibleModelFunc = func(apiBase, modelID, apiKey string) bool {
		mu.Lock()
		openAIProbes = append(openAIProbes, apiBase+"|"+modelID+"|"+apiKey)
		mu.Unlock()
		return apiBase == "http://127.0.0.1:8000/v1" && modelID == "custom-model" && apiKey == ""
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{
		{
			ModelName: "openai-no-key",
			Provider:  "openai",
			Model:     "gpt-5.4",
		},
		{
			ModelName: "vllm-local",
			Provider:  "vllm",
			Model:     "custom-model",
			APIBase:   "http://127.0.0.1:8000/v1",
		},
		{
			ModelName: "vllm-remote",
			Provider:  "vllm",
			Model:     "custom-model",
			APIBase:   "https://models.example.com/v1",
			APIKeys:   config.SimpleSecureStrings("remote-key"),
		},
	}
	cfg.Agents.Defaults.ModelName = "openai-no-key"
	err = config.SaveConfig(configPath, cfg)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Models []modelResponse `json:"models"`
	}
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	gotAvailable := make(map[string]bool, len(resp.Models))
	gotStatus := make(map[string]string, len(resp.Models))
	for _, model := range resp.Models {
		gotAvailable[model.ModelName] = model.Available
		gotStatus[model.ModelName] = model.Status
	}

	if gotAvailable["openai-no-key"] {
		t.Fatalf("openai model without credentials available = true, want false")
	}
	if !gotAvailable["vllm-local"] {
		t.Fatalf("vllm local model available = false, want true when local probe succeeds")
	}
	if !gotAvailable["vllm-remote"] {
		t.Fatalf("remote vllm model available = false, want true with api_key")
	}
	if gotStatus["openai-no-key"] != modelStatusUnconfigured {
		t.Fatalf("openai model status = %q, want %q", gotStatus["openai-no-key"], modelStatusUnconfigured)
	}
	if gotStatus["vllm-local"] != modelStatusAvailable {
		t.Fatalf("vllm local model status = %q, want %q", gotStatus["vllm-local"], modelStatusAvailable)
	}
	if gotStatus["vllm-remote"] != modelStatusAvailable {
		t.Fatalf("remote vllm model status = %q, want %q", gotStatus["vllm-remote"], modelStatusAvailable)
	}
	if len(openAIProbes) != 1 || openAIProbes[0] != "http://127.0.0.1:8000/v1|custom-model|" {
		t.Fatalf("openAI probes = %#v, want only local vllm probe", openAIProbes)
	}
}

// TestHasModelConfiguration_WithoutKeyOrBase verifies that only an api_key or a
// non-default api_base configures a model entry.
func TestHasModelConfiguration_RequiresAPIBase(t *testing.T) {
	withoutBase := &config.ModelConfig{
		Provider: "deepseek",
		Model:    "deepseek-chat",
		APIKeys:  config.SimpleSecureStrings("deepseek-key"),
	}
	if hasModelConfiguration(withoutBase) {
		t.Fatal("model without api_base should be unconfigured")
	}

	withBase := &config.ModelConfig{
		Provider: "local",
		Model:    "custom-model",
		APIBase:  "http://localhost:8000/v1",
	}
	if !hasModelConfiguration(withBase) {
		t.Fatal("model with api_base should be configured")
	}
}

func TestHandleListModels_ProbesLocalModelsConcurrently(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()
	resetModelProbeHooks(t)

	started := make(chan string, 2)
	release := make(chan struct{})

	probeOpenAICompatibleModelFunc = func(apiBase, modelID, apiKey string) bool {
		started <- apiBase + "|" + modelID
		<-release
		return true
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{
		{
			ModelName: "local-vllm-a",
			Model:     "vllm/custom-a",
			APIBase:   "http://127.0.0.1:8000/v1",
		},
		{
			ModelName: "local-vllm-b",
			Model:     "vllm/custom-b",
			APIBase:   "http://127.0.0.1:8001/v1",
		},
	}
	err = config.SaveConfig(configPath, cfg)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	recCh := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
		mux.ServeHTTP(rec, req)
		recCh <- rec
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(200 * time.Millisecond):
			t.Fatal("expected both local probes to start before the first one completed")
		}
	}
	close(release)

	rec := <-recCh
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandleListModels_NormalizesWildcardLocalAPIBaseForProbe(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()
	resetModelProbeHooks(t)

	var gotProbe string
	probeOpenAICompatibleModelFunc = func(apiBase, modelID, apiKey string) bool {
		gotProbe = apiBase + "|" + modelID + "|" + apiKey
		return apiBase == "http://127.0.0.1:8000/v1" && modelID == "custom-model" && apiKey == ""
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{{
		ModelName: "vllm-local",
		Provider:  "vllm",
		Model:     "custom-model",
		APIBase:   "http://0.0.0.0:8000/v1",
	}}

	err = config.SaveConfig(configPath, cfg)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Models []modelResponse `json:"models"`
	}
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(resp.Models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(resp.Models))
	}
	if !resp.Models[0].Available {
		t.Fatal("wildcard-bound local model available = false, want true after probe host normalization")
	}
	if gotProbe != "http://127.0.0.1:8000/v1|custom-model|" {
		t.Fatalf("probe api base = %q, want %q", gotProbe, "http://127.0.0.1:8000/v1|custom-model|")
	}
}

func TestHandleListModels_StatusMarksUnreachableLocalModel(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()
	resetModelProbeHooks(t)

	probeOpenAICompatibleModelFunc = func(apiBase, modelID, apiKey string) bool {
		return false
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{{
		ModelName: "vllm-local-down",
		Model:     "vllm/custom-model",
		APIBase:   "http://127.0.0.1:8000/v1",
		APIKeys:   config.SimpleSecureStrings("test-key"),
	}}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Models []modelResponse `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(resp.Models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(resp.Models))
	}

	if resp.Models[0].Available {
		t.Fatal("unreachable local model available = true, want false")
	}
	if resp.Models[0].Status != modelStatusUnreachable {
		t.Fatalf("unreachable local model status = %q, want %q", resp.Models[0].Status, modelStatusUnreachable)
	}
	if resp.Models[0].APIKey == "" {
		t.Fatal("masked API key preview should still be returned when API key is configured")
	}
}

func TestHandleListModels_RuntimeProbeUsesExplicitProviderField(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()
	resetModelProbeHooks(t)

	var gotProbe string
	probeOpenAICompatibleModelFunc = func(apiBase, modelID, apiKey string) bool {
		gotProbe = apiBase + "|" + modelID + "|" + apiKey
		return true
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{{
		ModelName: "vllm-local",
		Provider:  "vllm",
		Model:     "custom-model",
		APIBase:   "http://127.0.0.1:8000/v1",
	}}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if gotProbe != "http://127.0.0.1:8000/v1|custom-model|" {
		t.Fatalf("probe = %q, want %q", gotProbe, "http://127.0.0.1:8000/v1|custom-model|")
	}
}

func TestHandleAddModel_PersistsAPIKey(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models", bytes.NewBufferString(`{
		"provider":"openai","model_name":"new-model",
		"api_base":"https://example.test/v1","model":"openai/gpt-4o-mini",
		"api_key":"sk-new-model-key"
	}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(cfg.ModelList) != 2 {
		t.Fatalf("len(model_list) = %d, want 2", len(cfg.ModelList))
	}

	added := cfg.ModelList[1]
	if added.ModelName != "new-model" {
		t.Fatalf("model_name = %q, want %q", added.ModelName, "new-model")
	}
	if added.APIKey() != "sk-new-model-key" {
		t.Fatalf("api_key = %q, want %q", added.APIKey(), "sk-new-model-key")
	}
}

func TestHandleAddModel_PersistsProvider(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models", bytes.NewBufferString(`{
		"model_name":"nvidia-glm",
		"provider":"nvidia",
		"api_base": "https://example.test/v1", "model":"z-ai/glm-5.1",
		"api_key":"nv-key"
	}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	added := cfg.ModelList[len(cfg.ModelList)-1]
	if added.Provider != "nvidia" {
		t.Fatalf("provider = %q, want %q", added.Provider, "nvidia")
	}
	if added.Model != "z-ai/glm-5.1" {
		t.Fatalf("model = %q, want %q", added.Model, "z-ai/glm-5.1")
	}
}

func TestHandleAddModel_TrimsModelName(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models", bytes.NewBufferString(`{
		"model_name":"  trimmed-alias  ",
		"provider":"openai",
		"api_base": "https://example.test/v1", "model":"gpt-4o",
		"api_key":"test-key"
	}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := cfg.ModelList[len(cfg.ModelList)-1].ModelName; got != "trimmed-alias" {
		t.Fatalf("model_name = %q, want trimmed-alias", got)
	}
}

func TestHandleListModels_ReturnsStreamingConfig(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{{
		ModelName: "streaming-model",
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		APIKeys:   config.SimpleSecureStrings("sk-existing"),
		Streaming: config.ModelStreamingConfig{Enabled: true},
	}}
	if err = config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Models []modelResponse `json:"models"`
	}
	if err = json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(resp.Models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(resp.Models))
	}
	if !resp.Models[0].Streaming.Enabled {
		t.Fatal("streaming.enabled = false, want true")
	}
}

func TestHandleAddModel_PreservesExplicitProviderPrefixedModel(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models", bytes.NewBufferString(`{
		"model_name":"openai-gpt",
		"provider":"openai",
		"api_base": "https://example.test/v1", "model":"openai/gpt-4o-mini",
		"api_key":"sk-openai"
	}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	added := cfg.ModelList[len(cfg.ModelList)-1]
	if got := added.Provider; got != "openai" {
		t.Fatalf("provider = %q, want %q", got, "openai")
	}
	if got := added.Model; got != "openai/gpt-4o-mini" {
		t.Fatalf("model = %q, want %q", got, "openai/gpt-4o-mini")
	}
}

func TestHandleAddModel_PersistsCustomHeaders(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models", bytes.NewBufferString(`{
		"provider":"openai","model_name":"new-model-headers",
		"api_base":"https://example.test/v1","model":"openai/gpt-4o-mini",
		"custom_headers":{"X-Source":"coding-plan","X-Agent":"openclaw"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(cfg.ModelList) != 2 {
		t.Fatalf("len(model_list) = %d, want 2", len(cfg.ModelList))
	}

	added := cfg.ModelList[1]
	if added.CustomHeaders == nil {
		t.Fatal("custom_headers should not be nil")
	}
	if got := added.CustomHeaders["X-Source"]; got != "coding-plan" {
		t.Fatalf("custom_headers[X-Source] = %q, want %q", got, "coding-plan")
	}
	if got := added.CustomHeaders["X-Agent"]; got != "openclaw" {
		t.Fatalf("custom_headers[X-Agent] = %q, want %q", got, "openclaw")
	}
}

func TestHandleUpdateModel_CustomHeadersPreserveAndClear(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{{
		ModelName:     "editable",
		Model:         "openai/gpt-4o-mini",
		APIKeys:       config.SimpleSecureStrings("sk-existing"),
		CustomHeaders: map[string]string{"X-Source": "coding-plan"},
	}}
	err = config.SaveConfig(configPath, cfg)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Omitted custom_headers should preserve existing value.
	recPreserve := httptest.NewRecorder()
	reqPreserve := httptest.NewRequest(http.MethodPut, "/api/models/0", bytes.NewBufferString(`{
		"provider":"openai","model_name":"editable",
		"api_base":"https://example.test/v1","model":"openai/gpt-4o-mini"
	}`))
	reqPreserve.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(recPreserve, reqPreserve)
	if recPreserve.Code != http.StatusOK {
		t.Fatalf("preserve status = %d, want %d, body=%s", recPreserve.Code, http.StatusOK, recPreserve.Body.String())
	}

	afterPreserve, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() after preserve error = %v", err)
	}
	if got := afterPreserve.ModelList[0].CustomHeaders["X-Source"]; got != "coding-plan" {
		t.Fatalf("preserved custom_headers[X-Source] = %q, want %q", got, "coding-plan")
	}

	// Empty object should clear custom_headers.
	recClear := httptest.NewRecorder()
	reqClear := httptest.NewRequest(http.MethodPut, "/api/models/0", bytes.NewBufferString(`{
		"provider":"openai","model_name":"editable",
		"api_base":"https://example.test/v1","model":"openai/gpt-4o-mini",
		"custom_headers":{}
	}`))
	reqClear.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(recClear, reqClear)
	if recClear.Code != http.StatusOK {
		t.Fatalf("clear status = %d, want %d, body=%s", recClear.Code, http.StatusOK, recClear.Body.String())
	}

	afterClear, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() after clear error = %v", err)
	}
	if afterClear.ModelList[0].CustomHeaders != nil {
		t.Fatalf("custom_headers = %#v, want nil", afterClear.ModelList[0].CustomHeaders)
	}
}

func TestHandleUpdateModel_StreamingPreserveAndChange(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{{
		ModelName: "editable",
		Provider:  "openai",
		Model:     "gpt-4o-mini",
		APIKeys:   config.SimpleSecureStrings("sk-existing"),
		Streaming: config.ModelStreamingConfig{Enabled: true},
	}}
	if err = config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	recPreserve := httptest.NewRecorder()
	reqPreserve := httptest.NewRequest(http.MethodPut, "/api/models/0", bytes.NewBufferString(`{
		"model_name":"editable",
		"provider":"openai",
		"api_base": "https://example.test/v1", "model":"gpt-4o-mini"
	}`))
	reqPreserve.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(recPreserve, reqPreserve)
	if recPreserve.Code != http.StatusOK {
		t.Fatalf("preserve status = %d, want %d, body=%s", recPreserve.Code, http.StatusOK, recPreserve.Body.String())
	}

	afterPreserve, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() after preserve error = %v", err)
	}
	if !afterPreserve.ModelList[0].Streaming.Enabled {
		t.Fatal("preserved streaming.enabled = false, want true")
	}

	recChange := httptest.NewRecorder()
	reqChange := httptest.NewRequest(http.MethodPut, "/api/models/0", bytes.NewBufferString(`{
		"model_name":"editable",
		"provider":"openai",
		"api_base": "https://example.test/v1", "model":"gpt-4o-mini",
		"streaming":{"enabled":false}
	}`))
	reqChange.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(recChange, reqChange)
	if recChange.Code != http.StatusOK {
		t.Fatalf("change status = %d, want %d, body=%s", recChange.Code, http.StatusOK, recChange.Body.String())
	}

	afterChange, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() after change error = %v", err)
	}
	if afterChange.ModelList[0].Streaming.Enabled {
		t.Fatal("streaming.enabled = true, want false after explicit update")
	}
}

func TestHandleUpdateModel_PersistsProvider(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{{
		ModelName: "editable",
		Model:     "gpt-4o",
		Provider:  "openai",
	}}
	if err = config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/models/0", bytes.NewBufferString(`{
		"model_name":"editable",
		"provider":"openrouter",
		"api_base": "https://example.test/v1", "model":"openai/gpt-4o"
	}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := updated.ModelList[0].Provider; got != "openrouter" {
		t.Fatalf("provider = %q, want %q", got, "openrouter")
	}
}

func TestHandleUpdateModel_PreservesExplicitProviderPrefixedModel(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{{
		ModelName: "editable",
		Model:     "gpt-4o",
		Provider:  "openai",
	}}
	if err = config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/models/0", bytes.NewBufferString(`{
		"model_name":"editable",
		"provider":"openai",
		"api_base": "https://example.test/v1", "model":"openai/gpt-5.4"
	}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := updated.ModelList[0].Provider; got != "openai" {
		t.Fatalf("provider = %q, want %q", got, "openai")
	}
	if got := updated.ModelList[0].Model; got != "openai/gpt-5.4" {
		t.Fatalf("model = %q, want %q", got, "openai/gpt-5.4")
	}
}

func TestHandleListModels_PreservesExplicitProviderPrefixedModel(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{{
		ModelName: "openrouter-auto-explicit",
		Provider:  "openrouter",
		Model:     "openrouter/auto",
	}}
	err = config.SaveConfig(configPath, cfg)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Models []modelResponse `json:"models"`
	}
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(resp.Models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(resp.Models))
	}
	if got := resp.Models[0].Provider; got != "openrouter" {
		t.Fatalf("provider = %q, want %q", got, "openrouter")
	}
	if got := resp.Models[0].Model; got != "openrouter/auto" {
		t.Fatalf("model = %q, want %q", got, "openrouter/auto")
	}
}

func TestHandleListModels_ReturnsProviderField(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{{
		ModelName: "nvidia-glm",
		Provider:  "nvidia",
		Model:     "z-ai/glm-5.1",
		APIKeys:   config.SimpleSecureStrings("nv-key"),
	}}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Models []modelResponse `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(resp.Models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(resp.Models))
	}
	if got := resp.Models[0].Provider; got != "nvidia" {
		t.Fatalf("provider = %q, want %q", got, "nvidia")
	}
}

func TestHandleListModels_ReturnsEffectiveProviderField(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{
		{
			ModelName: "plain-openai",
			Model:     "gpt-4o",
		},
		{
			ModelName: "explicit-claude",
			Provider:  "claude",
			Model:     "claude-sonnet-4-6",
		},
		{
			ModelName: "explicit-qwen-intl",
			Provider:  "qwen-international",
			Model:     "qwen3-coder-plus",
		},
	}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Models []modelResponse `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if len(resp.Models) != 3 {
		t.Fatalf("len(models) = %d, want 3", len(resp.Models))
	}

	if got := resp.Models[0].Provider; got != "openai" {
		t.Fatalf("provider[0] = %q, want %q", got, "openai")
	}
	if got := resp.Models[0].Model; got != "gpt-4o" {
		t.Fatalf("model[0] = %q, want %q", got, "gpt-4o")
	}
	if got := resp.Models[1].Provider; got != "claude" {
		t.Fatalf("provider[1] = %q, want %q", got, "claude")
	}
	if got := resp.Models[1].Model; got != "claude-sonnet-4-6" {
		t.Fatalf("model[1] = %q, want %q", got, "claude-sonnet-4-6")
	}
	if got := resp.Models[2].Provider; got != "qwen-international" {
		t.Fatalf("provider[2] = %q, want %q", got, "qwen-international")
	}
	if got := resp.Models[2].Model; got != "qwen3-coder-plus" {
		t.Fatalf("model[2] = %q, want %q", got, "qwen3-coder-plus")
	}
}

func TestHandleSetDefaultModel_RejectsUnknownModelName(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	// First save a valid config with a primary model
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{
		{ModelName: "gpt-4", Model: "openai/gpt-4o"},
	}
	if err = config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/default", bytes.NewBufferString(`{
		"provider":"openai","model_name": "gpt-4o-mini"
	}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := updated.Agents.Defaults.ModelName; got == "gpt-4o-mini" {
		t.Fatalf("default model = %q, want request to leave it unchanged", got)
	}
}

func TestHandleSetDefaultModel_RemovesDefaultFromFallbackChain(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{
		openAIModelConfig("primary"),
		anthropicModelConfig("backup-a"),
		deepseekModelConfig("backup-b"),
	}
	cfg.Agents.Defaults.ModelName = "primary"
	cfg.Agents.Defaults.ModelFallbacks = []string{"backup-a", "backup-b"}
	if err = config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/default", bytes.NewBufferString(`{
		"provider":"openai","model_name": "backup-a"
	}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := updated.Agents.Defaults.ModelName; got != "backup-a" {
		t.Fatalf("default model = %q, want %q", got, "backup-a")
	}
	if len(updated.Agents.Defaults.ModelFallbacks) != 1 || updated.Agents.Defaults.ModelFallbacks[0] != "backup-b" {
		t.Fatalf("fallback_chain = %#v, want [backup-b]", updated.Agents.Defaults.ModelFallbacks)
	}
}

func TestHandleSetDefaultModel_NormalizesExistingFallbackChain(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	writeModelChainConfig(t, configPath, []*config.ModelConfig{
		openAIModelConfig("primary"),
		deepseekModelConfig("backup-b"),
		elevenLabsModelConfig("audio-only"),
	}, "primary", []string{"missing", "audio-only", "backup-b", "primary"})

	updated := setDefaultModelAndLoadConfig(t, configPath, "backup-b")

	assertDefaultChain(t, updated, "backup-b", []string{"audio-only", "primary"})
}

func TestModelGetHandlers_NormalizeDefaultChainSnapshot(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()
	writeModelChainConfig(t, configPath, []*config.ModelConfig{
		openAIModelConfig("primary"),
		deepseekModelConfig("backup-b"),
		elevenLabsModelConfig("audio-only"),
	}, "primary", []string{"audio-only", "elevenlabs/scribe_v1", "backup-b"})

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	t.Run("/api/models", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var response defaultChainResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if response.DefaultModel != "primary" {
			t.Fatalf("default_model = %q, want primary", response.DefaultModel)
		}
		if !slices.Equal(response.FallbackChain, []string{"audio-only", "backup-b"}) {
			t.Fatalf("fallback_chain = %#v, want [audio-only backup-b]", response.FallbackChain)
		}
	})
}

func TestHandleUpdateDefaultChain(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{
		openAIModelConfig("primary"),
		anthropicModelConfig("backup-a"),
		deepseekModelConfig("backup-b"),
	}
	err = config.SaveConfig(configPath, cfg)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/models/default-chain", bytes.NewBufferString(`{
		"default_model":"primary",
		"fallback_chain":["backup-a","backup-b","backup-a"]
	}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp defaultChainResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp.DefaultModel != "primary" {
		t.Fatalf("default_model = %q, want %q", resp.DefaultModel, "primary")
	}
	if len(resp.FallbackChain) != 2 || resp.FallbackChain[0] != "backup-a" || resp.FallbackChain[1] != "backup-b" {
		t.Fatalf("fallback_chain = %#v, want [backup-a backup-b]", resp.FallbackChain)
	}

	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if updated.Agents.Defaults.ModelName != "primary" {
		t.Fatalf("saved default model = %q, want %q", updated.Agents.Defaults.ModelName, "primary")
	}
	if len(updated.Agents.Defaults.ModelFallbacks) != 2 ||
		updated.Agents.Defaults.ModelFallbacks[0] != "backup-a" ||
		updated.Agents.Defaults.ModelFallbacks[1] != "backup-b" {
		t.Fatalf("saved fallback_chain = %#v, want [backup-a backup-b]", updated.Agents.Defaults.ModelFallbacks)
	}
}

func TestHandleUpdateDefaultChain_TrimsStoredLegacyAlias(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	writeModelChainConfig(t, configPath, []*config.ModelConfig{{
		ModelName: "  primary  ",
		Provider:  "openai",
		Model:     "gpt-4o",
		APIBase:   "https://api.example.test/v1",
		APIKeys:   config.SimpleSecureStrings("test-key"),
	}}, "", nil)

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/models/default-chain",
		bytes.NewBufferString(`{"default_model":"primary","fallback_chain":[]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := updated.ModelList[0].ModelName; got != "primary" {
		t.Fatalf("model_name = %q, want primary", got)
	}
}

func TestHandleDeleteModel_RemovesDeletedModelFromDefaultChain(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{
		openAIModelConfig("primary"),
		anthropicModelConfig("backup-a"),
		deepseekModelConfig("backup-b"),
	}
	cfg.Agents.Defaults.ModelName = "primary"
	cfg.Agents.Defaults.ModelFallbacks = []string{"backup-a", "backup-b"}
	err = config.SaveConfig(configPath, cfg)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/models/1", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if updated.Agents.Defaults.ModelName != "primary" {
		t.Fatalf("saved default model = %q, want %q", updated.Agents.Defaults.ModelName, "primary")
	}
	if len(updated.Agents.Defaults.ModelFallbacks) != 1 || updated.Agents.Defaults.ModelFallbacks[0] != "backup-b" {
		t.Fatalf("saved fallback_chain = %#v, want [backup-b]", updated.Agents.Defaults.ModelFallbacks)
	}
}

func TestHandleDeleteModel_ClearsFallbackChainWhenDefaultDeleted(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	writeModelChainConfig(t, configPath, []*config.ModelConfig{
		openAIModelConfig("primary"),
		anthropicModelConfig("backup-a"),
	}, "primary", []string{"backup-a"})

	updated := deleteModelAndLoadConfig(t, configPath, 0)

	assertDefaultChain(t, updated, "", nil)
}

func TestHandleDeleteModel_PreservesPlainEnvRefsAfterShift(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()
	t.Setenv("TEST_DELETE_BASE0", "https://zero.delete/v1")
	t.Setenv("TEST_DELETE_BASE1", "https://one.delete/v1")

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{
		{
			ModelName: "m0",
			Provider:  "openai",
			Model:     "gpt-4o",
			APIBase:   "$TEST_DELETE_BASE0",
			APIKeys:   config.SimpleSecureStrings("sk-a"),
		},
		{
			ModelName: "m1",
			Provider:  "openai",
			Model:     "gpt-4o-mini",
			APIBase:   "$TEST_DELETE_BASE1",
			APIKeys:   config.SimpleSecureStrings("sk-b"),
		},
	}
	cfg.Agents.Defaults.ModelName = "m1"
	cfg.Agents.Defaults.ModelFallbacks = nil
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	updated := deleteModelAndLoadConfig(t, configPath, 0)

	saved, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config) error = %v", err)
	}
	if !strings.Contains(string(saved), `"api_base": "$TEST_DELETE_BASE1"`) {
		t.Fatalf("delete materialized the remaining ref:\n%s", saved)
	}
	if strings.Contains(string(saved), "https://one.delete/v1") {
		t.Fatalf("delete baked the resolved value:\n%s", saved)
	}
	if got := updated.ModelList[0].APIBase; got != "https://one.delete/v1" {
		t.Fatalf("reloaded APIBase = %q", got)
	}
}

func TestHandleDeleteModel_PreservesDefaultChainReferenceWhenAliasStillExists(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.ModelList = []*config.ModelConfig{
		{
			ModelName: "shared-alias",
			Provider:  "openai",
			Model:     "gpt-4o",
			APIBase:   "https://api.openai.com/v1",
			APIKeys:   config.SimpleSecureStrings("sk-openai"),
		},
		{
			ModelName: "shared-alias",
			Provider:  "anthropic",
			Model:     "claude-sonnet-4.6",
			APIBase:   "https://api.example.com/v1",
			APIKeys:   config.SimpleSecureStrings("sk-anthropic"),
		},
		{
			ModelName: "backup-b",
			Provider:  "deepseek",
			Model:     "deepseek-chat",
			APIBase:   "https://api.deepseek.com/v1",
			APIKeys:   config.SimpleSecureStrings("sk-backup-b"),
		},
	}
	cfg.Agents.Defaults.ModelName = "shared-alias"
	cfg.Agents.Defaults.ModelFallbacks = []string{"backup-b"}
	err = config.SaveConfig(configPath, cfg)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/models/1", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	updated, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := updated.Agents.Defaults.ModelName; got != "shared-alias" {
		t.Fatalf("default model = %q, want %q", got, "shared-alias")
	}
	if len(updated.Agents.Defaults.ModelFallbacks) != 1 ||
		updated.Agents.Defaults.ModelFallbacks[0] != "backup-b" {
		t.Fatalf("fallback_chain = %#v, want [backup-b]", updated.Agents.Defaults.ModelFallbacks)
	}
}

func TestHandleUpdateModel_ReturnsReferenceRenameRoles(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()
	writeModelChainConfig(t, configPath, []*config.ModelConfig{
		openAIModelConfig("old-name"),
	}, "openrouter/primary-model", nil)

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/models/0", bytes.NewBufferString(`{
		"model_name":"new-name",
		"provider":"openai",
		"api_base":"https://api.openai.com/v1",
		"model":"gpt-4o",
		"api_key":"sk-primary"
	}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response struct {
		ReferenceRename struct {
			From     string `json:"from"`
			To       string `json:"to"`
			Default  bool   `json:"default"`
			Fallback bool   `json:"fallback"`
		} `json:"reference_rename"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if response.ReferenceRename.From != "old-name" || response.ReferenceRename.To != "new-name" {
		t.Fatalf("reference_rename = %#v, want old-name -> new-name", response.ReferenceRename)
	}
	if !response.ReferenceRename.Default || !response.ReferenceRename.Fallback {
		t.Fatalf("reference_rename roles = %#v, want both roles", response.ReferenceRename)
	}
}

func TestHandleUpdateModel_RenamesDefaultModelReference(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	writeModelChainConfig(t, configPath, []*config.ModelConfig{
		openAIModelConfig("primary"),
		anthropicModelConfig("backup-a"),
	}, "primary", []string{"backup-a"})

	updated := updateModelAndLoadConfig(t, configPath, 0, `{
		"model_name":"primary-renamed",
		"provider":"openai",
		"api_base":"https://api.openai.com/v1",
		"model":"gpt-4o",
		"api_key":"sk-primary"
	}`)

	assertDefaultChain(t, updated, "primary-renamed", []string{"backup-a"})
}

func TestHandleUpdateModel_RenamesFallbackModelReference(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	writeModelChainConfig(t, configPath, []*config.ModelConfig{
		openAIModelConfig("primary"),
		anthropicModelConfig("backup-a"),
		deepseekModelConfig("backup-b"),
	}, "primary", []string{"backup-a", "backup-b"})

	updated := updateModelAndLoadConfig(t, configPath, 1, `{
		"model_name":"backup-a-renamed",
		"provider":"anthropic",
		"api_base":"https://api.example.com/v1",
		"model":"claude-sonnet-4.6",
		"api_key":"sk-backup-a"
	}`)

	assertDefaultChain(t, updated, "primary", []string{"backup-a-renamed", "backup-b"})
}

func TestHandleUpdateModel_RemovesFallbackWhenRenamedToDefaultModel(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	writeModelChainConfig(t, configPath, []*config.ModelConfig{
		openAIModelConfig("primary"),
		anthropicModelConfig("backup-a"),
	}, "primary", []string{"backup-a"})

	updated := updateModelAndLoadConfig(t, configPath, 1, `{
		"model_name":"primary",
		"provider":"anthropic",
		"api_base":"https://api.example.com/v1",
		"model":"claude-sonnet-4.6",
		"api_key":"sk-backup-a"
	}`)

	assertDefaultChain(t, updated, "primary", nil)
}

func TestHandleUpdateModel_RemovesFallbackWhenDefaultRenamedToFallbackModel(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	writeModelChainConfig(t, configPath, []*config.ModelConfig{
		openAIModelConfig("primary"),
		anthropicModelConfig("backup-a"),
	}, "primary", []string{"backup-a"})

	updated := updateModelAndLoadConfig(t, configPath, 0, `{
		"model_name":"backup-a",
		"provider":"openai",
		"api_base":"https://api.openai.com/v1",
		"model":"gpt-4o",
		"api_key":"sk-primary"
	}`)

	assertDefaultChain(t, updated, "backup-a", nil)
}

func TestHandleUpdateModel_DoesNotRenameDefaultChainReferenceWhenAliasStillExists(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	writeModelChainConfig(t, configPath, []*config.ModelConfig{
		openAIModelConfig("shared-alias"),
		anthropicModelConfig("shared-alias"),
		deepseekModelConfig("backup-b"),
	}, "shared-alias", []string{"backup-b"})

	updated := updateModelAndLoadConfig(t, configPath, 1, `{
		"model_name":"renamed-alias",
		"provider":"anthropic",
		"api_base":"https://api.example.com/v1",
		"model":"claude-sonnet-4.6",
		"api_key":"sk-anthropic"
	}`)

	assertDefaultChain(t, updated, "shared-alias", []string{"backup-b"})
}

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "empty key",
			key:  "",
			want: "",
		},
		{
			name: "short key fully masked",
			key:  "abcd",
			want: "****",
		},
		{
			name: "length 8 boundary fully masked",
			key:  "12345678",
			want: "****",
		},
		{
			name: "length 9 boundary shows last 2",
			key:  "123456789",
			want: "123****89",
		},
		{
			name: "length 12 boundary shows last 2",
			key:  "abcdefghijkl",
			want: "abc****kl",
		},
		{
			name: "length 13 boundary shows last 4",
			key:  "abcdefghijklm",
			want: "abc****jklm",
		},
		{
			name: "typical api key",
			key:  "sk-1234567890abcd",
			want: "sk-****abcd",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := maskAPIKey(tc.key)
			if got != tc.want {
				t.Fatalf("maskAPIKey(%q) = %q, want %q", tc.key, got, tc.want)
			}

			if tc.key != "" {
				displayed := strings.Replace(tc.want, "****", "", 1)
				if len(tc.key) <= 8 {
					if displayed != "" {
						t.Fatalf("maskAPIKey(%q) displayed part = %q, want empty", tc.key, displayed)
					}
				} else {
					if len(displayed)*10 > len(tc.key)*6 {
						t.Fatalf(
							"maskAPIKey(%q) displayed length = %d, want at most 60%% of %d",
							tc.key,
							len(displayed),
							len(tc.key),
						)
					}
				}
			}
		})
	}
}

func TestFetchOpenAICompatibleModels_ResponseShapes(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		apiKey    string
		wantLen   int
		wantFirst struct {
			id, ownedBy string
		}
		wantSecond struct {
			id, ownedBy string
		}
	}{
		{
			name:     "envelope shape",
			response: `{"data":[{"id":"gpt-4o","owned_by":"openai"},{"id":"gpt-4o-mini","owned_by":"openai"}]}`,
			apiKey:   "test-key",
			wantLen:  2,
			wantFirst: struct {
				id, ownedBy string
			}{id: "gpt-4o", ownedBy: "openai"},
			wantSecond: struct {
				id, ownedBy string
			}{id: "gpt-4o-mini", ownedBy: "openai"},
		},
		{
			name:     "bare array shape",
			response: `[{"id":"qwen-max","owned_by":"qwen"},{"id":"qwen-plus","owned_by":"qwen"}]`,
			apiKey:   "",
			wantLen:  2,
			wantFirst: struct {
				id, ownedBy string
			}{id: "qwen-max", ownedBy: "qwen"},
			wantSecond: struct {
				id, ownedBy string
			}{id: "qwen-plus", ownedBy: "qwen"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tt.response)
			}))
			defer srv.Close()

			models, err := fetchOpenAICompatibleModels(t.Context(), srv.URL+"/models", tt.apiKey)
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if len(models) != tt.wantLen {
				t.Fatalf("len(models) = %d, want %d", len(models), tt.wantLen)
			}
			if models[0].ID != tt.wantFirst.id || models[0].OwnedBy != tt.wantFirst.ownedBy {
				t.Fatalf("models[0] = %+v, want {ID:%s OwnedBy:%s}", models[0], tt.wantFirst.id, tt.wantFirst.ownedBy)
			}
			if models[1].ID != tt.wantSecond.id || models[1].OwnedBy != tt.wantSecond.ownedBy {
				t.Fatalf("models[1] = %+v, want {ID:%s OwnedBy:%s}", models[1], tt.wantSecond.id, tt.wantSecond.ownedBy)
			}
		})
	}
}

func TestFetchOpenAICompatibleModels_EmptyEnvelopeReturnsEmptySlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer srv.Close()

	models, err := fetchOpenAICompatibleModels(t.Context(), srv.URL+"/models", "k")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(models) != 0 {
		t.Fatalf("len(models) = %d, want 0", len(models))
	}
}

func TestFetchOpenAICompatibleModels_EmptyBareArrayReturnsEmptySlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	models, err := fetchOpenAICompatibleModels(t.Context(), srv.URL+"/models", "k")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(models) != 0 {
		t.Fatalf("len(models) = %d, want 0", len(models))
	}
}

func TestFetchOpenAICompatibleModels_UnrecognizedShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"models":[],"error":"unsupported"}`)
	}))
	defer srv.Close()

	_, err := fetchOpenAICompatibleModels(t.Context(), srv.URL+"/models", "k")
	if err == nil {
		t.Fatal("error = nil, want unrecognized shape error")
	}
	if !strings.Contains(err.Error(), "unrecognized model list response") {
		t.Fatalf("error = %q, want it to contain 'unrecognized model list response'", err.Error())
	}
}

func TestFetchOpenAICompatibleModels_FiltersEmptyIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[`+
			`{"id":"gpt-4o","owned_by":"openai"},`+
			`{"id":"","owned_by":"openai"},`+
			`{"id":"gpt-4o-mini"}]}`)
	}))
	defer srv.Close()

	models, err := fetchOpenAICompatibleModels(t.Context(), srv.URL+"/models", "k")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("len(models) = %d, want 2 (empty IDs should be filtered)", len(models))
	}
	if models[0].ID != "gpt-4o" {
		t.Fatalf("models[0].ID = %q, want %q", models[0].ID, "gpt-4o")
	}
	if models[1].ID != "gpt-4o-mini" {
		t.Fatalf("models[1].ID = %q, want %q", models[1].ID, "gpt-4o-mini")
	}
}

func TestFetchOpenAICompatibleModels_SetsAuthorizationHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"m1"}]}`)
	}))
	defer srv.Close()

	if _, err := fetchOpenAICompatibleModels(t.Context(), srv.URL+"/models", "my-secret-key"); err != nil {
		t.Fatalf("error = %v", err)
	}
	if gotAuth != "Bearer my-secret-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer my-secret-key")
	}
}

func TestFetchOpenAICompatibleModels_NoAuthHeaderWhenKeyEmpty(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":"m1"}]`)
	}))
	defer srv.Close()

	if _, err := fetchOpenAICompatibleModels(t.Context(), srv.URL+"/models", ""); err != nil {
		t.Fatalf("error = %v", err)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization = %q, want empty", gotAuth)
	}
}

func TestHandleFetchModels_SiliconFlowUsesOpenAICompatibleEndpoint(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	var gotPath string
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"deepseek-ai/DeepSeek-V3","owned_by":"siliconflow"}]}`)
	}))
	defer srv.Close()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/fetch", bytes.NewBufferString(fmt.Sprintf(`{
		"provider":"siliconflow",
		"api_key":"sk-siliconflow",
		"api_base":"%s"
	}`, srv.URL)))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if gotPath != "/models" {
		t.Fatalf("path = %q, want %q", gotPath, "/models")
	}
	if gotAuth != "Bearer sk-siliconflow" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer sk-siliconflow")
	}

	var resp struct {
		Models []upstreamModel `json:"models"`
		Total  int             `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if resp.Total != 1 || len(resp.Models) != 1 {
		t.Fatalf("response = %+v, want one fetched model", resp)
	}
	if resp.Models[0].ID != "deepseek-ai/DeepSeek-V3" {
		t.Fatalf("model id = %q, want %q", resp.Models[0].ID, "deepseek-ai/DeepSeek-V3")
	}
}

func TestHandleFetchModels_ModelIndexUsesStoredKey(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"gpt-4o","owned_by":"openai"}]}`)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	oldHome := os.Getenv("MOLE_HOME")
	t.Setenv("MOLE_HOME", filepath.Join(tmp, ".mole"))
	defer func() {
		if oldHome != "" {
			os.Setenv("MOLE_HOME", oldHome)
		} else {
			os.Unsetenv("MOLE_HOME")
		}
	}()

	cfg := config.DefaultConfig()
	cfg.ModelList = []*config.ModelConfig{
		{
			ModelName: "my-openai",
			Provider:  "openai",
			Model:     "gpt-4o",
			APIKeys:   config.SimpleSecureStrings("sk-stored-secret"),
			APIBase:   srv.URL,
		},
	}
	configPath := filepath.Join(tmp, "config.json")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig error: %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	idx := 0
	body := fmt.Sprintf(`{"provider":"openai","api_base":"%s","model_index":%d}`, srv.URL, idx)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/fetch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if gotAuth != "Bearer sk-stored-secret" {
		t.Fatalf("Authorization = %q, want stored key to be used", gotAuth)
	}

	var resp struct {
		Models []upstreamModel `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(resp.Models) != 1 || resp.Models[0].ID != "gpt-4o" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestHandleFetchModels_ModelIndexProviderMismatchRejectsKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("stored key should NOT be sent to mismatched provider")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	t.Setenv("MOLE_HOME", filepath.Join(tmp, ".mole"))

	cfg := config.DefaultConfig()
	cfg.ModelList = []*config.ModelConfig{
		{
			ModelName: "my-openai",
			Provider:  "openai",
			Model:     "gpt-4o",
			APIKeys:   config.SimpleSecureStrings("sk-openai-secret"),
			APIBase:   "https://api.openai.com/v1",
		},
	}
	configPath := filepath.Join(tmp, "config.json")
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig error: %v", err)
	}

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := fmt.Sprintf(`{"provider":"siliconflow","api_base":"%s","model_index":0}`, srv.URL)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/fetch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
}

func TestHandleAddModel_AcceptsFreeFormProvider(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models", bytes.NewBufferString(`{
		"model_name":"gateway-model",
		"provider":"my-gateway",
		"api_base":"https://gateway.example.test/v1",
		"model":"vendor/model-v1"
	}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	added := cfg.ModelList[len(cfg.ModelList)-1]
	if added.Provider != "my-gateway" || added.Model != "vendor/model-v1" {
		t.Fatalf("stored model = %#v", added)
	}
}

func TestHandleAddModel_RejectsMissingAPIBase(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()

	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models", bytes.NewBufferString(`{
		"model_name":"no-base",
		"provider":"custom",
		"model":"model"
	}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "api_base") {
		t.Fatalf("body = %q, want api_base error", rec.Body.String())
	}
}

func TestHandleFetchModels_UsesConfiguredBaseForAnyProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"vendor/model-v1"}]}`)
	}))
	defer srv.Close()

	configPath, cleanup := setupTestEnv(t)
	defer cleanup()
	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := fmt.Sprintf(`{"provider":"anything","api_base":%q,"api_key":"test-key"}`, srv.URL+"/v1")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/models/fetch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "vendor/model-v1") {
		t.Fatalf("body = %s, want fetched model", rec.Body.String())
	}
}
