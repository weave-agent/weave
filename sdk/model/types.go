package model

import (
	"fmt"
	"os"
)

// ThinkingLevel represents the reasoning depth for a model request.
type ThinkingLevel string

const (
	ThinkingOff     ThinkingLevel = "off"
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
	ThinkingXHigh   ThinkingLevel = "xhigh"
)

// AllThinkingLevels is the ordered list of all thinking levels.
var AllThinkingLevels = []ThinkingLevel{
	ThinkingOff, ThinkingMinimal, ThinkingLow,
	ThinkingMedium, ThinkingHigh, ThinkingXHigh,
}

// ModelDef describes a model's metadata and capabilities.
type ModelDef struct {
	ID            string
	Provider      string
	DisplayName   string
	Reasoning     bool
	SupportsXHigh bool
	ContextWindow int
	MaxTokens     int
	Default       bool
}

// StreamOptions configures per-request behavior for provider streaming.
type StreamOptions struct {
	Model         string
	ThinkingLevel ThinkingLevel
	MaxTokens     int64
}

// StreamOption is a functional option for configuring per-request stream behavior.
type StreamOption func(*StreamOptions)

// NewStreamOptions creates StreamOptions with defaults, applying any given options.
func NewStreamOptions(opts ...StreamOption) *StreamOptions {
	so := &StreamOptions{
		ThinkingLevel: ThinkingOff,
	}
	for _, o := range opts {
		o(so)
	}

	return so
}

// WithModel sets the model for this request.
func WithModel(model string) StreamOption {
	return func(o *StreamOptions) { o.Model = model }
}

// WithThinkingLevel sets the thinking level for this request.
func WithThinkingLevel(level ThinkingLevel) StreamOption {
	return func(o *StreamOptions) { o.ThinkingLevel = level }
}

// WithMaxTokens sets the max output tokens for this request.
func WithMaxTokens(n int64) StreamOption {
	return func(o *StreamOptions) { o.MaxTokens = n }
}

// ClampForModel returns the level capped to what the model supports.
func ClampForModel(level ThinkingLevel, m ModelDef) ThinkingLevel {
	if level == ThinkingXHigh && !m.SupportsXHigh {
		return ThinkingHigh
	}

	return level
}

// DefaultThinkingLevel reads the initial thinking level from WEAVE_THINKING_LEVEL,
// falling back to ThinkingMedium.
func DefaultThinkingLevel() ThinkingLevel {
	if v := os.Getenv("WEAVE_THINKING_LEVEL"); v != "" {
		if lvl, err := ParseThinkingLevel(v); err == nil {
			return lvl
		}
	}

	return ThinkingMedium
}

// ParseThinkingLevel converts a string to a ThinkingLevel, returning an error if invalid.
func ParseThinkingLevel(s string) (ThinkingLevel, error) {
	for _, l := range AllThinkingLevels {
		if string(l) == s {
			return l, nil
		}
	}

	return "", fmt.Errorf("invalid thinking level %q (valid: off, minimal, low, medium, high, xhigh)", s)
}
