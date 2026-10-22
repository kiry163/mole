package agent

// EventMeta contains correlation fields shared by agent events and
// runtime events emitted from turn processing.
type EventMeta struct {
	AgentID      string
	TurnID       string
	ParentTurnID string
	SessionKey   string
	Iteration    int
	TracePath    string
	Source       string
	turnContext  *TurnContext
}
