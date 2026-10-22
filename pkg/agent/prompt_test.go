package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTurnProfileSkill(t *testing.T, workspace, name, content string) {
	t.Helper()
	dir := filepath.Join(workspace, "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}
}

func TestPromptRegistry_RejectsRegisteredSourceWrongPlacement(t *testing.T) {
	registry := NewPromptRegistry()
	if err := registry.RegisterSource(PromptSourceDescriptor{
		ID:      "test:source",
		Owner:   "test",
		Allowed: []PromptPlacement{{Layer: PromptLayerCapability, Slot: PromptSlotTooling}},
	}); err != nil {
		t.Fatalf("RegisterSource() error = %v", err)
	}

	err := registry.ValidatePart(PromptPart{
		ID:      "wrong.placement",
		Layer:   PromptLayerContext,
		Slot:    PromptSlotRuntime,
		Source:  PromptSource{ID: "test:source"},
		Content: "runtime text",
	})
	if err == nil {
		t.Fatal("ValidatePart() error = nil, want placement error")
	}
}

func TestPromptRegistry_AllowsUnregisteredSourceInCompatibilityMode(t *testing.T) {
	registry := NewPromptRegistry()

	err := registry.ValidatePart(PromptPart{
		ID:      "unregistered.part",
		Layer:   PromptLayerCapability,
		Slot:    PromptSlotMCP,
		Source:  PromptSource{ID: "mcp:dynamic-server"},
		Content: "dynamic MCP prompt",
	})
	if err != nil {
		t.Fatalf("ValidatePart() error = %v, want nil for unregistered source", err)
	}
}

func TestRenderPromptPartsLegacy_UsesLayerAndSlotOrder(t *testing.T) {
	parts := []PromptPart{
		{
			ID:      "context.runtime",
			Layer:   PromptLayerContext,
			Slot:    PromptSlotRuntime,
			Source:  PromptSource{ID: PromptSourceRuntime},
			Content: "runtime",
		},
		{
			ID:      "kernel.identity",
			Layer:   PromptLayerKernel,
			Slot:    PromptSlotIdentity,
			Source:  PromptSource{ID: PromptSourceKernel},
			Content: "kernel",
		},
		{
			ID:      "capability.skill",
			Layer:   PromptLayerCapability,
			Slot:    PromptSlotActiveSkill,
			Source:  PromptSource{ID: "skill:test"},
			Content: "skill",
		},
		{
			ID:      "instruction.workspace",
			Layer:   PromptLayerInstruction,
			Slot:    PromptSlotWorkspace,
			Source:  PromptSource{ID: PromptSourceWorkspace},
			Content: "workspace",
		},
	}

	got := renderPromptPartsLegacy(parts)
	want := strings.Join([]string{"kernel", "workspace", "skill", "runtime"}, "\n\n---\n\n")
	if got != want {
		t.Fatalf("renderPromptPartsLegacy() = %q, want %q", got, want)
	}
}

func TestBuildMessagesFromPrompt_AttachesInternalPromptMetadata(t *testing.T) {
	t.Setenv("MOLE_BUILTIN_SKILLS", t.TempDir())
	cb := NewContextBuilder(t.TempDir())

	messages := cb.BuildMessagesFromPrompt(PromptBuildRequest{
		CurrentMessage: "hello",
		Summary:        "prior context",
	})
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}

	system := messages[0]
	if len(system.SystemParts) < 3 {
		t.Fatalf("system parts len = %d, want at least 3", len(system.SystemParts))
	}
	if system.SystemParts[0].PromptLayer != string(PromptLayerKernel) ||
		system.SystemParts[0].PromptSlot != string(PromptSlotIdentity) ||
		system.SystemParts[0].PromptSource != string(PromptSourceKernel) {
		t.Fatalf("static system metadata = %#v, want kernel identity", system.SystemParts[0])
	}

	var hasRuntime, hasSummary bool
	for _, part := range system.SystemParts {
		switch part.PromptSource {
		case string(PromptSourceRuntime):
			hasRuntime = true
		case string(PromptSourceSummary):
			hasSummary = true
		}
	}
	if !hasRuntime {
		t.Fatal("system parts missing runtime prompt metadata")
	}
	if !hasSummary {
		t.Fatal("system parts missing summary prompt metadata")
	}

	user := messages[1]
	if user.PromptLayer != string(PromptLayerTurn) ||
		user.PromptSlot != string(PromptSlotMessage) ||
		user.PromptSource != string(PromptSourceUserMessage) {
		t.Fatalf("user message metadata = %#v, want turn message", user)
	}

	data, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(data), "PromptSource") ||
		strings.Contains(string(data), "PromptLayer") ||
		strings.Contains(string(data), "PromptSlot") {
		t.Fatalf("internal prompt metadata leaked into JSON: %s", data)
	}
}

func TestContextBuilder_CollectsToolDiscoveryContributor(t *testing.T) {
	t.Setenv("MOLE_BUILTIN_SKILLS", t.TempDir())
	cb := NewContextBuilder(t.TempDir()).WithToolDiscovery(true, false)

	messages := cb.BuildMessagesFromPrompt(PromptBuildRequest{CurrentMessage: "hello"})
	system := messages[0]
	if !strings.Contains(system.Content, "tool_search_tool_bm25") {
		t.Fatalf("system prompt missing tool discovery rule: %q", system.Content)
	}

	var found bool
	for _, part := range system.SystemParts {
		if part.PromptSource == string(PromptSourceToolDiscovery) {
			found = true
			if part.PromptLayer != string(PromptLayerCapability) || part.PromptSlot != string(PromptSlotTooling) {
				t.Fatalf("tool discovery metadata = %#v, want capability/tooling", part)
			}
		}
	}
	if !found {
		t.Fatal("system parts missing tool discovery prompt metadata")
	}
}

func TestContextBuilder_SuppressesToolDiscoveryContributorWhenToolsUnavailable(t *testing.T) {
	t.Setenv("MOLE_BUILTIN_SKILLS", t.TempDir())
	cb := NewContextBuilder(t.TempDir()).WithToolDiscovery(true, false)

	messages := cb.BuildMessagesFromPrompt(PromptBuildRequest{
		CurrentMessage:      "hello",
		SuppressToolUseRule: true,
	})
	system := messages[0]
	if strings.Contains(system.Content, "tool_search_tool_bm25") {
		t.Fatalf("system prompt includes tool discovery despite tools being unavailable: %q", system.Content)
	}
	for _, part := range system.SystemParts {
		if part.PromptSource == string(PromptSourceToolDiscovery) {
			t.Fatalf("system parts include tool discovery despite tools being unavailable: %#v", part)
		}
	}
}

func TestContextBuilder_SuppressesToolReferencesWhenToolsUnavailable(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("MOLE_BUILTIN_SKILLS", t.TempDir())
	writeTurnProfileSkill(
		t,
		workspace,
		"research",
		"---\ndescription: research skill\n---\n# research\n\nResearch carefully.",
	)
	cb := NewContextBuilder(workspace)

	messages := cb.BuildMessagesFromPrompt(PromptBuildRequest{
		CurrentMessage:      "hello",
		SuppressToolUseRule: true,
	})
	system := messages[0]
	if strings.Contains(system.Content, "When using tools") ||
		strings.Contains(system.Content, "read_file tool") ||
		strings.Contains(system.Content, "update "+workspace+"/memory/MEMORY.md") {
		t.Fatalf("system prompt includes tool references despite tools being unavailable: %q", system.Content)
	}
	if !strings.Contains(system.Content, "<name>research</name>") {
		t.Fatalf("system prompt should keep non-tool skill catalog context, got: %q", system.Content)
	}
}

func TestContextBuilder_CollectsMCPServerContributor(t *testing.T) {
	t.Setenv("MOLE_BUILTIN_SKILLS", t.TempDir())
	cb := NewContextBuilder(t.TempDir())
	err := cb.RegisterPromptContributor(mcpServerPromptContributor{
		serverName: "GitHub Server",
		toolCount:  3,
		deferred:   true,
	})
	if err != nil {
		t.Fatalf("RegisterPromptContributor() error = %v", err)
	}

	messages := cb.BuildMessagesFromPrompt(PromptBuildRequest{CurrentMessage: "hello"})
	system := messages[0]
	if !strings.Contains(system.Content, "MCP server `GitHub Server` is connected") {
		t.Fatalf("system prompt missing MCP contributor content: %q", system.Content)
	}

	var found bool
	for _, part := range system.SystemParts {
		if part.PromptSource == "mcp:github_server" {
			found = true
			if part.PromptLayer != string(PromptLayerCapability) || part.PromptSlot != string(PromptSlotMCP) {
				t.Fatalf("mcp metadata = %#v, want capability/mcp", part)
			}
		}
	}
	if !found {
		t.Fatal("system parts missing MCP prompt metadata")
	}
}

func TestContextBuilder_SuppressesMCPServerContributorWhenToolsUnavailable(t *testing.T) {
	t.Setenv("MOLE_BUILTIN_SKILLS", t.TempDir())
	cb := NewContextBuilder(t.TempDir())
	err := cb.RegisterPromptContributor(mcpServerPromptContributor{
		serverName: "GitHub Server",
		toolCount:  3,
		deferred:   false,
	})
	if err != nil {
		t.Fatalf("RegisterPromptContributor() error = %v", err)
	}

	messages := cb.BuildMessagesFromPrompt(PromptBuildRequest{
		CurrentMessage:      "hello",
		SuppressToolUseRule: true,
	})
	system := messages[0]
	if strings.Contains(system.Content, "MCP server `GitHub Server` is connected") ||
		strings.Contains(system.Content, "available as native tools") {
		t.Fatalf("system prompt includes MCP tooling despite tools being unavailable: %q", system.Content)
	}
	for _, part := range system.SystemParts {
		if part.PromptSource == "mcp:github_server" {
			t.Fatalf("system parts include MCP tooling despite tools being unavailable: %#v", part)
		}
	}
}

type testPromptContributor struct {
	desc PromptSourceDescriptor
	part PromptPart
}

func (c testPromptContributor) PromptSource() PromptSourceDescriptor {
	return c.desc
}

func (c testPromptContributor) ContributePrompt(_ context.Context, _ PromptBuildRequest) ([]PromptPart, error) {
	return []PromptPart{c.part}, nil
}

func TestContextBuilder_CollectsRegisteredPromptContributors(t *testing.T) {
	t.Setenv("MOLE_BUILTIN_SKILLS", t.TempDir())
	cb := NewContextBuilder(t.TempDir())

	sourceID := PromptSourceID("test:contributor")
	err := cb.RegisterPromptContributor(testPromptContributor{
		desc: PromptSourceDescriptor{
			ID:      sourceID,
			Owner:   "test",
			Allowed: []PromptPlacement{{Layer: PromptLayerCapability, Slot: PromptSlotMCP}},
		},
		part: PromptPart{
			ID:      "capability.mcp.test",
			Layer:   PromptLayerCapability,
			Slot:    PromptSlotMCP,
			Source:  PromptSource{ID: sourceID, Name: "test"},
			Content: "registered contributor prompt",
		},
	})
	if err != nil {
		t.Fatalf("RegisterPromptContributor() error = %v", err)
	}

	messages := cb.BuildMessagesFromPrompt(PromptBuildRequest{CurrentMessage: "hello"})
	if !strings.Contains(messages[0].Content, "registered contributor prompt") {
		t.Fatalf("system prompt missing contributor content: %q", messages[0].Content)
	}
}
