package approval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Outcome is what happened to a request.
type Outcome string

const (
	// OutcomeAllowed means a human allowed this one call.
	OutcomeAllowed Outcome = "allowed"
	// OutcomeAlwaysAllowed means a human allowed it and asked to remember it.
	OutcomeAlwaysAllowed Outcome = "always_allowed"
	// OutcomeDenied means a human refused it.
	OutcomeDenied Outcome = "denied"
	// OutcomeExpired means the turn (or the approval timeout) gave up waiting.
	OutcomeExpired Outcome = "expired"
	// OutcomeCancelled means the turn was cancelled or the process is stopping.
	OutcomeCancelled Outcome = "cancelled"
)

// RecentLimit bounds the decided-request history kept for the console.
const RecentLimit = 20

var (
	// ErrNotFound is returned for an id that was never registered.
	ErrNotFound = errors.New("approval request not found")
	// ErrAlreadyDecided is returned for an id that was already answered.
	ErrAlreadyDecided = errors.New("approval request already decided")
	// ErrClosed is returned once the store is shutting down.
	ErrClosed = errors.New("approval store is closed")
)

// Request is one tool call waiting for a decision.
type Request struct {
	ID        string         `json:"id"`
	Tool      string         `json:"tool"`
	Category  string         `json:"category"`
	Summary   string         `json:"summary"`
	Pin       string         `json:"pin,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Origin    Origin         `json:"origin"`
	CreatedAt time.Time      `json:"created_at"`
	Deadline  time.Time      `json:"deadline,omitempty"`
}

// Record is a decided request. It carries no arguments: the console does not
// need them and they can hold secrets.
type Record struct {
	ID        string    `json:"id"`
	Tool      string    `json:"tool"`
	Summary   string    `json:"summary"`
	Outcome   Outcome   `json:"outcome"`
	Reason    string    `json:"reason,omitempty"`
	DecidedBy string    `json:"decided_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	DecidedAt time.Time `json:"decided_at"`
}

type decision struct {
	outcome Outcome
	reason  string
}

type entry struct {
	req Request
	ch  chan decision
}

// Store holds the pending requests of one process. It is shared by the builtin
// approver (which waits) and the console API (which decides), and it outlives
// config reloads so a reload does not discard a request a human is looking at.
type Store struct {
	mu      sync.Mutex
	pending map[string]*entry
	recent  []Record
	closed  bool
	now     func() time.Time
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{pending: map[string]*entry{}, now: time.Now}
}

// Register adds a request and returns a handle to wait on it.
func (s *Store) Register(req Request) (*Pending, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	id, err := newRequestID()
	if err != nil {
		return nil, err
	}
	req.ID = id
	if req.Arguments != nil {
		// Own a private copy: the caller keeps its map and the store hands the
		// same one back out, so it must not alias a live call's arguments.
		args := make(map[string]any, len(req.Arguments))
		for key, value := range req.Arguments {
			args[key] = value
		}
		req.Arguments = args
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = s.now()
	}
	if req.Category == "" {
		req.Category = Category(req.Tool)
	}
	if req.Summary == "" {
		req.Summary = Summarize(req.Tool, req.Arguments)
	}
	e := &entry{req: req, ch: make(chan decision, 1)}
	s.pending[id] = e
	return &Pending{store: s, entry: e}, nil
}

// Pending returns a copy of the queue, oldest first.
func (s *Store) Pending() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, 0, len(s.pending))
	for _, e := range s.pending {
		out = append(out, e.req)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

// Recent returns the decided requests, newest first.
func (s *Store) Recent() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, len(s.recent))
	copy(out, s.recent)
	return out
}

// Decide answers a pending request. It reports ErrNotFound for an unknown id and
// ErrAlreadyDecided for one that was answered before, so the console can tell a
// stale row from a typo.
func (s *Store) Decide(id string, outcome Outcome, by, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.pending[id]
	if !ok {
		if s.wasDecidedLocked(id) {
			return fmt.Errorf("%w: %s", ErrAlreadyDecided, id)
		}
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	delete(s.pending, id)
	s.recordLocked(e.req, outcome, reason, by)
	// The buffered channel keeps this non-blocking even if the waiter is gone.
	e.ch <- decision{outcome: outcome, reason: reason}
	return nil
}

// Close wakes every waiter with OutcomeCancelled and refuses further work. It
// runs on shutdown so a stopping process cannot leave a turn waiting.
func (s *Store) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	for id, e := range s.pending {
		delete(s.pending, id)
		s.recordLocked(e.req, OutcomeCancelled, "the gateway is shutting down", "")
		e.ch <- decision{outcome: OutcomeCancelled, reason: "the gateway is shutting down"}
	}
}

func (s *Store) recordLocked(req Request, outcome Outcome, reason, by string) {
	s.recent = append([]Record{{
		ID:        req.ID,
		Tool:      req.Tool,
		Summary:   req.Summary,
		Outcome:   outcome,
		Reason:    reason,
		DecidedBy: by,
		CreatedAt: req.CreatedAt,
		DecidedAt: s.now(),
	}}, s.recent...)
	if len(s.recent) > RecentLimit {
		s.recent = s.recent[:RecentLimit]
	}
}

func (s *Store) wasDecidedLocked(id string) bool {
	for _, record := range s.recent {
		if record.ID == id {
			return true
		}
	}
	return false
}

// WasDecided reports whether the id is still in the decided-request history
// (the most recent RecentLimit decisions), which lets the console tell a stale
// row (409) from a typo (404). A decision older than that window degrades to
// 404.
func (s *Store) WasDecided(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.wasDecidedLocked(id)
}

// Pending is one registered request waiting for an answer.
type Pending struct {
	store *Store
	entry *entry
}

// ID is the opaque id the console sends back.
func (p *Pending) ID() string { return p.entry.req.ID }

// Request returns a copy of the request; its Arguments map is read-only and
// owned by the store.
func (p *Pending) Request() Request { return p.entry.req }

// Wait blocks until a human decides or ctx ends. When ctx ends the request is
// removed and recorded as expired (deadline) or cancelled (turn abort), because
// the engine never asks again and a queue must not keep a phantom row. If a
// decision or Close already resolved the request, that result is returned
// instead of the timeout outcome.
func (p *Pending) Wait(ctx context.Context) (Outcome, string) {
	select {
	case d := <-p.entry.ch:
		return d.outcome, d.reason
	case <-ctx.Done():
		outcome := OutcomeCancelled
		reason := "the turn stopped waiting for approval"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			outcome = OutcomeExpired
			reason = "approval timed out"
		}
		if !p.store.cancel(p.entry.req.ID, outcome, reason) {
			// A decision (or Close, or another waiter) already resolved this
			// handle. Take a buffered result if there is one; otherwise fall
			// back to this waiter's own outcome so a second Wait never blocks
			// on a channel nothing will send on.
			select {
			case d := <-p.entry.ch:
				return d.outcome, d.reason
			default:
				return outcome, reason
			}
		}
		return outcome, reason
	}
}

// cancel removes the entry and records the timeout outcome. It reports whether
// this call removed the entry: false means a decision or Close already resolved
// the request, and the waiter must take that result instead.
func (s *Store) cancel(id string, outcome Outcome, reason string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.pending[id]
	if !ok {
		// A decision landed at the same moment; that decision wins.
		return false
	}
	delete(s.pending, id)
	s.recordLocked(e.req, outcome, reason, "")
	return true
}

func newRequestID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate approval request id: %w", err)
	}
	return "ap_" + hex.EncodeToString(buf), nil
}
