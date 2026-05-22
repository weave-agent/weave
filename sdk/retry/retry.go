package retry

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// JitterMode controls how randomness is applied to retry delays.
type JitterMode string

const (
	JitterNone JitterMode = "none"
	JitterFull JitterMode = "full"
)

// Config holds retry configuration.
type Config struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Multiplier float64
	Jitter     JitterMode
}

// DefaultConfig returns a default retry configuration with 10 retries,
// 1-second base delay, 30-second cap, 2x exponential multiplier, and no jitter.
func DefaultConfig() Config {
	return Config{
		MaxRetries: 10,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
		Multiplier: 2,
		Jitter:     JitterNone,
	}
}

// Do executes fn, retrying on retriable errors with exponential backoff.
// It returns nil if fn succeeds on any attempt. If all attempts fail,
// it returns an error wrapping the last failure.
func Do(ctx context.Context, cfg Config, isRetriable func(error) bool, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		if !isRetriable(err) {
			return err
		}

		lastErr = err

		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("retry canceled: %w", ctxErr)
		}

		if attempt < cfg.MaxRetries {
			delay := JitteredDelay(CalculateDelay(cfg, attempt), cfg.Jitter)
			timer := time.NewTimer(delay)

			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()

				return fmt.Errorf("retry canceled: %w", ctx.Err())
			}
		}
	}

	return fmt.Errorf("max retries exceeded (%d): %w", cfg.MaxRetries, lastErr)
}

// CalculateDelay computes the backoff delay for a given attempt.
func CalculateDelay(cfg Config, attempt int) time.Duration {
	if attempt < 0 {
		return cfg.BaseDelay
	}

	delay := float64(cfg.BaseDelay) * math.Pow(cfg.Multiplier, float64(attempt))
	if delay > float64(cfg.MaxDelay) {
		delay = float64(cfg.MaxDelay)
	}

	return time.Duration(delay)
}

// JitteredDelay applies jitter to a calculated delay based on the jitter mode.
// JitterNone returns the delay unchanged. JitterFull returns a random delay
// in the range [0, delay].
func JitteredDelay(delay time.Duration, jitter JitterMode) time.Duration {
	if jitter != JitterFull || delay <= 0 {
		return delay
	}

	//nolint:gosec // math/rand is sufficient for retry jitter; not used for cryptography.
	return time.Duration(rand.Float64() * float64(delay))
}
