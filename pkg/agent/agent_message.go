// Mole - Ultra-lightweight personal AI agent

package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/kiry163/mole/pkg/bus"
	"github.com/kiry163/mole/pkg/logger"
	"github.com/kiry163/mole/pkg/routing"
	"github.com/kiry163/mole/pkg/session"
	"github.com/kiry163/mole/pkg/utils"
)

func (al *AgentLoop) buildContinuationTarget(msg bus.InboundMessage) (*continuationTarget, error) {
	route, _, err := al.resolveMessageRoute(msg)
	if err != nil {
		return nil, err
	}
	allocation := al.allocateRouteSession(route, msg)

	return &continuationTarget{
		SessionKey: resolveScopeKey(allocation.SessionKey, msg.SessionKey),
		Channel:    msg.Channel,
		ChatID:     msg.ChatID,
	}, nil
}

func (al *AgentLoop) prepareInboundMessageForAgent(
	ctx context.Context,
	msg bus.InboundMessage,
) bus.InboundMessage {
	msg = bus.NormalizeInboundMessage(msg)

	return msg
}

func (al *AgentLoop) processMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	msg = al.prepareInboundMessageForAgent(ctx, msg)

	// Add message preview to log (show full content for error messages)
	var logContent string
	if strings.Contains(msg.Content, "Error:") || strings.Contains(msg.Content, "error") {
		logContent = msg.Content // Full content for errors
	} else {
		logContent = utils.Truncate(msg.Content, 80)
	}
	logger.InfoCF(
		"agent",
		fmt.Sprintf("Processing message from %s:%s: %s", msg.Channel, msg.SenderID, logContent),
		map[string]any{
			"channel":     msg.Channel,
			"chat_id":     msg.ChatID,
			"sender_id":   msg.SenderID,
			"session_key": msg.SessionKey,
		},
	)

	route, agent, routeErr := al.resolveMessageRoute(msg)
	if routeErr != nil {
		return "", routeErr
	}

	allocation := al.allocateRouteSession(route, msg)

	// Resolve session key from the route allocation, while preserving explicit
	// agent-scoped keys supplied by the caller.
	scopeKey := resolveScopeKey(allocation.SessionKey, msg.SessionKey)
	sessionKey := scopeKey

	// Reset message-tool state for this round so we don't skip publishing due to a previous round.
	if tool, ok := agent.Tools.Get("message"); ok {
		if resetter, ok := tool.(interface{ ResetSentInRound(sessionKey string) }); ok {
			resetter.ResetSentInRound(sessionKey)
		}
	}

	logger.InfoCF("agent", "Routed message",
		map[string]any{
			"agent_id":           agent.ID,
			"scope_key":          scopeKey,
			"session_key":        sessionKey,
			"route_channel":      route.Channel,
			"route_main_session": allocation.MainSessionKey,
		})

	opts := processOptions{
		Dispatch: DispatchRequest{
			SessionKey:     sessionKey,
			InboundContext: cloneInboundContext(&msg.Context),
			RouteResult:    cloneResolvedRoute(&route),
			SessionScope:   session.CloneScope(&allocation.Scope),
			UserMessage:    msg.Content,
			Media:          append([]string(nil), msg.Media...),
		},
		SenderID:                   msg.SenderID,
		SenderDisplayName:          msg.Sender.DisplayName,
		DefaultResponse:            defaultResponse,
		EnableSummary:              true,
		SendResponse:               false,
		AllowInterimWebChatPublish: true,
	}

	// context-dependent commands check their own Runtime fields and report
	// "unavailable" when the required capability is nil.
	if response, handled := al.handleCommand(ctx, msg, agent, &opts); handled {
		return response, nil
	}

	if pending := al.takePendingSkills(opts.Dispatch.SessionKey); len(pending) > 0 {
		opts.ForcedSkills = append(opts.ForcedSkills, pending...)
		logger.InfoCF("agent", "Applying pending skill override",
			map[string]any{
				"session_key": opts.Dispatch.SessionKey,
				"skills":      strings.Join(pending, ","),
			})
	}

	return al.runAgentLoop(ctx, agent, opts)
}

func (al *AgentLoop) resolveMessageRoute(msg bus.InboundMessage) (routing.ResolvedRoute, *AgentInstance, error) {
	registry := al.GetRegistry()
	inboundCtx := normalizedInboundContext(msg)
	route := registry.ResolveRoute(inboundCtx)

	agent := registry.GetDefaultAgent()
	if agent == nil {
		return routing.ResolvedRoute{}, nil, fmt.Errorf("no agent available")
	}

	return route, agent, nil
}

func (al *AgentLoop) allocateRouteSession(route routing.ResolvedRoute, msg bus.InboundMessage) session.Allocation {
	return session.AllocateRouteSession(session.AllocationInput{
		AgentID:       routing.DefaultAgentID,
		Context:       normalizedInboundContext(msg),
		SessionPolicy: route.SessionPolicy,
	})
}
