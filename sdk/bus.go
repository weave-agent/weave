package sdk

import "context"

type busContextKey struct{}

// WithBus returns a context with the given bus attached.
func WithBus(ctx context.Context, bus Bus) context.Context {
	return context.WithValue(ctx, busContextKey{}, bus)
}

// BusFromContext retrieves the bus from the context, or nil if not present.
func BusFromContext(ctx context.Context) Bus {
	if bus, ok := ctx.Value(busContextKey{}).(Bus); ok {
		return bus
	}

	return nil
}
