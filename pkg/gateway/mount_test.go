package gateway

import "testing"

func TestMountInvokedOnlyWhenProvided(t *testing.T) {
	env := &RuntimeEnv{Address: "127.0.0.1:18790"}
	var got *RuntimeEnv
	opts := Options{
		Mount: func(e *RuntimeEnv) error {
			got = e
			return nil
		},
	}
	if err := opts.mount(env); err != nil {
		t.Fatalf("mount returned error: %v", err)
	}
	if got != env {
		t.Fatal("Mount must receive the same RuntimeEnv that gateway built")
	}

	got = nil
	if err := (Options{}).mount(env); err != nil {
		t.Fatalf("nil Mount must be a no-op, got %v", err)
	}
	if got != nil {
		t.Fatal("nil Mount must not be invoked")
	}
}
