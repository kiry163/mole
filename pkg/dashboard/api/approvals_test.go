package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kiry163/mole/pkg/approval"
	"github.com/kiry163/mole/pkg/config"
)

// newApprovalTestServer builds a console with a process-wide approval store and
// a reload counter, so a test can assert which decisions ask the gateway to
// re-read the policy.
func newApprovalTestServer(t *testing.T) (mux *http.ServeMux, store *approval.Store, reloads *int, configPath string) {
	t.Helper()
	configPath, cleanup := setupTestEnv(t)
	t.Cleanup(cleanup)
	store = approval.NewStore()
	t.Cleanup(store.Close)
	reloads = new(int)

	h := NewHandler(configPath)
	h.SetRuntime(&Runtime{
		Approvals: func() *approval.Store { return store },
		Reload:    func() { *reloads++ },
	})
	mux = http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux, store, reloads, configPath
}

func postApprovalDecision(t *testing.T, mux *http.ServeMux, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/approvals/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	return rec
}

// decodeDecisionResponse pins the `{"status":"ok","outcome":"..."}` contract the
// console client reads.
func decodeDecisionResponse(t *testing.T, rec *httptest.ResponseRecorder) (status, outcome string) {
	t.Helper()
	var resp struct {
		Status  string `json:"status"`
		Outcome string `json:"outcome"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v body=%s", err, rec.Body.String())
	}
	return resp.Status, resp.Outcome
}

func TestApprovalsListReportsTheQueue(t *testing.T) {
	mux, store, _, _ := newApprovalTestServer(t)
	pending, err := store.Register(approval.Request{
		Tool: "exec", Arguments: map[string]any{"command": "git status"}, Summary: "exec: git status", Pin: "command",
		Origin: approval.Origin{Channel: "cli", SenderID: "cli"},
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/approvals", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Pending []approval.Request `json:"pending"`
		Recent  []approval.Record  `json:"recent"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error = %v body=%s", err, rec.Body.String())
	}
	if len(resp.Pending) != 1 || resp.Pending[0].ID != pending.ID() || resp.Pending[0].Summary != "exec: git status" {
		t.Fatalf("pending = %+v", resp.Pending)
	}
	if resp.Pending[0].Pin != "command" {
		t.Fatalf("pin = %q, want command so the console can offer a pinned remember", resp.Pending[0].Pin)
	}
}

func TestApprovalDecisionDeliversToTheWaiter(t *testing.T) {
	mux, store, _, _ := newApprovalTestServer(t)
	pending, err := store.Register(approval.Request{Tool: "exec", Arguments: map[string]any{"command": "ls"}})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	outcomes := make(chan approval.Outcome, 1)
	go func() {
		outcome, _ := pending.Wait(context.Background())
		outcomes <- outcome
	}()

	rec := postApprovalDecision(t, mux, pending.ID(), `{"decision":"allow"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if status, outcome := decodeDecisionResponse(t, rec); status != "ok" || outcome != string(approval.OutcomeAllowed) {
		t.Fatalf("body = status %q outcome %q, want ok/allowed", status, outcome)
	}
	select {
	case outcome := <-outcomes:
		if outcome != approval.OutcomeAllowed {
			t.Fatalf("outcome = %q, want allowed", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("the waiter was not woken")
	}
}

func TestApprovalDecisionRejectsStaleAndUnknownRequests(t *testing.T) {
	mux, store, _, _ := newApprovalTestServer(t)
	pending, err := store.Register(approval.Request{Tool: "exec"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := store.Decide(pending.ID(), approval.OutcomeAllowed, "console", ""); err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	if rec := postApprovalDecision(t, mux, pending.ID(), `{"decision":"deny"}`); rec.Code != http.StatusConflict {
		t.Fatalf("stale status = %d body=%s, want 409", rec.Code, rec.Body.String())
	}
	if rec := postApprovalDecision(t, mux, "ap_missing", `{"decision":"deny"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown status = %d body=%s, want 404", rec.Code, rec.Body.String())
	}
	if rec := postApprovalDecision(t, mux, pending.ID(), `{"decision":"maybe"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad decision status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
}

func TestApprovalAlwaysAllowPersistsThePinnedEntry(t *testing.T) {
	mux, store, _, configPath := newApprovalTestServer(t)
	pending, err := store.Register(approval.Request{
		Tool: "exec", Arguments: map[string]any{"command": "git status"}, Pin: "command",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	go func() { _, _ = pending.Wait(context.Background()) }()

	rec := postApprovalDecision(t, mux, pending.ID(), `{"decision":"always_allow"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if status, outcome := decodeDecisionResponse(t, rec); status != "ok" || outcome != string(approval.OutcomeAlwaysAllowed) {
		t.Fatalf("body = status %q outcome %q, want ok/always_allowed", status, outcome)
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	entries := cfg.Tools.Approval.AlwaysAllow
	if len(entries) != 1 || entries[0].Tool != "exec" || entries[0].Arg != "command" || entries[0].Value != "git status" {
		t.Fatalf("always_allow = %+v, want the pinned exec entry", entries)
	}
}

func TestApprovalAlwaysAllowDoesNotDuplicateAnEntry(t *testing.T) {
	mux, store, _, configPath := newApprovalTestServer(t)
	// Seed the entry, then approve the same command again.
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	cfg.Tools.Approval.AlwaysAllow = []approval.AllowEntry{{Tool: "exec", Arg: "command", Value: "git status"}}
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	pending, err := store.Register(approval.Request{Tool: "exec", Arguments: map[string]any{"command": "git status"}, Pin: "command"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	go func() { _, _ = pending.Wait(context.Background()) }()
	if rec := postApprovalDecision(t, mux, pending.ID(), `{"decision":"always_allow"}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	reloaded, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := len(reloaded.Tools.Approval.AlwaysAllow); got != 1 {
		t.Fatalf("always_allow has %d entries, want 1 (the same command must not be remembered twice)", got)
	}
}

// TestApprovalAlwaysAllowReloadsOnlyWhenItAppends covers the gap that let a
// remembered entry never reach the running approver: the mount-time policy only
// changes after a reload, so appending must ask for one and de-duplicating must
// not.
func TestApprovalAlwaysAllowReloadsOnlyWhenItAppends(t *testing.T) {
	mux, store, reloads, _ := newApprovalTestServer(t)
	approve := func() {
		pending, err := store.Register(approval.Request{Tool: "exec", Arguments: map[string]any{"command": "git status"}, Pin: "command"})
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}
		go func() { _, _ = pending.Wait(context.Background()) }()
		if rec := postApprovalDecision(t, mux, pending.ID(), `{"decision":"always_allow"}`); rec.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
	}

	approve()
	if *reloads != 1 {
		t.Fatalf("reloads after the first always_allow = %d, want 1", *reloads)
	}
	approve()
	if *reloads != 1 {
		t.Fatalf("reloads after a duplicate always_allow = %d, want 1 (nothing was appended)", *reloads)
	}
}

func TestApprovalAlwaysAllowRejectsAnUnrememberableRequest(t *testing.T) {
	mux, store, reloads, _ := newApprovalTestServer(t)
	// exec is pinned on "command", but this call carries no string command, so it
	// cannot be turned into an "always allow" entry.
	pending, err := store.Register(approval.Request{Tool: "exec", Arguments: map[string]any{"command": 42}, Pin: "command"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	rec := postApprovalDecision(t, mux, pending.ID(), `{"decision":"always_allow"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
	if got := len(store.Pending()); got != 1 {
		t.Fatalf("pending = %d, want 1 (a rejected remember must leave the request answerable)", got)
	}
	if *reloads != 0 {
		t.Fatalf("reloads = %d, want 0 (nothing was written)", *reloads)
	}
}

func TestApprovalAlwaysAllowRemembersAWholeTool(t *testing.T) {
	mux, store, _, configPath := newApprovalTestServer(t)
	// web_fetch is not pinned, so "always allow" remembers the whole tool.
	pending, err := store.Register(approval.Request{Tool: "web_fetch", Arguments: map[string]any{"url": "https://example.com"}})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	go func() { _, _ = pending.Wait(context.Background()) }()

	if rec := postApprovalDecision(t, mux, pending.ID(), `{"decision":"always_allow"}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	entries := cfg.Tools.Approval.AlwaysAllow
	if len(entries) != 1 || entries[0].Tool != "web_fetch" || entries[0].Arg != "" || entries[0].Value != "" {
		t.Fatalf("always_allow = %+v, want a bare {tool: web_fetch} entry", entries)
	}
}

func TestApprovalAlwaysAllowRejectsADecidedRequest(t *testing.T) {
	mux, store, reloads, _ := newApprovalTestServer(t)
	pending, err := store.Register(approval.Request{Tool: "exec", Arguments: map[string]any{"command": "git status"}, Pin: "command"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := store.Decide(pending.ID(), approval.OutcomeAllowed, "console", ""); err != nil {
		t.Fatalf("Decide() error = %v", err)
	}

	rec := postApprovalDecision(t, mux, pending.ID(), `{"decision":"always_allow"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s, want 409", rec.Code, rec.Body.String())
	}
	if *reloads != 0 {
		t.Fatalf("reloads = %d, want 0 (nothing was written)", *reloads)
	}
}

func TestApprovalsUnavailableWithoutAStore(t *testing.T) {
	configPath, cleanup := setupTestEnv(t)
	defer cleanup()
	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/approvals", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("list status = %d, want 503", rec.Code)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/approvals/ap_any", strings.NewReader(`{"decision":"allow"}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("decide status = %d, want 503", rec.Code)
	}
}
