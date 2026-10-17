package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kiry163/mole/pkg/config"
)

func TestProbeLocalModelAvailability_OpenAICompatibleIncludesAPIKey(t *testing.T) {
	const apiKey = "test-api-key"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/models")
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"custom-model"}]}`))
	}))
	defer srv.Close()

	model := &config.ModelConfig{
		Provider: "openai",
		Model:    "custom-model",
		APIBase:  srv.URL + "/v1",
	}
	model.SetAPIKey(apiKey)

	if !probeLocalModelAvailability(model) {
		t.Fatal("probeLocalModelAvailability() = false, want true when api_key is configured")
	}
}

func TestProbeOpenAICompatibleModel_SupportsBareArrayModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/models")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":"gpt-4o-mini"},{"id":"deepseek-r1"}]`)
	}))
	defer srv.Close()

	if !probeOpenAICompatibleModel(srv.URL+"/v1", "gpt-4o-mini", "") {
		t.Fatal("probeOpenAICompatibleModel() = false, want true for bare array /models response")
	}
}

func TestModelProbeCacheKey_DifferentAPIKeysProduceDifferentKeys(t *testing.T) {
	base := &config.ModelConfig{
		ModelName: "local-vllm",
		Model:     "vllm/custom-model",
		APIBase:   "http://127.0.0.1:8000/v1",
	}

	m1 := *base
	m1.SetAPIKey("key-a")
	m2 := *base
	m2.SetAPIKey("key-b")

	k1 := modelProbeCacheKey(&m1)
	k2 := modelProbeCacheKey(&m2)
	if k1 == k2 {
		t.Fatal("modelProbeCacheKey() should differ when api key changes")
	}
}

func TestModelProbeCacheKey_NormalizesTrailingSlashInAPIBase(t *testing.T) {
	m1 := &config.ModelConfig{
		ModelName: "local-vllm",
		Model:     "vllm/custom-model",
		APIBase:   "http://127.0.0.1:8000/v1",
	}
	m2 := &config.ModelConfig{
		ModelName: "local-vllm",
		Model:     "vllm/custom-model",
		APIBase:   "http://127.0.0.1:8000/v1/",
	}

	k1 := modelProbeCacheKey(m1)
	k2 := modelProbeCacheKey(m2)
	if k1 != k2 {
		t.Fatalf("modelProbeCacheKey() mismatch for equivalent api_base values: %q vs %q", k1, k2)
	}
}

func TestModelProbeCacheKey_IgnoresDisplayFields(t *testing.T) {
	base := &config.ModelConfig{
		ModelName: "vllm-one",
		Model:     "vllm/custom-model",
		APIBase:   "http://127.0.0.1:8000/v1",
	}
	changed := &config.ModelConfig{
		ModelName: "vllm-two",
		Model:     "vllm/custom-model",
		APIBase:   "http://127.0.0.1:8000/v1",
	}

	k1 := modelProbeCacheKey(base)
	k2 := modelProbeCacheKey(changed)
	if k1 != k2 {
		t.Fatalf("modelProbeCacheKey() should ignore non-probe fields, got %q vs %q", k1, k2)
	}
}

func TestProbeLocalModelAvailability_SuccessBackoff(t *testing.T) {
	resetModelProbeHooks(t)

	now := time.Unix(1700000000, 0)
	modelProbeNowFunc = func() time.Time { return now }

	calls := 0
	probeOpenAICompatibleModelFunc = func(apiBase, modelID, apiKey string) bool {
		calls++
		return true
	}

	model := &config.ModelConfig{
		ModelName: "local-vllm",
		Model:     "vllm/custom-model",
		APIBase:   "http://127.0.0.1:8000/v1",
	}

	if !probeLocalModelAvailability(model) {
		t.Fatal("first probe result = false, want true")
	}
	if calls != 1 {
		t.Fatalf("probe calls after first probe = %d, want 1", calls)
	}

	if !probeLocalModelAvailability(model) {
		t.Fatal("cached probe result = false, want true")
	}
	if calls != 1 {
		t.Fatalf("probe calls after immediate re-check = %d, want 1", calls)
	}

	now = now.Add(modelProbeSuccessBaseInterval)
	if !probeLocalModelAvailability(model) {
		t.Fatal("second probe result = false, want true")
	}
	if calls != 2 {
		t.Fatalf("probe calls after success backoff window = %d, want 2", calls)
	}

	now = now.Add(modelProbeSuccessBaseInterval)
	if !probeLocalModelAvailability(model) {
		t.Fatal("cached result after doubled backoff = false, want true")
	}
	if calls != 2 {
		t.Fatalf("probe calls before doubled backoff expires = %d, want 2", calls)
	}

	now = now.Add(modelProbeSuccessBaseInterval)
	if !probeLocalModelAvailability(model) {
		t.Fatal("third probe result = false, want true")
	}
	if calls != 3 {
		t.Fatalf("probe calls after doubled backoff expires = %d, want 3", calls)
	}
}

func TestProbeLocalModelAvailability_FailureBackoff(t *testing.T) {
	resetModelProbeHooks(t)

	now := time.Unix(1700000100, 0)
	modelProbeNowFunc = func() time.Time { return now }

	calls := 0
	probeOpenAICompatibleModelFunc = func(apiBase, modelID, apiKey string) bool {
		calls++
		return false
	}

	model := &config.ModelConfig{
		ModelName: "local-vllm",
		Model:     "vllm/custom-model",
		APIBase:   "http://127.0.0.1:8000/v1",
	}

	if probeLocalModelAvailability(model) {
		t.Fatal("first probe result = true, want false")
	}
	if calls != 1 {
		t.Fatalf("probe calls after first failure = %d, want 1", calls)
	}

	if probeLocalModelAvailability(model) {
		t.Fatal("cached failed probe result = true, want false")
	}
	if calls != 1 {
		t.Fatalf("probe calls after immediate failed re-check = %d, want 1", calls)
	}

	now = now.Add(modelProbeFailureBaseInterval)
	if probeLocalModelAvailability(model) {
		t.Fatal("second failed probe result = true, want false")
	}
	if calls != 2 {
		t.Fatalf("probe calls after failure backoff window = %d, want 2", calls)
	}

	now = now.Add(modelProbeFailureBaseInterval)
	if probeLocalModelAvailability(model) {
		t.Fatal("cached failure after doubled backoff = true, want false")
	}
	if calls != 2 {
		t.Fatalf("probe calls before doubled failure backoff expires = %d, want 2", calls)
	}

	now = now.Add(modelProbeFailureBaseInterval)
	if probeLocalModelAvailability(model) {
		t.Fatal("third failed probe result = true, want false")
	}
	if calls != 3 {
		t.Fatalf("probe calls after doubled failure backoff expires = %d, want 3", calls)
	}
}

func TestProbeLocalModelAvailability_ResultFlipResetsBackoff(t *testing.T) {
	resetModelProbeHooks(t)

	now := time.Unix(1700000200, 0)
	modelProbeNowFunc = func() time.Time { return now }

	results := []bool{true, false, false}
	index := 0
	probeOpenAICompatibleModelFunc = func(apiBase, modelID, apiKey string) bool {
		if index >= len(results) {
			return false
		}
		result := results[index]
		index++
		return result
	}

	model := &config.ModelConfig{
		ModelName: "local-vllm",
		Model:     "vllm/custom-model",
		APIBase:   "http://127.0.0.1:8000/v1",
	}

	if !probeLocalModelAvailability(model) {
		t.Fatal("first probe result = false, want true")
	}

	now = now.Add(modelProbeSuccessBaseInterval)
	if probeLocalModelAvailability(model) {
		t.Fatal("second probe result = true, want false")
	}

	now = now.Add(modelProbeFailureBaseInterval)
	if probeLocalModelAvailability(model) {
		t.Fatal("third probe result = true, want false")
	}

	if index != 3 {
		t.Fatalf("probe invocations = %d, want 3", index)
	}
}

func TestProbeLocalModelAvailability_DeduplicatesInflightProbe(t *testing.T) {
	resetModelProbeHooks(t)

	now := time.Unix(1700000300, 0)
	modelProbeNowFunc = func() time.Time { return now }

	var calls int32
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})

	probeOpenAICompatibleModelFunc = func(apiBase, modelID, apiKey string) bool {
		if atomic.AddInt32(&calls, 1) == 1 {
			close(probeStarted)
		}
		<-releaseProbe
		return true
	}

	model := &config.ModelConfig{
		ModelName: "local-vllm",
		Model:     "vllm/custom-model",
		APIBase:   "http://127.0.0.1:8000/v1",
	}

	const workers = 8
	var wg sync.WaitGroup
	results := make(chan bool, workers)
	workerStarted := make(chan struct{}, workers)

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			workerStarted <- struct{}{}
			results <- probeLocalModelAvailability(model)
		}()
	}

	for range workers {
		<-workerStarted
	}

	select {
	case <-probeStarted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("probe did not start in time")
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("concurrent probe calls = %d, want 1", got)
	}

	close(releaseProbe)
	wg.Wait()
	close(results)

	for result := range results {
		if !result {
			t.Fatal("deduplicated probe result = false, want true")
		}
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("final probe calls = %d, want 1", got)
	}
}
