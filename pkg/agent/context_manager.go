package agent

import (
	"context"

	"github.com/kiry163/mole/pkg/providers"
)

// ContextManager manages conversation context for an AgentLoop.
// The only implementation is legacyContextManager, which preserves the
// existing summarization behavior.
type ContextManager interface {
	// Assemble builds budget-aware context from the ContextManager's own storage.
	// Called before BuildMessages. Returns assembled messages ready for LLM.
	Assemble(ctx context.Context, req *AssembleRequest) (*AssembleResponse, error)

	// Compact compresses conversation history.
	// Called after turn completes (may be async internally) and on context overflow (sync).
	Compact(ctx context.Context, req *CompactRequest) error

	// Ingest records a message into the ContextManager's own storage.
	// Called after each message is persisted to session JSONL.
	Ingest(ctx context.Context, req *IngestRequest) error

	// Clear removes all stored context for a session (messages, summaries, etc.).
	// Called when the user issues /clear or /reset.
	Clear(ctx context.Context, sessionKey string) error
}

// AssembleRequest is the input to Assemble.
type AssembleRequest struct {
	SessionKey string // session identifier
	Budget     int    // context window in tokens
	MaxTokens  int    // max response tokens
}

// AssembleResponse is the output of Assemble.
type AssembleResponse struct {
	History []providers.Message // assembled conversation history for BuildMessages
	Summary string              // conversation summary embedded into system prompt by BuildMessages
}

// CompactRequest is the input to Compact.
type CompactRequest struct {
	SessionKey string                // session identifier
	Reason     ContextCompressReason // proactive_budget | llm_retry | summarize
	Budget     int                   // context window budget (used for retry aggressive compaction)
}

// IngestRequest is the input to Ingest.
type IngestRequest struct {
	SessionKey string            // session identifier
	Message    providers.Message // the message just persisted
}
