package approval

import "testing"

func TestPolicyDecideUsesCategoryDefaults(t *testing.T) {
	var p Policy
	tests := []struct {
		tool string
		want Action
	}{
		{"read_file", ActionAllow},
		{"list_dir", ActionAllow},
		{"web_search", ActionAllow},
		{"message", ActionAllow},
		{"spawn", ActionAllow},
		{"exec", ActionRequire},
		{"write_file", ActionRequire},
		{"edit_file", ActionRequire},
		{"append_file", ActionRequire},
		{"unknown_tool", ActionRequire},
		{"mcp__github__create_issue", ActionRequire},
	}
	for _, tc := range tests {
		if got := p.Decide(tc.tool, nil, Origin{}).Action; got != tc.want {
			t.Errorf("%s: action = %q, want %q", tc.tool, got, tc.want)
		}
	}
}

func TestPolicyDecideRulesAndAlwaysAllow(t *testing.T) {
	p := Policy{Rules: []Rule{
		{Tool: "exec", Action: "deny"},
		{Category: "write", Action: "allow"},
	}}
	if got := p.Decide("exec", nil, Origin{}).Action; got != ActionDeny {
		t.Fatalf("rule deny: %q", got)
	}
	if got := p.Decide("write_file", nil, Origin{}).Action; got != ActionAllow {
		t.Fatalf("rule allow by category: %q", got)
	}

	pinned := Policy{AlwaysAllow: []AllowEntry{{Tool: "exec", Arg: "command", Value: "git status"}}}
	if got := pinned.Decide("exec", map[string]any{"command": "git status"}, Origin{}).Action; got != ActionAllow {
		t.Fatalf("pinned command action = %q, want allow", got)
	}
	if got := pinned.Decide("exec", map[string]any{"command": "rm -rf /"}, Origin{}).Action; got != ActionRequire {
		t.Fatalf("different command action = %q, want require", got)
	}
	if got := pinned.Decide("exec", nil, Origin{}).Action; got != ActionRequire {
		t.Fatalf("missing arg action = %q, want require", got)
	}

	whole := Policy{AlwaysAllow: []AllowEntry{{Tool: "exec"}}}
	if got := whole.Decide("exec", map[string]any{"command": "rm -rf /"}, Origin{}).Action; got != ActionAllow {
		t.Fatalf("whole-tool allow action = %q, want allow", got)
	}
}

func TestPinArgFreezesDangerousArguments(t *testing.T) {
	tests := map[string]string{"exec": "command", "write_file": "path", "edit_file": "path", "append_file": "path", "read_file": "", "unknown": ""}
	for tool, want := range tests {
		if got := PinArg(tool); got != want {
			t.Errorf("PinArg(%q) = %q, want %q", tool, got, want)
		}
	}
}

// TestCategoriesAreAllAcceptedByTheValidator guards the exported Categories
// slice against drifting away from knownCategory: a category added to the slice
// must be saveable through the console.
func TestCategoriesAreAllAcceptedByTheValidator(t *testing.T) {
	for _, category := range Categories {
		if !knownCategory(category) {
			t.Fatalf("knownCategory(%q) = false, want the exported Categories to all be valid", category)
		}
		if errs := ValidatePolicy([]Rule{{Category: category, Action: "allow"}}, nil); len(errs) != 0 {
			t.Fatalf("ValidatePolicy(category %q) = %v, want none", category, errs)
		}
	}
}

func TestSummarizeIsOneLineAndNeverPanics(t *testing.T) {
	tests := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"exec", map[string]any{"command": "git status"}, "exec: git status"},
		{"write_file", map[string]any{"path": "/tmp/a.txt", "content": "x"}, "write_file: /tmp/a.txt"},
		{"exec", map[string]any{"command": 42}, `exec: {"command":42}`},
		{"exec", map[string]any{"command": "a\nb"}, "exec: a b"},
		{"exec", nil, "exec"},
	}
	for _, tc := range tests {
		if got := Summarize(tc.tool, tc.args); got != tc.want {
			t.Errorf("Summarize(%s) = %q, want %q", tc.tool, got, tc.want)
		}
	}
}
