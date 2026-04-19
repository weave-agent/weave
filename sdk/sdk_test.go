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

	ext := NewExtensionFunc("test-ext", func(b Bus) {
		subscribed = true
	})

	assert.Equal(t, "test-ext", ext.Name())

	bus := &BusMock{}
	ext.Subscribe(bus)

	assert.True(t, subscribed)
}

func TestExtensionFuncSatisfiesInterface(t *testing.T) {
	var _ Extension = NewExtensionFunc("x", func(Bus) {})

	ext := Extension(NewExtensionFunc("check", func(b Bus) {
		b.Publish(NewEvent("fired", nil))
	}))
	bus := &BusMock{
		PublishFunc: func(e Event) bool { return true },
	}
	ext.Subscribe(bus)

	calls := bus.PublishCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "fired", calls[0].Event.Topic)
}

func TestExtensionFuncMultipleSubscriptions(t *testing.T) {
	var calls []string

	ext := NewExtensionFunc("multi", func(b Bus) {
		calls = append(calls, "called")

		b.Publish(NewEvent("e1", 1))
		b.Publish(NewEvent("e2", 2))
	})

	bus := &BusMock{
		PublishFunc: func(e Event) bool { return true },
	}
	ext.Subscribe(bus)

	require.Len(t, calls, 1)

	pubCalls := bus.PublishCalls()
	require.Len(t, pubCalls, 2)
	assert.Equal(t, "e1", pubCalls[0].Event.Topic)
	assert.Equal(t, "e2", pubCalls[1].Event.Topic)
}

func TestExtensionFunc_CloseNil(t *testing.T) {
	ext := NewExtensionFunc("no-close", func(Bus) {})
	require.NoError(t, ext.Close())
}

func TestExtensionFuncWithClose(t *testing.T) {
	var closed bool

	ext := NewExtensionFuncWithClose("with-close", func(Bus) {}, func() error {
		closed = true
		return nil
	})

	require.NoError(t, ext.Close())
	assert.True(t, closed)
}

func TestExtensionFuncWithClose_Error(t *testing.T) {
	ext := NewExtensionFuncWithClose("close-err", func(Bus) {}, func() error {
		return errors.New("close failed")
	})

	require.Error(t, ext.Close())
}
