package approval

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestStoreRegisterAndDecide(t *testing.T) {
	store := NewStore()
	defer store.Close()

	pending, err := store.Register(Request{Tool: "exec", Summary: "exec: ls", Pin: "command"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if pending.ID() == "" || pending.ID()[:3] != "ap_" {
		t.Fatalf("pending.ID() = %q, want ap_ prefixed", pending.ID())
	}

	listed := store.Pending()
	if len(listed) != 1 || listed[0].ID != pending.ID() || listed[0].Tool != "exec" {
		t.Fatalf("Pending() = %+v, want the registered request", listed)
	}

	var (
		outcome Outcome
		reason  string
		wg      sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		outcome, reason = pending.Wait(context.Background())
	}()

	if err := store.Decide(pending.ID(), OutcomeAllowed, "console", ""); err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	wg.Wait()
	if outcome != OutcomeAllowed || reason != "" {
		t.Fatalf("Wait() = (%q, %q), want (allowed, \"\")", outcome, reason)
	}
	if len(store.Pending()) != 0 {
		t.Fatal("a decided request must leave the queue")
	}

	// A second click on the same row is an error the console maps to 409.
	if err := store.Decide(pending.ID(), OutcomeDenied, "console", ""); !errors.Is(err, ErrAlreadyDecided) {
		t.Fatalf("second Decide() error = %v, want ErrAlreadyDecided", err)
	}
	if err := store.Decide("ap_nope", OutcomeDenied, "console", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown Decide() error = %v, want ErrNotFound", err)
	}
	if !store.WasDecided(pending.ID()) {
		t.Fatal("WasDecided() = false for a decided request, want true")
	}
	if store.WasDecided("ap_nope") {
		t.Fatal("WasDecided() = true for an unknown request, want false")
	}

	recent := store.Recent()
	if len(recent) != 1 || recent[0].Outcome != OutcomeAllowed || recent[0].DecidedBy != "console" {
		t.Fatalf("Recent() = %+v, want one allowed record", recent)
	}
}

func TestStoreWaitRemovesTheEntryWhenTheTurnStopsWaiting(t *testing.T) {
	store := NewStore()
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	pending, err := store.Register(Request{Tool: "exec"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	outcome, reason := pending.Wait(ctx)
	if outcome != OutcomeExpired {
		t.Fatalf("Wait() outcome = %q, want expired", outcome)
	}
	if reason == "" {
		t.Fatal("an expired request must carry a reason the model can read")
	}
	if len(store.Pending()) != 0 {
		t.Fatal("an expired request must leave the queue: the engine never asks again")
	}
	// The record carries the summary (Register derives it from the tool) but
	// never the arguments: those can hold secrets and the console does not need
	// them after the fact.
	if got := store.Recent(); len(got) != 1 || got[0].Outcome != OutcomeExpired || got[0].Summary != "exec" {
		t.Fatalf("Recent() = %+v, want one expired record for exec", got)
	}

	// A cancelled turn reports cancellation, not a timeout.
	cancelled, cancelTurn := context.WithCancel(context.Background())
	pending2, err := store.Register(Request{Tool: "exec"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	cancelTurn()
	if outcome, _ := pending2.Wait(cancelled); outcome != OutcomeCancelled {
		t.Fatalf("Wait() outcome = %q, want cancelled", outcome)
	}
}

func TestStoreCloseWakesEveryWaiter(t *testing.T) {
	store := NewStore()
	first, err := store.Register(Request{Tool: "exec"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	second, err := store.Register(Request{Tool: "write_file"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if first.ID() == second.ID() {
		t.Fatal("two concurrent requests must not share an id")
	}

	outcomes := make(chan Outcome, 2)
	go func() { outcome, _ := first.Wait(context.Background()); outcomes <- outcome }()
	go func() { outcome, _ := second.Wait(context.Background()); outcomes <- outcome }()

	store.Close()
	for range 2 {
		select {
		case outcome := <-outcomes:
			if outcome != OutcomeCancelled {
				t.Fatalf("outcome after Close() = %q, want cancelled", outcome)
			}
		case <-time.After(time.Second):
			t.Fatal("Close() left a waiter blocked")
		}
	}

	// A closed store refuses new work instead of accepting it and dropping it.
	if _, err := store.Register(Request{Tool: "exec"}); !errors.Is(err, ErrClosed) {
		t.Fatalf("Register() after Close() error = %v, want ErrClosed", err)
	}
}

func TestStoreRecentIsBoundedAndCarriesNoArguments(t *testing.T) {
	store := NewStore()
	defer store.Close()
	for range RecentLimit + 5 {
		pending, err := store.Register(Request{Tool: "exec", Arguments: map[string]any{"command": "secret"}, Summary: "exec: secret"})
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}
		if err := store.Decide(pending.ID(), OutcomeDenied, "console", "no"); err != nil {
			t.Fatalf("Decide() error = %v", err)
		}
	}
	recent := store.Recent()
	if len(recent) != RecentLimit {
		t.Fatalf("len(Recent()) = %d, want %d", len(recent), RecentLimit)
	}
	for _, record := range recent {
		if record.Outcome != OutcomeDenied {
			t.Fatalf("record = %+v, want denied", record)
		}
	}
	if len(recent[0].Summary) == 0 {
		t.Fatal("a record should carry the summary so the console can show what was decided")
	}
}

// TestStoreWaitTwiceDoesNotBlock proves a second Wait on the same handle cannot
// hang: the first Wait removes the entry, so the second must fall back to its
// own outcome rather than block on a channel nothing will send on.
func TestStoreWaitTwiceDoesNotBlock(t *testing.T) {
	store := NewStore()
	defer store.Close()

	pending, err := store.Register(Request{Tool: "exec"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if outcome, _ := pending.Wait(ctx); outcome != OutcomeCancelled {
		t.Fatalf("first Wait() outcome = %q, want cancelled", outcome)
	}

	second := make(chan Outcome, 1)
	go func() {
		outcome, _ := pending.Wait(ctx)
		second <- outcome
	}()
	select {
	case outcome := <-second:
		if outcome != OutcomeCancelled {
			t.Fatalf("second Wait() outcome = %q, want cancelled", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("a second Wait() on the same handle blocked forever")
	}
}

func TestStoreWaitPrefersADecisionOverItsOwnDeadline(t *testing.T) {
	// The decision and the waiter's context end can land together. Run it enough
	for range 100 {
		store := NewStore()
		pending, err := store.Register(Request{Tool: "exec"})
		if err != nil {
			t.Fatalf("Register() error = %v", err)
		}
		if err := store.Decide(pending.ID(), OutcomeAllowed, "console", ""); err != nil {
			t.Fatalf("Decide() error = %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		outcome, _ := pending.Wait(ctx)
		if outcome != OutcomeAllowed {
			t.Fatalf("Wait() outcome = %q, want the recorded decision %q", outcome, OutcomeAllowed)
		}
		recent := store.Recent()
		if len(recent) != 1 || recent[0].Outcome != OutcomeAllowed {
			t.Fatalf("Recent() = %+v, want exactly one allowed record", recent)
		}
		store.Close()
	}
}
