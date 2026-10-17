package session

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/kiry163/mole/pkg/routing"
)

const sessionKeyV1Prefix = "sk_v1_"

// BuildOpaqueSessionKey returns a stable opaque session key derived from a
// canonical identity string.
func BuildOpaqueSessionKey(alias string) string {
	normalized := strings.TrimSpace(strings.ToLower(alias))
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(normalized))
	return sessionKeyV1Prefix + hex.EncodeToString(sum[:])
}

// IsOpaqueSessionKey returns true when the key matches the current opaque
// session-key format.
func IsOpaqueSessionKey(key string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(key)), sessionKeyV1Prefix)
}

// IsExplicitSessionKey reports whether key was supplied by a caller rather than
// derived from a chat route.
func IsExplicitSessionKey(key string) bool {
	return IsOpaqueSessionKey(key)
}

// ResolveAgentID returns the routed agent ID recorded in a session's scope
// metadata, or "" when the session carries none.
func ResolveAgentID(store any, sessionKey string) string {
	if scopeReader, ok := store.(interface {
		GetSessionScope(sessionKey string) *SessionScope
	}); ok {
		scope := scopeReader.GetSessionScope(sessionKey)
		if scope != nil && strings.TrimSpace(scope.AgentID) != "" {
			return routing.NormalizeAgentID(scope.AgentID)
		}
	}

	return ""
}

// BuildMainSessionKey returns the canonical opaque main-session key for an
// agent: the agent's shared fallback session.
func BuildMainSessionKey(agentID string) string {
	return BuildOpaqueSessionKey(fmt.Sprintf("agent:%s:main", routing.NormalizeAgentID(agentID)))
}

// CanonicalSessionIdentityID collapses an identity using identity_links when
// possible, then returns a normalized lowercase identifier.
func CanonicalSessionIdentityID(channel, rawID string, identityLinks map[string][]string) string {
	normalizedID := strings.TrimSpace(rawID)
	if normalizedID == "" {
		return ""
	}
	if linked := resolveLinkedPeerID(identityLinks, channel, normalizedID); linked != "" {
		normalizedID = linked
	}
	return strings.ToLower(normalizedID)
}

func resolveLinkedPeerID(identityLinks map[string][]string, channel, peerID string) string {
	if len(identityLinks) == 0 {
		return ""
	}
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return ""
	}

	candidates := make(map[string]bool)
	rawCandidate := strings.ToLower(peerID)
	if rawCandidate != "" {
		candidates[rawCandidate] = true
	}
	channel = strings.ToLower(strings.TrimSpace(channel))
	if channel != "" {
		candidates[fmt.Sprintf("%s:%s", channel, rawCandidate)] = true
	}
	if idx := strings.Index(rawCandidate, ":"); idx > 0 && idx < len(rawCandidate)-1 {
		candidates[rawCandidate[idx+1:]] = true
	}

	for canonical, ids := range identityLinks {
		canonicalName := strings.TrimSpace(canonical)
		if canonicalName == "" {
			continue
		}
		for _, id := range ids {
			normalized := strings.ToLower(strings.TrimSpace(id))
			if normalized != "" && candidates[normalized] {
				return canonicalName
			}
		}
	}
	return ""
}

// CanonicalScopeSignature returns a stable serialized representation of scope.
func CanonicalScopeSignature(scope SessionScope) string {
	parts := []string{
		fmt.Sprintf("v=%d", scope.Version),
		fmt.Sprintf("agent=%s", strings.TrimSpace(strings.ToLower(scope.AgentID))),
		fmt.Sprintf("channel=%s", strings.TrimSpace(strings.ToLower(scope.Channel))),
		fmt.Sprintf("account=%s", strings.TrimSpace(strings.ToLower(scope.Account))),
	}
	for _, dimension := range scope.Dimensions {
		dimension = strings.TrimSpace(strings.ToLower(dimension))
		if dimension == "" {
			continue
		}
		value := strings.TrimSpace(strings.ToLower(scope.Values[dimension]))
		parts = append(parts, fmt.Sprintf("%s=%s", dimension, value))
	}
	return strings.Join(parts, "|")
}

// BuildSessionKey returns the current opaque key for a structured session scope.
func BuildSessionKey(scope SessionScope) string {
	return BuildOpaqueSessionKey(CanonicalScopeSignature(scope))
}
