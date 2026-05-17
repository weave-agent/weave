package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDo_SuccessOnFirstAttempt(t *testing.T) {
	callCount := 0

	err := Do(context.Background(), Config{MaxRetries: 2, BaseDelay: 1 * time.Millisecond},
		func(error) bool { return true },
		func() error {
			callCount++
			return nil
		},
	)

	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestDo_SuccessOnRetry(t *testing.T) {
	callCount := 0

	err := Do(context.Background(), Config{MaxRetries: 3, BaseDelay: 1 * time.Millisecond},
		func(error) bool { return true },
		func() error {
			callCount++
			if callCount < 3 {
				return errors.New("transient")
			}

			return nil
		},
	)

	require.NoError(t, err)
	assert.Equal(t, 3, callCount)
}

func TestDo_MaxRetriesExceeded(t *testing.T) {
	callCount := 0

	err := Do(context.Background(), Config{MaxRetries: 2, BaseDelay: 1 * time.Millisecond},
		func(error) bool { return true },
		func() error {
			callCount++
			return errors.New("persistent")
		},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "max retries exceeded")
	assert.Equal(t, 3, callCount)
}

func TestDo_NonRetriableError(t *testing.T) {
	callCount := 0

	err := Do(context.Background(), Config{MaxRetries: 5, BaseDelay: 1 * time.Millisecond},
		func(error) bool { return false },
		func() error {
			callCount++
			return errors.New("fatal")
		},
	)

	require.Error(t, err)
	assert.Equal(t, 1, callCount)
}

func TestDo_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0

	err := Do(ctx, Config{MaxRetries: 5, BaseDelay: 1 * time.Hour},
		func(error) bool { return true },
		func() error {
			callCount++
			if callCount == 1 {
				cancel()
			}

			return errors.New("transient")
		},
	)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDo_RetriablePredicate(t *testing.T) {
	callCount := 0

	err := Do(context.Background(), Config{MaxRetries: 2, BaseDelay: 1 * time.Millisecond},
		func(err error) bool { return err.Error() == "retry me" },
		func() error {
			callCount++
			if callCount == 1 {
				return errors.New("retry me")
			}

			return errors.New("do not retry")
		},
	)

	require.Error(t, err)
	assert.Equal(t, 2, callCount)
	assert.Equal(t, "do not retry", err.Error())
}

func TestCalculateDelay(t *testing.T) {
	cfg := Config{BaseDelay: 1 * time.Second, MaxDelay: 30 * time.Second, Multiplier: 2}

	assert.Equal(t, 1*time.Second, CalculateDelay(cfg, 0))
	assert.Equal(t, 2*time.Second, CalculateDelay(cfg, 1))
	assert.Equal(t, 4*time.Second, CalculateDelay(cfg, 2))
	assert.Equal(t, 8*time.Second, CalculateDelay(cfg, 3))
	assert.Equal(t, 16*time.Second, CalculateDelay(cfg, 4))
	assert.Equal(t, 30*time.Second, CalculateDelay(cfg, 5))  // capped
	assert.Equal(t, 30*time.Second, CalculateDelay(cfg, 10)) // capped
}

func TestCalculateDelay_NegativeAttempt(t *testing.T) {
	cfg := Config{BaseDelay: 1 * time.Second, MaxDelay: 30 * time.Second, Multiplier: 2}

	assert.Equal(t, 1*time.Second, CalculateDelay(cfg, -1))
}

func TestCalculateDelay_ZeroBase(t *testing.T) {
	cfg := Config{BaseDelay: 0, MaxDelay: 30 * time.Second, Multiplier: 2}

	assert.Equal(t, 0*time.Second, CalculateDelay(cfg, 0))
	assert.Equal(t, 0*time.Second, CalculateDelay(cfg, 5))
}
