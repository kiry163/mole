package providers

import "testing"

func TestNormalizeProvider(t *testing.T) {
	tests := map[string]string{
		"  OpenAI ": "openai",
		"Custom":    "custom",
		"":          "",
	}
	for input, want := range tests {
		if got := NormalizeProvider(input); got != want {
			t.Errorf("NormalizeProvider(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestModelKey(t *testing.T) {
	if got := ModelKey(" OpenAI ", " GPT-4O "); got != "openai/gpt-4o" {
		t.Fatalf("ModelKey() = %q, want openai/gpt-4o", got)
	}
}
