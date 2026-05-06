package sdk

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEvent(t *testing.T) {
	before := time.Now()
	evt := NewEvent("test.topic", "hello")
	after := time.Now()

	assert.Equal(t, "test.topic", evt.Topic)
	assert.Equal(t, "hello", evt.Payload)
	assert.True(t, !evt.Timestamp.Before(before) && !evt.Timestamp.After(after))
}

func TestEventNilPayload(t *testing.T) {
	evt := NewEvent("empty", nil)
	assert.Nil(t, evt.Payload)
}

func TestConfigInterface(t *testing.T) {
	cfg := FilePathConfig("/path/to/.weave.yaml")
	assert.Equal(t, "/path/to/.weave.yaml", cfg.FilePath())
}

func TestExtensionFunc(t *testing.T) {
	var subscribed bool

	ext := NewExtensionFunc("test-ext", func(b Bus) error {
		subscribed = true
		return nil
	})

	assert.Equal(t, "test-ext", ext.Name())

	bus := &BusMock{}
	require.NoError(t, ext.Subscribe(bus))

	assert.True(t, subscribed)
}

func TestExtensionFuncSatisfiesInterface(t *testing.T) {
	var _ Extension = NewExtensionFunc("x", func(Bus) error { return nil })

	ext := Extension(NewExtensionFunc("check", func(b Bus) error {
		b.Publish(NewEvent("fired", nil))
		return nil
	}))
	bus := &BusMock{
		PublishFunc: func(e Event) {},
	}
	require.NoError(t, ext.Subscribe(bus))

	calls := bus.PublishCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "fired", calls[0].Event.Topic)
}

func TestExtensionFuncMultipleSubscriptions(t *testing.T) {
	var calls []string

	ext := NewExtensionFunc("multi", func(b Bus) error {
		calls = append(calls, "called")

		b.Publish(NewEvent("e1", 1))
		b.Publish(NewEvent("e2", 2))

		return nil
	})

	bus := &BusMock{
		PublishFunc: func(e Event) {},
	}
	require.NoError(t, ext.Subscribe(bus))

	require.Len(t, calls, 1)

	pubCalls := bus.PublishCalls()
	require.Len(t, pubCalls, 2)
	assert.Equal(t, "e1", pubCalls[0].Event.Topic)
	assert.Equal(t, "e2", pubCalls[1].Event.Topic)
}

func TestExtensionFunc_CloseNil(t *testing.T) {
	ext := NewExtensionFunc("no-close", func(Bus) error { return nil })
	require.NoError(t, ext.Close())
}

func TestExtensionFuncWithClose(t *testing.T) {
	var closed bool

	ext := NewExtensionFuncWithClose("with-close", func(Bus) error { return nil }, func() error {
		closed = true
		return nil
	})

	require.NoError(t, ext.Close())
	assert.True(t, closed)
}

func TestExtensionFuncWithClose_Error(t *testing.T) {
	ext := NewExtensionFuncWithClose("close-err", func(Bus) error { return nil }, func() error {
		return errors.New("close failed")
	})

	require.Error(t, ext.Close())
}

func TestExtensionFunc_UsesBusOn(t *testing.T) {
	ext := NewExtensionFunc("on-ext", func(b Bus) error {
		b.On("test.topic", func(e Event) error { return nil })
		return nil
	})

	bus := &BusMock{}
	require.NoError(t, ext.Subscribe(bus))

	onCalls := bus.OnCalls()
	require.Len(t, onCalls, 1)
	assert.Equal(t, "test.topic", onCalls[0].Topic)
}

func TestExtensionFunc_UsesBusOff(t *testing.T) {
	handler := func(e Event) error { return nil }

	ext := NewExtensionFunc("off-ext", func(b Bus) error {
		b.On("topic", handler)
		b.Off(handler)

		return nil
	})

	bus := &BusMock{}
	require.NoError(t, ext.Subscribe(bus))

	offCalls := bus.OffCalls()
	require.Len(t, offCalls, 1)

	onCalls := bus.OnCalls()
	require.Len(t, onCalls, 1)
}
