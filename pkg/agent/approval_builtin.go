package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kiry163/mole/pkg/approval"
	"github.com/kiry163/mole/pkg/config"
	runtimeevents "github.com/kiry163/mole/pkg/events"
	"github.com/kiry163/mole/pkg/logger"
)

// BuiltinWebUIApproval is the name of the built-in hook that asks the console.
const BuiltinWebUIApproval = "webui_approval"

func init() {
	if err := RegisterBuiltinHook(BuiltinWebUIApproval, newWebUIApprovalHook); err != nil {
		panic(fmt.Sprintf("register builtin hook %s: %v", BuiltinWebUIApproval, err))
	}
}

// webUIApprovalHook implements ToolApprover by applying tools.approval and, for
// the calls that need a human, waiting on the shared store.
type webUIApprovalHook struct {
	policy approval.Policy
	store  *approval.Store
	events runtimeevents.Bus
}

func newWebUIApprovalHook(_ context.Context, deps BuiltinHookDeps, _ config.BuiltinHookConfig) (any, error) {
	hook := &webUIApprovalHook{}
	if deps.Config != nil {
		hook.policy = approval.Policy{
			Rules:       deps.Config.Tools.Approval.Rules,
			AlwaysAllow: deps.Config.Tools.Approval.AlwaysAllow,
		}
	}
	hook.store = deps.Approvals
	hook.events = deps.Events
	return hook, nil
}

func (h *webUIApprovalHook) ApproveTool(ctx context.Context, req *ToolApprovalRequest) (ApprovalDecision, error) {
	if req == nil {
		// Fail closed: this package's posture is that an approver without a
		// request to inspect cannot approve anything.
		return ApprovalDecision{Approved: false, Reason: "approval request is nil"}, nil
	}
	origin := approvalOrigin(req)
	decision := h.policy.Decide(req.Tool, req.Arguments, origin)
	switch decision.Action {
	case approval.ActionAllow:
		return ApprovalDecision{Approved: true}, nil
	case approval.ActionDeny:
		h.emitDecided(req, origin, "", string(approval.OutcomeDenied), decision.Reason, "", 0)
		return ApprovalDecision{Approved: false, Reason: decision.Reason}, nil
	}

	if h.store == nil {
		// Fail closed: an approver without a queue cannot ask anyone.
		reason := fmt.Sprintf("%s needs approval, but the approval queue is not available", req.Tool)
		return ApprovalDecision{Approved: false, Reason: reason}, nil
	}

	pending, err := h.store.Register(approval.Request{
		Tool:      req.Tool,
		Arguments: req.Arguments,
		Summary:   approval.Summarize(req.Tool, req.Arguments),
		Pin:       decision.Pin,
		Origin:    origin,
		Deadline:  deadlineFrom(ctx),
	})
	if err != nil {
		return ApprovalDecision{Approved: false, Reason: fmt.Sprintf("approval queue unavailable: %v", err)}, nil
	}

	started := time.Now()
	h.emitRequested(req, pending.Request())
	outcome, reason := pending.Wait(ctx)
	waited := time.Since(started)
	h.emitDecided(req, origin, pending.ID(), string(outcome), reason, declinedBy(outcome), waited)

	switch outcome {
	case approval.OutcomeAllowed, approval.OutcomeAlwaysAllowed:
		return ApprovalDecision{Approved: true}, nil
	default:
		if strings.TrimSpace(reason) == "" {
			reason = string(outcome)
		}
		return ApprovalDecision{Approved: false, Reason: reason}, nil
	}
}

func approvalOrigin(req *ToolApprovalRequest) approval.Origin {
	origin := approval.Origin{}
	if req.Meta.TurnID != "" {
		origin.TurnID = req.Meta.TurnID
	}
	origin.SessionKey = req.Meta.SessionKey
	origin.Iteration = req.Meta.Iteration
	if req.Context != nil && req.Context.Inbound != nil {
		origin.Channel = req.Context.Inbound.Channel
		origin.ChatID = req.Context.Inbound.ChatID
		origin.SenderID = req.Context.Inbound.SenderID
	}
	return origin
}

func deadlineFrom(ctx context.Context) time.Time {
	if deadline, ok := ctx.Deadline(); ok {
		return deadline
	}
	return time.Time{}
}

func declinedBy(outcome approval.Outcome) string {
	switch outcome {
	case approval.OutcomeAllowed, approval.OutcomeAlwaysAllowed, approval.OutcomeDenied:
		return "console"
	default:
		return ""
	}
}

func (h *webUIApprovalHook) emitRequested(source *ToolApprovalRequest, req approval.Request) {
	if h.events == nil {
		return
	}
	h.events.PublishNonBlocking(runtimeevents.Event{
		Kind:     runtimeevents.KindAgentToolApprovalRequested,
		Time:     time.Now(),
		Source:   runtimeevents.Source{Component: "agent", Name: source.Meta.AgentID},
		Scope:    runtimeevents.Scope{AgentID: source.Meta.AgentID, SessionKey: req.Origin.SessionKey, TurnID: req.Origin.TurnID, Channel: req.Origin.Channel, ChatID: req.Origin.ChatID, SenderID: req.Origin.SenderID},
		Severity: approvalRequestedEventSeverity(),
		Payload: ToolApprovalRequestedPayload{
			RequestID:  req.ID,
			Tool:       req.Tool,
			Category:   req.Category,
			Summary:    req.Summary,
			Origin:     originLabel(req.Origin),
			Channel:    req.Origin.Channel,
			ChatID:     req.Origin.ChatID,
			SessionKey: req.Origin.SessionKey,
			TurnID:     req.Origin.TurnID,
			Iteration:  req.Origin.Iteration,
		},
	})
}

func (h *webUIApprovalHook) emitDecided(req *ToolApprovalRequest, origin approval.Origin, requestID, outcome, reason, decidedBy string, waited time.Duration) {
	if h.events == nil {
		return
	}
	h.events.PublishNonBlocking(runtimeevents.Event{
		Kind:     runtimeevents.KindAgentToolApprovalDecided,
		Time:     time.Now(),
		Source:   runtimeevents.Source{Component: "agent", Name: req.Meta.AgentID},
		Scope:    runtimeevents.Scope{AgentID: req.Meta.AgentID, SessionKey: origin.SessionKey, TurnID: origin.TurnID, Channel: origin.Channel, ChatID: origin.ChatID, SenderID: origin.SenderID},
		Severity: approvalDecidedEventSeverity(outcome),
		Payload: ToolApprovalDecidedPayload{
			RequestID: requestID,
			Tool:      req.Tool,
			Outcome:   outcome,
			Reason:    reason,
			DecidedBy: decidedBy,
			WaitedMS:  waited.Milliseconds(),
		},
	})
	logger.InfoCF("approval", "tool approval decided", map[string]any{
		"tool": req.Tool, "outcome": outcome, "reason": reason, "decided_by": decidedBy, "waited_ms": waited.Milliseconds(),
	})
}

// approvalRequestedEventSeverity and approvalDecidedEventSeverity are the one
// place the approval severities are decided. The hook publishes straight to the
// bus with one of these values, so runtimeSeverityForAgentEvent never sees the
// approval kinds and must not repeat the rule.
func approvalRequestedEventSeverity() runtimeevents.Severity {
	return runtimeevents.SeverityInfo
}

func approvalDecidedEventSeverity(outcome string) runtimeevents.Severity {
	if outcome == string(approval.OutcomeAllowed) || outcome == string(approval.OutcomeAlwaysAllowed) {
		return runtimeevents.SeverityInfo
	}
	return runtimeevents.SeverityWarn
}

func originLabel(origin approval.Origin) string {
	if origin.Channel == "" {
		return "interactive"
	}
	return origin.Channel
}
