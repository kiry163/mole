package approval

import (
	"fmt"
	"strings"
)

// ValidatePolicy reports why a tools.approval section cannot be used. It is
// called by the console's config save path, so a bad policy is rejected before
// it can silently disarm the approver.
func ValidatePolicy(rules []Rule, always []AllowEntry) []string {
	var errs []string
	for i, rule := range rules {
		if strings.TrimSpace(rule.Tool) == "" && strings.TrimSpace(rule.Category) == "" {
			errs = append(errs, fmt.Sprintf("tools.approval.rules[%d]: set tool or category", i))
		}
		// The runtime compares tools, categories and always-allow identifiers
		// verbatim, so a padded identifier can never match. Reject it instead
		// of saving a rule that silently never fires.
		errs = rejectPadded(errs, fmt.Sprintf("tools.approval.rules[%d].tool", i), rule.Tool)
		errs = rejectPadded(errs, fmt.Sprintf("tools.approval.rules[%d].category", i), rule.Category)
		errs = rejectPadded(errs, fmt.Sprintf("tools.approval.rules[%d].action", i), rule.Action)

		if category := strings.TrimSpace(rule.Category); category != "" && !knownCategory(category) {
			errs = append(errs, fmt.Sprintf("tools.approval.rules[%d].category: unknown category %q", i, rule.Category))
		}
		switch strings.TrimSpace(rule.Action) {
		case string(ActionAllow), string(ActionRequire), string(ActionDeny):
		default:
			errs = append(errs, fmt.Sprintf("tools.approval.rules[%d].action: want allow, require or deny, got %q", i, rule.Action))
		}
	}
	for i, entry := range always {
		if strings.TrimSpace(entry.Tool) == "" {
			errs = append(errs, fmt.Sprintf("tools.approval.always_allow[%d].tool: required", i))
		}
		errs = rejectPadded(errs, fmt.Sprintf("tools.approval.always_allow[%d].tool", i), entry.Tool)
		errs = rejectPadded(errs, fmt.Sprintf("tools.approval.always_allow[%d].arg", i), entry.Arg)
		if strings.TrimSpace(entry.Arg) == "" && strings.TrimSpace(entry.Value) != "" {
			errs = append(errs, fmt.Sprintf("tools.approval.always_allow[%d].arg: required when value is set", i))
		}
		if strings.TrimSpace(entry.Arg) != "" && entry.Value == "" {
			errs = append(errs, fmt.Sprintf("tools.approval.always_allow[%d].value: required when arg is set", i))
		}
	}
	return errs
}

// rejectPadded appends an error when value carries leading or trailing
// whitespace, naming the field so the console can point at it.
func rejectPadded(errs []string, field, value string) []string {
	if value != strings.TrimSpace(value) {
		return append(errs, fmt.Sprintf("%s: must not have leading or trailing whitespace, got %q", field, value))
	}
	return errs
}

func knownCategory(category string) bool {
	for _, known := range Categories {
		if category == known {
			return true
		}
	}
	return false
}
