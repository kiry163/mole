package agent

import (
	"github.com/kiry163/mole/pkg/providers"
)

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
