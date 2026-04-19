package truncate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncate_EmptyInput(t *testing.T) {
	result := Truncate("", DefaultMaxLines, DefaultMaxBytes)
	assert.Equal(t, "", result.Content)
	assert.False(t, result.Truncated)
	assert.Equal(t, 0, result.Lines)
	assert.Equal(t, 0, result.Bytes)
}

func TestTruncate_UnderLimit(t *testing.T) {
	input := "hello\nworld\nfoo"
	result := Truncate(input, DefaultMaxLines, DefaultMaxBytes)
	assert.Equal(t, input, result.Content)
	assert.False(t, result.Truncated)
	assert.Equal(t, 3, result.Lines)
	assert.Equal(t, len(input), result.Bytes)
}

func TestTruncate_LineLimit(t *testing.T) {
	maxLines := 3
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "line"
	}
	input := strings.Join(lines, "\n")

	result := Truncate(input, maxLines, DefaultMaxBytes)
	assert.True(t, result.Truncated)
	assert.Equal(t, 3, result.Lines)
	assert.Equal(t, "line\nline\nline", result.Content)
}

func TestTruncate_ByteLimit(t *testing.T) {
	maxBytes := 10
	input := "hi\nshort\nthis line is way too long for the byte limit\nok"

	result := Truncate(input, DefaultMaxLines, maxBytes)
	assert.True(t, result.Truncated)
	assert.LessOrEqual(t, result.Bytes, maxBytes)
	for _, line := range strings.Split(result.Content, "\n") {
		assert.True(t, len(line) <= maxBytes, "line exceeds byte limit: %q", line)
	}
}

func TestTruncate_SingleHugeLine(t *testing.T) {
	maxBytes := 5
	input := strings.Repeat("x", 1000)

	result := Truncate(input, DefaultMaxLines, maxBytes)
	assert.True(t, result.Truncated)
	assert.Equal(t, "", result.Content)
	assert.Equal(t, 0, result.Lines)
	assert.Equal(t, 0, result.Bytes)
}

func TestTruncate_ExactLineBoundary(t *testing.T) {
	maxLines := 3
	input := "a\nb\nc"

	result := Truncate(input, maxLines, DefaultMaxBytes)
	assert.False(t, result.Truncated)
	assert.Equal(t, "a\nb\nc", result.Content)
	assert.Equal(t, 3, result.Lines)
}

func TestTruncate_ExactByteBoundary(t *testing.T) {
	maxBytes := 5
	input := "hello"

	result := Truncate(input, DefaultMaxLines, maxBytes)
	assert.False(t, result.Truncated)
	assert.Equal(t, "hello", result.Content)
	assert.Equal(t, 5, result.Bytes)
}

func TestTruncate_BothLimitsActive(t *testing.T) {
	maxLines := 2
	maxBytes := 5
	input := "aa\nbb\ncc\ndd"

	result := Truncate(input, maxLines, maxBytes)
	assert.True(t, result.Truncated)
	assert.LessOrEqual(t, result.Bytes, maxBytes)
	assert.LessOrEqual(t, result.Lines, maxLines)
}

func TestTruncate_NeverPartialLine(t *testing.T) {
	maxLines := 1
	maxBytes := 3
	input := "abcde\nfghij"

	result := Truncate(input, maxLines, maxBytes)
	// Even though 1 line is allowed, "abcde" exceeds 3 bytes
	// so we drop it and return empty
	assert.True(t, result.Truncated)
}
