package sdk

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOnBusReady(t *testing.T) {
	ResetBusSubscribers()
	defer ResetBusSubscribers()

	var called bool

	OnBusReady(func(_ Bus) {
		called = true
	})

	mockBus := &BusMock{
		OnFunc: func(topic string, handler Handler) {},
	}

	InvokeBusSubscribers(mockBus)
	assert.True(t, called)
}

func TestInvokeBusSubscribers_Multiple(t *testing.T) {
	ResetBusSubscribers()
	defer ResetBusSubscribers()

	count := 0

	OnBusReady(func(_ Bus) { count++ })
	OnBusReady(func(_ Bus) { count++ })

	mockBus := &BusMock{
		OnFunc: func(topic string, handler Handler) {},
	}

	InvokeBusSubscribers(mockBus)
	assert.Equal(t, 2, count)
}

func TestResetBusSubscribers(t *testing.T) {
	ResetBusSubscribers()
	defer ResetBusSubscribers()

	var called bool

	OnBusReady(func(_ Bus) { called = true })

	ResetBusSubscribers()

	mockBus := &BusMock{
		OnFunc: func(topic string, handler Handler) {},
	}

	InvokeBusSubscribers(mockBus)
	assert.False(t, called)
}

func TestOutputRedirectPayload(t *testing.T) {
	var w io.Writer

	payload := OutputRedirectPayload{Writer: w}
	assert.Nil(t, payload.Writer)
}
