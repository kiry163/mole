package approval

import (
	"strings"
	"testing"
)

func TestValidatePolicyAcceptsTheDefaultsAndTheDocumentedShapes(t *testing.T) {
	errs := ValidatePolicy(nil, nil)
	if len(errs) != 0 {
		t.Fatalf("ValidatePolicy(defaults) = %v, want none", errs)
	}
	errs = ValidatePolicy(
		[]Rule{{Tool: "exec", Action: "deny"}, {Category: CategoryWrite, Action: "allow"}},
		[]AllowEntry{{Tool: "exec", Arg: "command", Value: "git status"}, {Tool: "web_fetch"}},
	)
	if len(errs) != 0 {
		t.Fatalf("ValidatePolicy(valid) = %v, want none", errs)
	}
}

func TestValidatePolicyRejectsBadEntries(t *testing.T) {
	tests := []struct {
		name   string
		rules  []Rule
		always []AllowEntry
		want   string
	}{
		{"unknown action", []Rule{{Tool: "exec", Action: "maybe"}}, nil, "rules[0].action"},
		{"missing action", []Rule{{Tool: "exec"}}, nil, "rules[0].action"},
		{"rule without target", []Rule{{Action: "allow"}}, nil, "rules[0]"},
		{"unknown category", []Rule{{Category: "banana", Action: "allow"}}, nil, "rules[0].category"},
		{"padded rule tool", []Rule{{Tool: " exec ", Action: "allow"}}, nil, "rules[0].tool"},
		{"padded rule category", []Rule{{Category: " write ", Action: "allow"}}, nil, "rules[0].category"},
		{"padded rule action", []Rule{{Tool: "exec", Action: " allow "}}, nil, "rules[0].action"},
		{"allow entry without tool", nil, []AllowEntry{{Arg: "command", Value: "x"}}, "always_allow[0].tool"},
		{"padded always tool", nil, []AllowEntry{{Tool: " exec ", Arg: "command", Value: "x"}}, "always_allow[0].tool"},
		{"padded always arg", nil, []AllowEntry{{Tool: "exec", Arg: " command ", Value: "x"}}, "always_allow[0].arg"},
		{"arg without value", nil, []AllowEntry{{Tool: "exec", Arg: "command"}}, "always_allow[0].value"},
		{"value without arg", nil, []AllowEntry{{Tool: "exec", Value: "git status"}}, "always_allow[0].arg"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidatePolicy(tc.rules, tc.always)
			if len(errs) == 0 {
				t.Fatalf("ValidatePolicy() = none, want an error mentioning %q", tc.want)
			}
			joined := strings.Join(errs, "; ")
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("ValidatePolicy() = %q, want it to mention %q", joined, tc.want)
			}
		})
	}
}
