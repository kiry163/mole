package events

// Filter decides whether an event should pass through an EventChannel.
type Filter func(Event) bool

// MatchKind matches events whose kind is in kinds. Empty kinds match all events.
func MatchKind(kinds ...Kind) Filter {
	if len(kinds) == 0 {
		return matchAll
	}

	allowed := make(map[Kind]struct{}, len(kinds))
	for _, kind := range kinds {
		allowed[kind] = struct{}{}
	}

	return func(evt Event) bool {
		_, ok := allowed[evt.Kind]
		return ok
	}
}

func matchAll(Event) bool {
	return true
}

func matchesFilters(filters []Filter, evt Event) bool {
	for _, filter := range filters {
		if filter != nil && !filter(evt) {
			return false
		}
	}
	return true
}
