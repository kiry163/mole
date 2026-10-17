package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kiry163/mole/pkg/bus"
	"github.com/kiry163/mole/pkg/config"
	runtimeevents "github.com/kiry163/mole/pkg/events"
	"github.com/kiry163/mole/pkg/providers"
)

// ---------------------------------------------------------------------------
// resolveContextManager tests
// ---------------------------------------------------------------------------

func TestResolveContextManager_ReturnsLegacy(t *testing.T) {
	cfg := testConfig(t)
	al := newCMTestAgentLoop(cfg)

	if _, ok := al.contextManager.(*legacyContextManager); !ok {
		t.Fatalf("expected *legacyContextManager, got %T", al.contextManager)
	}
}

// ---------------------------------------------------------------------------
// Legacy Assemble tests
// ---------------------------------------------------------------------------

func TestLegacyAssemble_Passthrough(t *testing.T) {
	cfg := testConfig(t)
	al := newCMTestAgentLoop(cfg)

	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		t.Fatal("expected default agent")
	}

	history := []providers.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	agent.Sessions.SetHistory("test-session", history)

	resp, err := al.contextManager.Assemble(context.Background(), &AssembleRequest{
		SessionKey: "test-session",
		Budget:     8000,
		MaxTokens:  4096,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.History) != len(history) {
		t.Fatalf("expected %d messages, got %d", len(history), len(resp.History))
	}
	for i, msg := range resp.History {
		if msg.Content != history[i].Content || msg.Role != history[i].Role {
			t.Fatalf("message %d mismatch: want %+v, got %+v", i, history[i], msg)
		}
	}
}

func TestLegacyAssemble_EmptyHistory(t *testing.T) {
	cfg := testConfig(t)
	al := newCMTestAgentLoop(cfg)

	resp, err := al.contextManager.Assemble(context.Background(), &AssembleRequest{
		SessionKey: "test-session",
		Budget:     8000,
		MaxTokens:  4096,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.History) != 0 {
		t.Fatalf("expected empty messages, got %d", len(resp.History))
	}
}

// ---------------------------------------------------------------------------
// Legacy Compact overflow tests
// ---------------------------------------------------------------------------

func TestLegacyCompact_Overflow(t *testing.T) {
	cfg := testConfig(t)
	al := newCMTestAgentLoop(cfg)

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("expected default agent")
	}

	history := []providers.Message{
		{Role: "user", Content: "msg 1"},
		{Role: "assistant", Content: "resp 1"},
		{Role: "user", Content: "msg 2"},
		{Role: "assistant", Content: "resp 2"},
		{Role: "user", Content: "msg 3"},
	}
	defaultAgent.Sessions.SetHistory("session-overflow", history)

	runtimeCh, closeRuntimeEvents := subscribeRuntimeEventsForTest(
		t,
		al,
		16,
		runtimeevents.KindAgentContextCompress,
	)
	defer closeRuntimeEvents()

	err := al.contextManager.Compact(context.Background(), &CompactRequest{
		SessionKey: "session-overflow",
		Reason:     ContextCompressReasonRetry,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// After overflow compression, history should be shorter
	newHistory := defaultAgent.Sessions.GetHistory("session-overflow")
	if len(newHistory) >= len(history) {
		t.Fatalf("expected compressed history, got %d messages (was %d)", len(newHistory), len(history))
	}

	// Summary should contain compression note
	summary := defaultAgent.Sessions.GetSummary("session-overflow")
	if !strings.Contains(summary, "Emergency compression") {
		t.Fatalf("expected compression note in summary, got %q", summary)
	}

	// Event should carry the proactive reason
	events := collectRuntimeEventStream(runtimeCh)
	compressEvt, ok := findRuntimeEvent(events, runtimeevents.KindAgentContextCompress)
	if !ok {
		t.Fatal("expected context compress event")
	}
	payload, ok := compressEvt.Payload.(ContextCompressPayload)
	if !ok {
		t.Fatalf("expected ContextCompressPayload, got %T", compressEvt.Payload)
	}
	if payload.Reason != ContextCompressReasonRetry {
		t.Fatalf("expected retry reason, got %q", payload.Reason)
	}
}

func TestLegacyCompact_Overflow_ProactiveReason(t *testing.T) {
	cfg := testConfig(t)
	al := newCMTestAgentLoop(cfg)

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("expected default agent")
	}

	history := []providers.Message{
		{Role: "user", Content: "msg 1"},
		{Role: "assistant", Content: "resp 1"},
		{Role: "user", Content: "msg 2"},
		{Role: "assistant", Content: "resp 2"},
		{Role: "user", Content: "msg 3"},
	}
	defaultAgent.Sessions.SetHistory("session-proactive", history)

	runtimeCh, closeRuntimeEvents := subscribeRuntimeEventsForTest(
		t,
		al,
		16,
		runtimeevents.KindAgentContextCompress,
	)
	defer closeRuntimeEvents()

	err := al.contextManager.Compact(context.Background(), &CompactRequest{
		SessionKey: "session-proactive",
		Reason:     ContextCompressReasonProactive,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := collectRuntimeEventStream(runtimeCh)
	compressEvt, ok := findRuntimeEvent(events, runtimeevents.KindAgentContextCompress)
	if !ok {
		t.Fatal("expected context compress event")
	}
	payload, ok := compressEvt.Payload.(ContextCompressPayload)
	if !ok {
		t.Fatalf("expected ContextCompressPayload, got %T", compressEvt.Payload)
	}
	if payload.Reason != ContextCompressReasonProactive {
		t.Fatalf("expected proactive reason, got %q", payload.Reason)
	}
}

func TestLegacyCompact_Overflow_TooShortToCompress(t *testing.T) {
	cfg := testConfig(t)
	al := newCMTestAgentLoop(cfg)

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("expected default agent")
	}

	history := []providers.Message{
		{Role: "user", Content: "only one"},
	}
	defaultAgent.Sessions.SetHistory("session-tiny", history)

	err := al.contextManager.Compact(context.Background(), &CompactRequest{
		SessionKey: "session-tiny",
		Reason:     ContextCompressReasonRetry,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// History should be unchanged (too short to compress)
	newHistory := defaultAgent.Sessions.GetHistory("session-tiny")
	if len(newHistory) != len(history) {
		t.Fatalf("expected history unchanged, got %d messages (was %d)", len(newHistory), len(history))
	}
}

// ---------------------------------------------------------------------------
// Legacy Compact post-turn tests
// ---------------------------------------------------------------------------

func TestLegacyCompact_PostTurn_BelowThreshold(t *testing.T) {
	cfg := testConfig(t)
	al := newCMTestAgentLoop(cfg)

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("expected default agent")
	}

	// Small history, below summarization thresholds
	history := []providers.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	defaultAgent.Sessions.SetHistory("session-small", history)

	err := al.contextManager.Compact(context.Background(), &CompactRequest{
		SessionKey: "session-small",
		Reason:     ContextCompressReasonSummarize,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// History should remain unchanged
	newHistory := defaultAgent.Sessions.GetHistory("session-small")
	if len(newHistory) != len(history) {
		t.Fatalf("expected unchanged history, got %d messages (was %d)", len(newHistory), len(history))
	}
}

func TestLegacyCompact_PostTurn_ExceedsMessageThreshold(t *testing.T) {
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:                 t.TempDir(),
				ModelName:                 "test-model",
				MaxTokens:                 4096,
				MaxToolIterations:         10,
				ContextWindow:             8000,
				SummarizeMessageThreshold: 2,
				SummarizeTokenPercent:     75,
			},
		},
	}
	msgBus := bus.NewMessageBus()
	al := NewAgentLoop(cfg, msgBus, &simpleMockProvider{response: "summary"})

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("expected default agent")
	}

	// 6 messages > threshold of 2
	history := []providers.Message{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2"},
		{Role: "user", Content: "q3"},
		{Role: "assistant", Content: "a3"},
	}
	defaultAgent.Sessions.SetHistory("session-threshold", history)

	runtimeCh, closeRuntimeEvents := subscribeRuntimeEventsForTest(
		t,
		al,
		16,
		runtimeevents.KindAgentSessionSummarize,
	)
	defer closeRuntimeEvents()

	err := al.contextManager.Compact(context.Background(), &CompactRequest{
		SessionKey: "session-threshold",
		Reason:     ContextCompressReasonSummarize,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	waitForRuntimeEvent(t, runtimeCh, 5*time.Second, func(evt runtimeevents.Event) bool {
		return evt.Kind == runtimeevents.KindAgentSessionSummarize
	})

	newHistory := defaultAgent.Sessions.GetHistory("session-threshold")
	if len(newHistory) >= len(history) {
		t.Fatalf("expected summarization to reduce history from %d messages, got %d", len(history), len(newHistory))
	}
}

// ---------------------------------------------------------------------------
// Legacy Ingest tests
// ---------------------------------------------------------------------------

func TestLegacyIngest_NoOp(t *testing.T) {
	cfg := testConfig(t)
	al := newCMTestAgentLoop(cfg)

	err := al.contextManager.Ingest(context.Background(), &IngestRequest{
		SessionKey: "session-ingest",
		Message:    providers.Message{Role: "user", Content: "test"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// forceCompression edge cases (via legacy Compact)
// ---------------------------------------------------------------------------

func TestLegacyCompact_Overflow_SingleTurnKeepsLastUserMessage(t *testing.T) {
	cfg := testConfig(t)
	al := newCMTestAgentLoop(cfg)

	defaultAgent := al.registry.GetDefaultAgent()
	if defaultAgent == nil {
		t.Fatal("expected default agent")
	}

	// History with only 2 messages — forceCompression should still handle it
	history := []providers.Message{
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
	}
	defaultAgent.Sessions.SetHistory("session-2msg", history)

	err := al.contextManager.Compact(context.Background(), &CompactRequest{
		SessionKey: "session-2msg",
		Reason:     ContextCompressReasonRetry,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	newHistory := defaultAgent.Sessions.GetHistory("session-2msg")
	// With 2 messages, forceCompression returns false (len <= 2), so no compression
	if len(newHistory) != len(history) {
		t.Fatalf("expected no compression for 2-message history, got %d", len(newHistory))
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         t.TempDir(),
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}
}

func newCMTestAgentLoop(cfg *config.Config) *AgentLoop {
	msgBus := bus.NewMessageBus()
	return NewAgentLoop(cfg, msgBus, &simpleMockProvider{response: "test"})
}
