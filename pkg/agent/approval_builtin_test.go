package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kiry163/mole/pkg/approval"
	"github.com/kiry163/mole/pkg/config"
	runtimeevents "github.com/kiry163/mole/pkg/events"
	"github.com/kiry163/mole/pkg/tools"
)

// newApprovalTestLoop builds a loop whose hooks come from cfg (so the builtin is
// mounted) and whose approval requests are answerable by the returned store.
func newApprovalTestLoop(t *testing.T, provider *toolHookProvider, mut func(*config.Config)) (*AgentLoop, *AgentInstance, *approval.Store, func()) {
	t.Helper()
	al, agentInstance, cleanup := newHookTestLoop(t, provider)
	cfg := al.GetConfig()
	if mut != nil {
		mut(cfg)
	}
	store := approval.NewStore()
	// Hooks mount through ensureHooksInitialized (initOnce) and their timeouts come
	// from the config through configureHookManagerFromConfig; runAgentLoop calls
	// neither, so apply both here, after the mutation, exactly as a config reload
	// would (ReloadProviderAndConfig: configureHookManagerFromConfig then
	// ensureHooksInitialized). The real message entry points (Run, processMessage)
	// fill the same role for their first turn.
	al.SetApprovalStore(store)
	configureHookManagerFromConfig(al.hooks, cfg)
	if err := al.ensureHooksInitialized(context.Background()); err != nil {
		t.Fatalf("ensureHooksInitialized() error = %v", err)
	}
	return al, agentInstance, store, func() { store.Close(); cleanup() }
}

func TestBuiltinApproverAllowsSafeToolsWithoutAsking(t *testing.T) {
	// The provider must ask for the tool under test: its default call is
	// echo_text, which the policy would send to approval.
	provider := &toolHookProvider{toolName: "read_file", toolArgs: map[string]any{"path": "/tmp/x"}}
	al, agentInstance, store, cleanup := newApprovalTestLoop(t, provider, func(cfg *config.Config) {
		cfg.Hooks.Enabled = true
		cfg.Hooks.Builtins = map[string]config.BuiltinHookConfig{BuiltinWebUIApproval: {Enabled: true}}
		// Bounded so a policy that wrongly queued the read fails in seconds
		// instead of sitting on the default approval timeout.
		cfg.Hooks.Defaults.ApprovalTimeoutMS = 5000
	})
	defer cleanup()
	al.RegisterTool(&readOnlyEchoTool{})

	response, err := al.runAgentLoop(context.Background(), agentInstance, processOptions{
		SessionKey: "sess", Channel: "cli", ChatID: "direct", UserMessage: "run it",
		DefaultResponse: "done", EnableSummary: false, SendResponse: false,
	})
	if err != nil {
		t.Fatalf("runAgentLoop() error = %v", err)
	}
	if !strings.Contains(response, "read /tmp/x") {
		t.Fatalf("response = %q, want the read tool to have run", response)
	}
	if pending := store.Pending(); len(pending) != 0 {
		t.Fatalf("a read tool must not queue an approval, got %+v", pending)
	}
	if recent := store.Recent(); len(recent) != 0 {
		t.Fatalf("a read tool must not reach the queue at all, got %+v", recent)
	}
}

func TestBuiltinApproverQueuesAndHonoursADecision(t *testing.T) {
	provider := &toolHookProvider{toolName: "exec", toolArgs: map[string]any{"command": "echo approved-tool-ran"}}
	al, agentInstance, store, cleanup := newApprovalTestLoop(t, provider, func(cfg *config.Config) {
		cfg.Hooks.Enabled = true
		cfg.Hooks.Builtins = map[string]config.BuiltinHookConfig{BuiltinWebUIApproval: {Enabled: true}}
		cfg.Hooks.Defaults.ApprovalTimeoutMS = 5000
	})
	defer cleanup()
	al.RegisterTool(&dangerousEchoTool{})

	type result struct {
		response string
		err      error
	}
	done := make(chan result, 1)
	go func() {
		response, err := al.runAgentLoop(context.Background(), agentInstance, processOptions{
			SessionKey: "sess", Channel: "cli", ChatID: "direct", UserMessage: "run it",
			DefaultResponse: "done", EnableSummary: false, SendResponse: false,
		})
		done <- result{response: response, err: err}
	}()

	pending := waitForPendingApproval(t, store, 2*time.Second)
	if pending.Tool != "exec" {
		t.Fatalf("pending.Tool = %q, want exec", pending.Tool)
	}
	if pending.Pin != "command" {
		t.Fatalf("pending.Pin = %q, want command", pending.Pin)
	}
	if pending.Origin.Channel != "cli" {
		t.Fatalf("pending.Origin = %+v, want the turn's channel", pending.Origin)
	}
	if err := store.Decide(pending.ID, approval.OutcomeAllowed, "console", ""); err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("runAgentLoop() error = %v", got.err)
		}
		if !strings.Contains(got.response, "approved-tool-ran") {
			t.Fatalf("response = %q, want the tool to have run", got.response)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the turn did not finish after the approval")
	}
}

func TestBuiltinApproverExpiresWhenNobodyAnswers(t *testing.T) {
	provider := &toolHookProvider{toolName: "exec", toolArgs: map[string]any{"command": "echo approved-tool-ran"}}
	al, agentInstance, store, cleanup := newApprovalTestLoop(t, provider, func(cfg *config.Config) {
		cfg.Hooks.Enabled = true
		cfg.Hooks.Builtins = map[string]config.BuiltinHookConfig{BuiltinWebUIApproval: {Enabled: true}}
		cfg.Hooks.Defaults.ApprovalTimeoutMS = 300
	})
	defer cleanup()
	al.RegisterTool(&dangerousEchoTool{})

	sub, decisions, err := al.RuntimeEvents().OfKind(runtimeevents.KindAgentToolApprovalDecided).SubscribeChan(
		context.Background(),
		runtimeevents.SubscribeOptions{Name: "approval-expiry-test", Buffer: 2},
	)
	if err != nil {
		t.Fatalf("SubscribeChan() error = %v", err)
	}
	defer func() { _ = sub.Close() }()

	started := time.Now()
	response, err := al.runAgentLoop(context.Background(), agentInstance, processOptions{
		SessionKey: "sess", Channel: "cli", ChatID: "direct", UserMessage: "run it",
		DefaultResponse: "done", EnableSummary: false, SendResponse: false,
	})
	if err != nil {
		t.Fatalf("runAgentLoop() error = %v", err)
	}
	// The hook manager's own context arm can hand the turn back first
	// (runApprovalHook selects on the same deadline), so the model reads that
	// reason instead; either way it must be the timeout, never the pipeline's
	// fixed "denied by approval hook" prefix with an empty reason behind it.
	if !strings.Contains(response, "timed out") {
		t.Fatalf("response = %q, want the timeout reason the model reads", response)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("the turn waited %v, want the 300ms approval timeout to bound it", elapsed)
	}
	// The turn can finish before the approver's goroutine has run store.cancel,
	// so poll for the settled queue instead of reading it once.
	if record := waitForExpiredApproval(t, store, 2*time.Second); record.Reason != "approval timed out" {
		t.Fatalf("recorded reason = %q, want the store's reason for a deadline", record.Reason)
	}

	select {
	case evt := <-decisions:
		payload, ok := evt.Payload.(ToolApprovalDecidedPayload)
		if !ok {
			t.Fatalf("payload = %T, want ToolApprovalDecidedPayload", evt.Payload)
		}
		if payload.Outcome != string(approval.OutcomeExpired) {
			t.Fatalf("payload.Outcome = %q, want %q", payload.Outcome, approval.OutcomeExpired)
		}
		if payload.Reason != "approval timed out" {
			t.Fatalf("payload.Reason = %q, want the reason the approver returned", payload.Reason)
		}
		if payload.RequestID == "" {
			t.Fatal("payload.RequestID is empty, want the id the console decides on")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no approval_decided event was published")
	}
}

func TestBuiltinApproverDeniesWithoutAStore(t *testing.T) {
	provider := &toolHookProvider{toolName: "exec", toolArgs: map[string]any{"command": "echo approved-tool-ran"}}
	al, agentInstance, cleanup := newHookTestLoop(t, provider) // hooks come from the loop config
	defer cleanup()
	cfg := al.GetConfig()
	cfg.Hooks.Enabled = true
	cfg.Hooks.Builtins = map[string]config.BuiltinHookConfig{BuiltinWebUIApproval: {Enabled: true}}
	if err := al.ensureHooksInitialized(context.Background()); err != nil {
		t.Fatalf("ensureHooksInitialized() error = %v", err)
	}
	al.RegisterTool(&dangerousEchoTool{})

	response, err := al.runAgentLoop(context.Background(), agentInstance, processOptions{
		SessionKey: "sess", Channel: "cli", ChatID: "direct", UserMessage: "run it",
		DefaultResponse: "done", EnableSummary: false, SendResponse: false,
	})
	if err != nil {
		t.Fatalf("runAgentLoop() error = %v", err)
	}
	if !strings.Contains(response, "queue is not available") {
		t.Fatalf("response = %q, want a fail-closed denial naming the missing approval queue", response)
	}
}

func TestBuiltinApproverDeniesWithAReadableReason(t *testing.T) {
	// No store: the approver cannot ask anyone, so it must say so instead of
	// letting the call through.
	storeless := &webUIApprovalHook{}
	decision, err := storeless.ApproveTool(context.Background(), &ToolApprovalRequest{
		Tool:      "exec",
		Arguments: map[string]any{"command": "echo hi"},
	})
	if err != nil {
		t.Fatalf("ApproveTool() error = %v", err)
	}
	if decision.Approved {
		t.Fatal("a missing store must fail closed")
	}
	if !strings.Contains(decision.Reason, "queue is not available") {
		t.Fatalf("Reason = %q, want the missing-queue reason", decision.Reason)
	}

	// A store, but nobody answers: ctx is the only deadline the approver has.
	store := approval.NewStore()
	defer store.Close()
	hook := &webUIApprovalHook{store: store}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	started := time.Now()
	decision, err = hook.ApproveTool(ctx, &ToolApprovalRequest{
		Tool:      "exec",
		Arguments: map[string]any{"command": "echo hi"},
	})
	if err != nil {
		t.Fatalf("ApproveTool() error = %v", err)
	}
	if decision.Approved {
		t.Fatal("an unanswered request must fail closed once ctx ends")
	}
	if !strings.Contains(decision.Reason, "approval timed out") {
		t.Fatalf("Reason = %q, want the deadline reason the model reads", decision.Reason)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("ApproveTool() waited %v, want it bounded by the ctx deadline", elapsed)
	}
	if pending := store.Pending(); len(pending) != 0 {
		t.Fatalf("an expired request must leave the queue, got %+v", pending)
	}
}

func waitForPendingApproval(t *testing.T, store *approval.Store, timeout time.Duration) approval.Request {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pending := store.Pending(); len(pending) > 0 {
			return pending[0]
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no approval request appeared")
	return approval.Request{}
}

// waitForExpiredApproval waits for the approver's goroutine to settle the queue:
// the turn can return (the hook manager's context arm) while Wait's context arm
// still has to run store.cancel, so both the empty queue and the expired record
// are observed by polling rather than by a single read.
func waitForExpiredApproval(t *testing.T, store *approval.Store, timeout time.Duration) approval.Record {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var (
		pending []approval.Request
		recent  []approval.Record
	)
	for {
		pending = store.Pending()
		recent = store.Recent()
		if len(pending) == 0 && len(recent) == 1 && recent[0].Outcome == approval.OutcomeExpired {
			return recent[0]
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("after the approval timeout: Pending() = %+v, Recent() = %+v, want an empty queue and one expired record",
				pending, recent)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestBuiltinApproverDeniesANilRequest(t *testing.T) {
	hook := &webUIApprovalHook{}
	decision, err := hook.ApproveTool(context.Background(), nil)
	if err != nil {
		t.Fatalf("ApproveTool(nil) error = %v", err)
	}
	if decision.Approved {
		t.Fatal("ApproveTool(nil) must fail closed, got approved")
	}
}

// TestBuiltinApproverStaysMountedWhenAProcessHookFails is the regression for the
// disarmed-gate bug: a process hook that cannot start must not unmount the
// builtin approver, and a require-tool call must still queue an approval.
func TestBuiltinApproverStaysMountedWhenAProcessHookFails(t *testing.T) {
	provider := &toolHookProvider{toolName: "exec", toolArgs: map[string]any{"command": "echo approved-tool-ran"}}
	al, agentInstance, store, cleanup := newApprovalTestLoop(t, provider, func(cfg *config.Config) {
		cfg.Hooks.Enabled = true
		cfg.Hooks.Builtins = map[string]config.BuiltinHookConfig{BuiltinWebUIApproval: {Enabled: true}}
		cfg.Hooks.Defaults.ApprovalTimeoutMS = 5000
		cfg.Hooks.Processes = map[string]config.ProcessHookConfig{
			"broken_ipc": {
				Enabled:   true,
				Transport: "stdio",
				Command:   []string{"/nonexistent/mole-hook-does-not-exist"},
				Intercept: []string{"before_tool"},
			},
		}
	})
	defer cleanup()
	al.RegisterTool(&dangerousEchoTool{})

	if degraded := al.hookRuntime.getDegraded(); degraded == nil {
		t.Fatal("a failed process hook must be reported as a degraded runtime")
	}
	mounted := false
	for _, reg := range al.hooks.snapshotHooks() {
		if reg.Name == BuiltinWebUIApproval {
			mounted = true
		}
	}
	if !mounted {
		t.Fatalf("the builtin approver %q must stay mounted after a process hook fails", BuiltinWebUIApproval)
	}

	type result struct {
		response string
		err      error
	}
	done := make(chan result, 1)
	go func() {
		response, err := al.runAgentLoop(context.Background(), agentInstance, processOptions{
			SessionKey: "sess", Channel: "cli", ChatID: "direct", UserMessage: "run it",
			DefaultResponse: "done", EnableSummary: false, SendResponse: false,
		})
		done <- result{response: response, err: err}
	}()

	pending := waitForPendingApproval(t, store, 2*time.Second)
	if pending.Tool != "exec" {
		t.Fatalf("pending.Tool = %q, want exec", pending.Tool)
	}
	if err := store.Decide(pending.ID, approval.OutcomeAllowed, "console", ""); err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("runAgentLoop() error = %v", got.err)
		}
		if !strings.Contains(got.response, "approved-tool-ran") {
			t.Fatalf("response = %q, want the approved tool to have run", got.response)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the turn did not finish after the approval")
	}
}

// readOnlyEchoTool has a name the policy allows without asking.
type readOnlyEchoTool struct{}

func (t *readOnlyEchoTool) Name() string        { return "read_file" }
func (t *readOnlyEchoTool) Description() string { return "read" }
func (t *readOnlyEchoTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}
}
func (t *readOnlyEchoTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	return tools.NewToolResult("read " + fmt.Sprint(args["path"]))
}

// dangerousEchoTool has a name the policy requires approval for.
type dangerousEchoTool struct{}

func (t *dangerousEchoTool) Name() string        { return "exec" }
func (t *dangerousEchoTool) Description() string { return "exec" }
func (t *dangerousEchoTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}}
}
func (t *dangerousEchoTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	return tools.NewToolResult("ran " + fmt.Sprint(args["command"]))
}
