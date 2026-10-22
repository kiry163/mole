package agent

import (
	"context"
	"testing"

	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/providers"
	"github.com/kiry163/mole/pkg/tools"
)

type mockRegistryProvider struct{}

type mockRegistryTool struct{}

func (m *mockRegistryTool) Name() string        { return "registry_tool" }
func (m *mockRegistryTool) Description() string { return "test tool" }
func (m *mockRegistryTool) Parameters() map[string]any {
	return map[string]any{"type": "object"}
}
func (m *mockRegistryTool) Execute(context.Context, map[string]any) *tools.ToolResult {
	return tools.SilentResult("ok")
}

func (m *mockRegistryProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{Content: "mock", FinishReason: "stop"}, nil
}

func (m *mockRegistryProvider) GetDefaultModel() string {
	return "mock-model"
}

func testCfg(t *testing.T) *config.Config {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = t.TempDir()
	cfg.Agents.Defaults.ModelName = "gpt-4"
	cfg.Agents.Defaults.MaxTokens = 8192
	cfg.Agents.Defaults.MaxToolIterations = 10
	return cfg
}

func TestNewAgentRegistry_CreatesMain(t *testing.T) {
	registry := NewAgentRegistry(testCfg(t), &mockRegistryProvider{})
	defer registry.Close()

	ids := registry.ListAgentIDs()
	if len(ids) != 1 || ids[0] != "main" {
		t.Fatalf("ListAgentIDs() = %v, want [main]", ids)
	}
	agent, ok := registry.GetAgent("main")
	if !ok || agent == nil {
		t.Fatal("expected to find 'main' agent")
	}
	if agent.ID != "main" {
		t.Fatalf("agent.ID = %q, want main", agent.ID)
	}
}

func TestAgentRegistry_GetAgent_Normalize(t *testing.T) {
	registry := NewAgentRegistry(testCfg(t), &mockRegistryProvider{})
	defer registry.Close()

	agent, ok := registry.GetAgent("Main")
	if !ok || agent == nil {
		t.Fatal("expected normalized lookup to find main")
	}
	if _, ok := registry.GetAgent("unknown"); ok {
		t.Fatal("unknown agent should not resolve")
	}
}

func TestAgentRegistry_GetDefaultAgent(t *testing.T) {
	registry := NewAgentRegistry(testCfg(t), &mockRegistryProvider{})
	defer registry.Close()

	if registry.GetDefaultAgent() == nil {
		t.Fatal("expected a default agent")
	}
}

func TestAgentRegistry_ForEachTool(t *testing.T) {
	registry := NewAgentRegistry(testCfg(t), &mockRegistryProvider{})
	defer registry.Close()

	registry.GetDefaultAgent().Tools.Register(&mockRegistryTool{})
	called := false
	registry.ForEachTool("registry_tool", func(tool tools.Tool) {
		called = true
	})
	if !called {
		t.Fatal("ForEachTool did not visit the registered tool")
	}
}
