package providers

import "strings"

// NormalizeProvider normalizes a free-form provider label for grouping and
// diagnostics. Provider names are never validated against a built-in catalog.
func NormalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

// ModelKey returns a canonical "provider/model" key for deduplication.
func ModelKey(provider, model string) string {
	return NormalizeProvider(provider) + "/" + strings.ToLower(strings.TrimSpace(model))
}
