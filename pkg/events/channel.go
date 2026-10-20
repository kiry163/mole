package events

import "context"

// EventChannel is a filtered view over an EventBus.
type EventChannel interface {
	Filter(filter Filter) EventChannel
	OfKind(kinds ...Kind) EventChannel

	Subscribe(ctx context.Context, opts SubscribeOptions, handler Handler) (Subscription, error)
	SubscribeChan(ctx context.Context, opts SubscribeOptions) (Subscription, <-chan Event, error)
}

type eventChannel struct {
	bus     *EventBus
	filters []Filter
}

// Filter returns a new EventChannel with filter appended.
func (c eventChannel) Filter(filter Filter) EventChannel {
	filters := append([]Filter(nil), c.filters...)
	if filter != nil {
		filters = append(filters, filter)
	}
	return eventChannel{bus: c.bus, filters: filters}
}

// OfKind returns a new EventChannel matching any of kinds.
func (c eventChannel) OfKind(kinds ...Kind) EventChannel {
	return c.Filter(MatchKind(kinds...))
}

// Subscribe registers handler for events matching this channel.
func (c eventChannel) Subscribe(ctx context.Context, opts SubscribeOptions, handler Handler) (Subscription, error) {
	if handler == nil {
		return nil, ErrNilHandler
	}
	return c.bus.subscribe(ctx, c.filters, opts, handler)
}

// SubscribeChan registers a channel subscription for events matching this channel.
func (c eventChannel) SubscribeChan(ctx context.Context, opts SubscribeOptions) (Subscription, <-chan Event, error) {
	sub, err := c.bus.subscribe(ctx, c.filters, opts, nil)
	if err != nil {
		return nil, nil, err
	}
	return sub, sub.(*eventSubscription).ch, nil
}
