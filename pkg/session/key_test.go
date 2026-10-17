package session

import "testing"

type testScopeReader struct {
	scope *SessionScope
}

func (r testScopeReader) GetSessionScope(sessionKey string) *SessionScope {
	return CloneScope(r.scope)
}

func TestIsExplicitSessionKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"sk_v1_abc", true},
		{"agent:main:direct:user123", false},
		{"custom-key", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := IsExplicitSessionKey(tt.key); got != tt.want {
			t.Fatalf("IsExplicitSessionKey(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestBuildMainSessionKey(t *testing.T) {
	got := BuildMainSessionKey("Main")
	if !IsOpaqueSessionKey(got) {
		t.Fatalf("BuildMainSessionKey() = %q, want opaque key", got)
	}
	if got != BuildOpaqueSessionKey("agent:main:main") {
		t.Fatalf("BuildMainSessionKey() = %q, want stable main-key hash", got)
	}
}

func TestResolveAgentID_PrefersSessionScope(t *testing.T) {
	store := testScopeReader{
		scope: &SessionScope{
			Version: ScopeVersionV1,
			AgentID: "Support",
			Channel: "slack",
		},
	}

	if got := ResolveAgentID(store, "sk_v1_anything"); got != "support" {
		t.Fatalf("ResolveAgentID() = %q, want support", got)
	}
}

func TestResolveAgentID_WithoutSessionScope(t *testing.T) {
	if got := ResolveAgentID(nil, "sk_v1_anything"); got != "" {
		t.Fatalf("ResolveAgentID() = %q, want empty for a store without scope metadata", got)
	}
}
