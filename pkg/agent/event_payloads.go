package agent

import "time"

// TurnEndStatus describes the terminal state of a turn.
type TurnEndStatus string

const (
	// TurnEndStatusCompleted indicates the turn finished normally.
	TurnEndStatusCompleted TurnEndStatus = "completed"
	// TurnEndStatusError indicates the turn ended because of an error.
	TurnEndStatusError TurnEndStatus = "error"
	// TurnEndStatusAborted indicates the turn was hard-aborted and rolled back.
	TurnEndStatusAborted TurnEndStatus = "aborted"
)

// TurnStartPayload describes the start of a turn.
type TurnStartPayload struct {
	UserMessage string
	MediaCount  int
}

const (
	skillContextTriggerInitialBuild        = "initial_build"
	skillContextTriggerContextRetryRebuild = "context_retry_rebuild"
)

type SkillContextSnapshot struct {
	Sequence   int      `json:"sequence"`
	Trigger    string   `json:"trigger"`
	SkillNames []string `json:"skill_names,omitempty"`
}

type ToolExecutionRecord struct {
	Name         string   `json:"name"`
	Success      bool     `json:"success"`
	ErrorSummary string   `json:"error_summary,omitempty"`
	SkillNames   []string `json:"skill_names,omitempty"`
}

// TurnEndPayload describes the completion of a turn.
type TurnEndPayload struct {
	Status                TurnEndStatus
	Workspace             string
	Iterations            int
	Duration              time.Duration
	FinalContentLen       int
	UserMessage           string
	FinalContent          string
	ActiveSkills          []string
	AttemptedSkills       []string
	FinalSuccessfulPath   []string
	SkillContextSnapshots []SkillContextSnapshot
	ToolKinds             []string
	ToolExecutions        []ToolExecutionRecord
}

// LLMRequestPayload describes an outbound LLM request.
type LLMRequestPayload struct {
	Model         string
	MessagesCount int
	ToolsCount    int
	MaxTokens     int
	Temperature   float64
}

// LLMResponsePayload describes an inbound LLM response.
type LLMResponsePayload struct {
	ContentLen   int
	ToolCalls    int
	HasReasoning bool
}

// LLMRetryPayload describes a retry of an LLM request.
type LLMRetryPayload struct {
	Attempt    int
	MaxRetries int
	Reason     string
	Error      string
	Backoff    time.Duration
}

// ContextCompressReason identifies why emergency compression ran.
type ContextCompressReason string

const (
	// ContextCompressReasonProactive indicates compression before the first LLM call.
	ContextCompressReasonProactive ContextCompressReason = "proactive_budget"
	// ContextCompressReasonRetry indicates compression during context-error retry handling.
	ContextCompressReasonRetry ContextCompressReason = "llm_retry"
	// ContextCompressReasonSummarize indicates post-turn async summarization.
	ContextCompressReasonSummarize ContextCompressReason = "summarize"
)

// ContextCompressPayload describes a forced history compression.
type ContextCompressPayload struct {
	Reason            ContextCompressReason
	DroppedMessages   int
	RemainingMessages int
}

// SessionSummarizePayload describes a completed async session summarization.
type SessionSummarizePayload struct {
	SummarizedMessages int
	KeptMessages       int
	SummaryLen         int
	OmittedOversized   bool
}

// ToolExecStartPayload describes a tool execution request.
type ToolExecStartPayload struct {
	Tool      string
	Arguments map[string]any
}

// ToolExecEndPayload describes the outcome of a tool execution.
type ToolExecEndPayload struct {
	Tool       string
	Duration   time.Duration
	ForLLMLen  int
	ForUserLen int
	IsError    bool
}

// ToolExecSkippedPayload describes a skipped tool call.
type ToolExecSkippedPayload struct {
	Tool   string
	Reason string
}

// SteeringInjectedPayload describes steering messages appended before the next LLM call.
type SteeringInjectedPayload struct {
	Count           int
	TotalContentLen int
}

type InterruptKind string

const (
	InterruptKindSteering InterruptKind = "steering"
	InterruptKindGraceful InterruptKind = "graceful"
	InterruptKindHard     InterruptKind = "hard_abort"
)

// InterruptReceivedPayload describes accepted turn-control input.
type InterruptReceivedPayload struct {
	Kind       InterruptKind
	Role       string
	ContentLen int
	QueueDepth int
	HintLen    int
}

// ErrorPayload describes an execution error inside the agent loop.
type ErrorPayload struct {
	Stage   string
	Message string
}
