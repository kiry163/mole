package agent

import (
	"github.com/kiry163/mole/pkg/bus"
	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/logger"
	"github.com/kiry163/mole/pkg/providers"
	"github.com/kiry163/mole/pkg/routing"
	"github.com/kiry163/mole/pkg/tools"
)

// AgentRegistry owns the single agent instance used by the process.
type AgentRegistry struct {
	cfg      *config.Config
	agent    *AgentInstance
	resolver *routing.RouteResolver
}

// NewAgentRegistry creates the single default agent.
func NewAgentRegistry(
	cfg *config.Config,
	provider providers.LLMProvider,
) *AgentRegistry {
	instance := NewAgentInstance(&cfg.Agents.Defaults, cfg, provider)
	logger.InfoCF("agent", "Created main agent",
		map[string]any{
			"workspace": instance.Workspace,
			"model":     instance.Model,
		})

	return &AgentRegistry{
		cfg:      cfg,
		agent:    instance,
		resolver: routing.NewRouteResolver(cfg),
	}
}

// GetAgent returns the single agent instance when agentID resolves to it.
func (r *AgentRegistry) GetAgent(agentID string) (*AgentInstance, bool) {
	if r.agent == nil || routing.NormalizeAgentID(agentID) != routing.DefaultAgentID {
		return nil, false
	}
	return r.agent, true
}

// ResolveRoute determines the session scope for the normalized inbound context.
func (r *AgentRegistry) ResolveRoute(inbound bus.InboundContext) routing.ResolvedRoute {
	return r.resolver.ResolveRoute(inbound)
}

// ListAgentIDs returns the single registered agent ID.
func (r *AgentRegistry) ListAgentIDs() []string {
	if r.agent == nil {
		return nil
	}
	return []string{routing.DefaultAgentID}
}

func (r *AgentRegistry) allowedMCPServers() map[string]struct{} {
	if r.agent == nil || r.agent.MCPServerAllowlist == nil {
		return nil
	}
	allowed := make(map[string]struct{}, len(r.agent.MCPServerAllowlist))
	for serverName := range r.agent.MCPServerAllowlist {
		allowed[serverName] = struct{}{}
	}
	return allowed
}

// ForEachTool calls fn when the agent has a tool registered under the given name.
// This is useful for propagating dependencies (e.g. MediaStore) to tools.
func (r *AgentRegistry) ForEachTool(name string, fn func(tools.Tool)) {
	if r.agent == nil {
		return
	}
	if tool, ok := r.agent.Tools.Get(name); ok {
		fn(tool)
	}
}

// Close releases resources held by the agent.
func (r *AgentRegistry) Close() {
	if r.agent == nil {
		return
	}
	if err := r.agent.Close(); err != nil {
		logger.WarnCF("agent", "Failed to close agent",
			map[string]any{"agent_id": r.agent.ID, "error": err.Error()})
	}
}

// GetDefaultAgent returns the single agent instance.
func (r *AgentRegistry) GetDefaultAgent() *AgentInstance {
	return r.agent
}
