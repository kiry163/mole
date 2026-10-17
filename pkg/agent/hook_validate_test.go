package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kiry163/mole/pkg/config"
)

func TestValidateHooksConfigAcceptsTheDefaultSection(t *testing.T) {
	if errs := ValidateHooksConfig(config.DefaultConfig().Hooks); len(errs) != 0 {
		t.Fatalf("default hooks config rejected: %v", errs)
	}
}

func TestValidateHooksConfigRejectsUnmountableSections(t *testing.T) {
	base := config.DefaultConfig().Hooks

	tests := []struct {
		name    string
		mutate  func(*config.HooksConfig)
		wantSub string
	}{
		{"unregistered builtin", func(c *config.HooksConfig) {
			c.Builtins = map[string]config.BuiltinHookConfig{"nope": {Enabled: true}}
		}, "is not registered"},
		{"unsupported transport", func(c *config.HooksConfig) {
			c.Processes = map[string]config.ProcessHookConfig{
				"p": {Enabled: true, Transport: "grpc", Command: []string{"x"}, Observe: []string{"turn_start"}},
			}
		}, "unsupported transport"},
		{"missing command", func(c *config.HooksConfig) {
			c.Processes = map[string]config.ProcessHookConfig{
				"p": {Enabled: true, Observe: []string{"turn_start"}},
			}
		}, "command is required"},
		{"no modes", func(c *config.HooksConfig) {
			c.Processes = map[string]config.ProcessHookConfig{
				"p": {Enabled: true, Command: []string{"x"}},
			}
		}, "no hook modes enabled"},
		{"bad intercept", func(c *config.HooksConfig) {
			c.Processes = map[string]config.ProcessHookConfig{
				"p": {Enabled: true, Command: []string{"x"}, Intercept: []string{"nope"}},
			}
		}, "unsupported intercept"},
		{"bad observe", func(c *config.HooksConfig) {
			c.Processes = map[string]config.ProcessHookConfig{
				"p": {Enabled: true, Command: []string{"x"}, Observe: []string{"not_a_kind"}},
			}
		}, "unsupported observe"},
		{"zero observer timeout", func(c *config.HooksConfig) {
			c.Defaults.ObserverTimeoutMS = 0
		}, "observer_timeout_ms"},
		{"negative interceptor timeout", func(c *config.HooksConfig) {
			c.Defaults.InterceptorTimeoutMS = -1
		}, "interceptor_timeout_ms"},
		{"zero approval timeout", func(c *config.HooksConfig) {
			c.Defaults.ApprovalTimeoutMS = 0
		}, "approval_timeout_ms"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			cfg.Builtins = nil
			cfg.Processes = nil
			tc.mutate(&cfg)

			errs := ValidateHooksConfig(cfg)
			if len(errs) == 0 {
				t.Fatal("expected a validation error")
			}
			if !strings.Contains(strings.Join(errs, "; "), tc.wantSub) {
				t.Fatalf("errors = %v, want one containing %q", errs, tc.wantSub)
			}
		})
	}
}

func TestValidateHooksConfigIgnoresDisabledSection(t *testing.T) {
	cfg := config.DefaultConfig().Hooks
	cfg.Enabled = false
	cfg.Defaults.ObserverTimeoutMS = 0
	cfg.Builtins = map[string]config.BuiltinHookConfig{"nope": {Enabled: true}}

	if errs := ValidateHooksConfig(cfg); len(errs) != 0 {
		t.Fatalf("a disabled hooks section must not be validated, got %v", errs)
	}
}

func TestValidateHooksConfigRejectsABuiltinShadowedByAProcess(t *testing.T) {
	cfg := config.DefaultConfig().Hooks
	cfg.Enabled = true
	cfg.Builtins = map[string]config.BuiltinHookConfig{
		"shadowed": {Enabled: true, Config: json.RawMessage(`{}`)},
	}
	cfg.Processes = map[string]config.ProcessHookConfig{
		"shadowed": {Enabled: true, Transport: "stdio", Command: []string{"true"}, Intercept: []string{"before_tool"}},
	}
	if err := RegisterBuiltinHook("shadowed", func(ctx context.Context, _ BuiltinHookDeps, spec config.BuiltinHookConfig) (any, error) {
		return struct{}{}, nil
	}); err != nil {
		t.Fatalf("RegisterBuiltinHook() error = %v", err)
	}
	t.Cleanup(func() { unregisterBuiltinHook("shadowed") })

	errs := ValidateHooksConfig(cfg)
	if len(errs) == 0 {
		t.Fatal("ValidateHooksConfig() = none, want a name-collision error")
	}
	joined := strings.Join(errs, "; ")
	if !strings.Contains(joined, "already used by an enabled builtin hook") {
		t.Fatalf("ValidateHooksConfig() = %q, want the builtin-shadowing phrase", joined)
	}
}

func TestBuiltinHookNamesListsTheRegistry(t *testing.T) {
	if err := RegisterBuiltinHook("p3-test-builtin", func(context.Context, BuiltinHookDeps, config.BuiltinHookConfig) (any, error) {
		return struct{}{}, nil
	}); err != nil {
		t.Fatalf("RegisterBuiltinHook() error = %v", err)
	}
	t.Cleanup(func() { unregisterBuiltinHook("p3-test-builtin") })

	names := BuiltinHookNames()
	found := false
	for _, name := range names {
		if name == "p3-test-builtin" {
			found = true
		}
	}
	if !found {
		t.Fatalf("BuiltinHookNames() = %v, want the registered name", names)
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("BuiltinHookNames() = %v, want sorted", names)
		}
	}
}
