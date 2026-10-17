package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/kiry163/mole/pkg/approval"
	"github.com/kiry163/mole/pkg/config"
	"github.com/kiry163/mole/pkg/logger"
)

// ErrPolicyRejected marks an always_allow write the policy validator refused.
var ErrPolicyRejected = errors.New("approval policy rejected")

// ErrNotRememberable marks an always_allow whose request cannot be turned into a
// policy entry: the pinned argument is missing or not a string.
var ErrNotRememberable = errors.New("approval request cannot be remembered")

// registerApprovalRoutes binds the tool-approval queue. The console polls the
// list and posts decisions; there is no push channel.
func (h *Handler) registerApprovalRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/approvals", h.handleListApprovals)
	mux.HandleFunc("POST /api/approvals/{id}", h.handleDecideApproval)
}

// GET /api/approvals
func (h *Handler) handleListApprovals(w http.ResponseWriter, r *http.Request) {
	store := h.approvalStore()
	if store == nil {
		http.Error(w, "approval queue is not available", http.StatusServiceUnavailable)
		return
	}
	pending := store.Pending()
	if pending == nil {
		pending = []approval.Request{}
	}
	recent := store.Recent()
	if recent == nil {
		recent = []approval.Record{}
	}
	writeJSONResponse(w, map[string]any{"pending": pending, "recent": recent})
}

type approvalDecisionRequest struct {
	Decision string `json:"decision"`
}

// POST /api/approvals/{id} with allow, deny or always_allow.
func (h *Handler) handleDecideApproval(w http.ResponseWriter, r *http.Request) {
	store := h.approvalStore()
	if store == nil {
		http.Error(w, "approval queue is not available", http.StatusServiceUnavailable)
		return
	}

	var body approvalDecisionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	if err := decoder.Decode(&body); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	id := r.PathValue("id")
	decision := strings.TrimSpace(body.Decision)
	if decision != "allow" && decision != "deny" && decision != "always_allow" {
		http.Error(w, "decision must be allow, deny or always_allow", http.StatusBadRequest)
		return
	}

	if decision == "always_allow" {
		// Persist first: if the policy cannot be written the request stays
		// pending and the human can still answer it with a plain allow.
		appended, err := h.rememberApproval(id, store)
		if err != nil {
			switch {
			case errors.Is(err, approval.ErrNotFound), errors.Is(err, approval.ErrAlreadyDecided):
				http.Error(w, err.Error(), statusForApprovalError(err))
			case errors.Is(err, ErrNotRememberable), errors.Is(err, ErrPolicyRejected):
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		if appended {
			// The running approver read the policy when it mounted, so a newly
			// remembered entry only takes effect after a reload. The de-dup path
			// appended nothing and must not trigger one.
			h.requestReload()
		}
	}

	outcome := approval.OutcomeAllowed
	if decision == "deny" {
		outcome = approval.OutcomeDenied
	}
	if decision == "always_allow" {
		outcome = approval.OutcomeAlwaysAllowed
	}
	if err := store.Decide(id, outcome, "console", ""); err != nil {
		http.Error(w, err.Error(), statusForApprovalError(err))
		return
	}
	writeJSONResponse(w, map[string]any{"status": "ok", "outcome": string(outcome)})
}

// rememberApproval appends the pinned entry for a pending request to
// tools.approval.always_allow and saves the config, so the console's "always
// allow" survives a restart and a reload. It reports whether it appended a new
// entry (false on the de-duplication path), so the caller can reload only when
// the policy actually changed.
func (h *Handler) rememberApproval(id string, store *approval.Store) (bool, error) {
	var target approval.Request
	found := false
	for _, pending := range store.Pending() {
		if pending.ID == id {
			target, found = pending, true
			break
		}
	}
	if !found {
		if store.WasDecided(id) {
			return false, approval.ErrAlreadyDecided
		}
		return false, approval.ErrNotFound
	}

	entry, ok := approvalEntryFor(target)
	if !ok {
		return false, ErrNotRememberable
	}

	h.configMu.Lock()
	defer h.configMu.Unlock()
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		return false, err
	}
	for _, existing := range cfg.Tools.Approval.AlwaysAllow {
		if existing.Tool == entry.Tool && existing.Arg == entry.Arg && existing.Value == entry.Value {
			return false, nil
		}
	}
	cfg.Tools.Approval.AlwaysAllow = append(cfg.Tools.Approval.AlwaysAllow, entry)
	if errs := validateConfig(cfg); len(errs) > 0 {
		return false, fmt.Errorf("%w: %s", ErrPolicyRejected, strings.Join(errs, "; "))
	}
	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		return false, err
	}
	logger.InfoCF("approval", "remembered approval entry", map[string]any{"entry": entry.Describe(), "path": h.configPath})
	return true, nil
}

// approvalEntryFor turns a pending request into the entry to remember. Tools
// with a pinned argument must carry it: "always allow exec" without the command
// would mean "any shell command, forever".
func approvalEntryFor(req approval.Request) (approval.AllowEntry, bool) {
	if req.Tool == "" {
		return approval.AllowEntry{}, false
	}
	if req.Pin == "" {
		return approval.AllowEntry{Tool: req.Tool}, true
	}
	value, ok := req.Arguments[req.Pin].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return approval.AllowEntry{}, false
	}
	return approval.AllowEntry{Tool: req.Tool, Arg: req.Pin, Value: value}, true
}

func statusForApprovalError(err error) int {
	switch {
	case errors.Is(err, approval.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, approval.ErrAlreadyDecided):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
