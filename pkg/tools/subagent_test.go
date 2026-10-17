package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestSubagentManager_WithoutSpawnerReturnsReadableError pins the degradation
// path when the subagent tools are registered but no spawner was injected: the
// task must fail with a message naming the missing wiring instead of hanging or
// reporting an empty success.
func TestSubagentManager_WithoutSpawnerReturnsReadableError(t *testing.T) {
	m := NewSubagentManager("test-model")

	results := make(chan *ToolResult, 1)
	if _, err := m.Spawn(
		context.Background(),
		"do something",
		"task-1",
		"",
		"",
		"",
		func(_ context.Context, res *ToolResult) { results <- res },
	); err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}

	select {
	case res := <-results:
		if res == nil {
			t.Fatal("callback received nil result")
		}
		if !res.IsError {
			t.Fatalf("result should be an error, got: %#v", res)
		}
		if !strings.Contains(res.ForLLM, "spawner") {
			t.Fatalf("error should name the missing spawner, got: %q", res.ForLLM)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the subagent failure callback")
	}
}
