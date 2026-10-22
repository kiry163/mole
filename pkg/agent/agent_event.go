// Mole - Ultra-lightweight personal AI agent

package agent

import (
	"fmt"

	runtimeevents "github.com/kiry163/mole/pkg/events"
)

func (al *AgentLoop) newTurnEventScope(agentID, sessionKey string, turnCtx *TurnContext) turnEventScope {
	seq := al.turnSeq.Add(1)
	return turnEventScope{
		agentID:    agentID,
		sessionKey: sessionKey,
		turnID:     fmt.Sprintf("%s-turn-%d", agentID, seq),
		context:    cloneTurnContext(turnCtx),
	}
}

func (ts turnEventScope) meta(iteration int, source, tracePath string) EventMeta {
	return EventMeta{
		AgentID:     ts.agentID,
		TurnID:      ts.turnID,
		SessionKey:  ts.sessionKey,
		Iteration:   iteration,
		Source:      source,
		TracePath:   tracePath,
		turnContext: cloneTurnContext(ts.context),
	}
}

func (al *AgentLoop) emitEvent(kind runtimeevents.Kind, meta EventMeta, payload any) {
	clonedMeta := cloneEventMeta(meta)
	eventCtx := cloneTurnContext(clonedMeta.turnContext)
	evt := runtimeevents.Event{
		Kind:        kind,
		Source:      runtimeevents.Source{Component: "agent", Name: clonedMeta.AgentID},
		Scope:       runtimeScopeFromEventMeta(clonedMeta, eventCtx),
		Correlation: runtimeCorrelationFromEventMeta(clonedMeta),
		Severity:    runtimeSeverityForAgentEvent(kind, payload),
		Payload:     payload,
		Attrs:       runtimeAttrsFromEventMeta(clonedMeta),
	}

	if al == nil {
		return
	}

	al.publishRuntimeEvent(evt)
}

// RuntimeEventBus returns the runtime event bus used by the agent loop.
func (al *AgentLoop) RuntimeEventBus() runtimeevents.Bus {
	if al == nil {
		return nil
	}
	return al.runtimeEvents
}
