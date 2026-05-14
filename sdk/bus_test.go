package sdk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithBus(t *testing.T) {
	t.Run("stores and retrieves bus", func(t *testing.T) {
		bus := &mockBus{}
		ctx := WithBus(context.Background(), bus)
		got := BusFromContext(ctx)
		assert.Equal(t, bus, got)
	})

	t.Run("returns nil when not present", func(t *testing.T) {
		got := BusFromContext(context.Background())
		assert.Nil(t, got)
	})

	t.Run("returns nil for wrong type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), busContextKey{}, "not a bus")
		got := BusFromContext(ctx)
		assert.Nil(t, got)
	})
}

type mockBus struct{}

func (m *mockBus) Publish(e Event)            {}
func (m *mockBus) On(topic string, h Handler) {}
func (m *mockBus) OnAll(h Handler)            {}
func (m *mockBus) Off(h Handler)              {}
func (m *mockBus) Close() error               { return nil }
