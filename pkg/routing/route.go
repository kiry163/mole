package routing

import (
	"strings"

	"github.com/kiry163/mole/pkg/bus"
	"github.com/kiry163/mole/pkg/config"
)

// SessionPolicy describes how a routed message should be mapped to a session.
type SessionPolicy struct {
	Dimensions    []string
	IdentityLinks map[string][]string
}

// ResolvedRoute is the result of resolving session scope for an inbound message.
type ResolvedRoute struct {
	Channel       string
	AccountID     string
	SessionPolicy SessionPolicy
}

// RouteResolver resolves the session policy for inbound messages.
type RouteResolver struct {
	cfg *config.Config
}

// NewRouteResolver creates a new route resolver.
func NewRouteResolver(cfg *config.Config) *RouteResolver {
	return &RouteResolver{cfg: cfg}
}

// ResolveRoute returns the normalized channel/account and the configured
// session policy for the inbound context.
func (r *RouteResolver) ResolveRoute(inbound bus.InboundContext) ResolvedRoute {
	return ResolvedRoute{
		Channel:   strings.ToLower(strings.TrimSpace(inbound.Channel)),
		AccountID: NormalizeAccountID(inbound.Account),
		SessionPolicy: SessionPolicy{
			Dimensions:    normalizeSessionDimensions(r.cfg.Session.Dimensions),
			IdentityLinks: cloneIdentityLinks(r.cfg.Session.IdentityLinks),
		},
	}
}

func normalizeSessionDimensions(dimensions []string) []string {
	if len(dimensions) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(dimensions))
	seen := make(map[string]struct{}, len(dimensions))
	for _, dimension := range dimensions {
		dimension = strings.ToLower(strings.TrimSpace(dimension))
		switch dimension {
		case "space", "chat", "topic", "sender":
		default:
			continue
		}
		if _, ok := seen[dimension]; ok {
			continue
		}
		seen[dimension] = struct{}{}
		normalized = append(normalized, dimension)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func cloneIdentityLinks(src map[string][]string) map[string][]string {
	if len(src) == 0 {
		return nil
	}
	cloned := make(map[string][]string, len(src))
	for canonical, ids := range src {
		dup := make([]string, len(ids))
		copy(dup, ids)
		cloned[canonical] = dup
	}
	return cloned
}
