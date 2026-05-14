package pathutil

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizePathCurlyQuotes(t *testing.T) {
	t.Run("double curly quotes", func(t *testing.T) {
		input := "“path/to/file”"
		got := NormalizePath(input)
		assert.Equal(t, "\"path/to/file\"", got)
	})

	t.Run("single curly quotes", func(t *testing.T) {
		input := "‘path/to/file’"
		got := NormalizePath(input)
		assert.Equal(t, "'path/to/file'", got)
	})

	t.Run("mixed curly quotes", func(t *testing.T) {
		input := "“hello ‘world’”"
		got := NormalizePath(input)
		assert.Equal(t, "\"hello 'world'\"", got)
	})
}

func TestNormalizePathUnicodeSpaces(t *testing.T) {
	t.Run("non-breaking space", func(t *testing.T) {
		input := "path to file"
		got := NormalizePath(input)
		assert.Equal(t, "path to file", got)
	})

	t.Run("thin space", func(t *testing.T) {
		input := "path to file"
		got := NormalizePath(input)
		assert.Equal(t, "path to file", got)
	})

	t.Run("en space", func(t *testing.T) {
		input := "path to file"
		got := NormalizePath(input)
		assert.Equal(t, "path to file", got)
	})

	t.Run("em space", func(t *testing.T) {
		input := "path to file"
		got := NormalizePath(input)
		assert.Equal(t, "path to file", got)
	})

	t.Run("narrow no-break space", func(t *testing.T) {
		input := "path to file"
		got := NormalizePath(input)
		assert.Equal(t, "path to file", got)
	})

	t.Run("figure space", func(t *testing.T) {
		input := "path to file"
		got := NormalizePath(input)
		assert.Equal(t, "path to file", got)
	})

	t.Run("multiple unicode spaces", func(t *testing.T) {
		input := "path to file name"
		got := NormalizePath(input)
		assert.Equal(t, "path to file name", got)
	})
}

func TestNormalizePathNFD(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("NFD normalization only applied on macOS")
	}

	// é in NFC (precomposed U+00E9) should become NFD (e + U+0301)
	nfc := "café.txt"  // café with precomposed é
	nfd := "café.txt" // café with decomposed e + combining acute

	got := NormalizePath(nfc)
	assert.Equal(t, nfd, got, "NFC input should be normalized to NFD")
}

func TestNormalizePathCombined(t *testing.T) {
	input := "“path to café”"
	got := NormalizePath(input)

	// All platforms: curly quotes and unicode spaces are normalized
	want := "\"path to café\""
	if runtime.GOOS == "darwin" {
		// macOS also applies NFD normalization
		want = "\"path to café\""
	}

	assert.Equal(t, want, got)
}

func TestNormalizePathNoOp(t *testing.T) {
	t.Run("plain ASCII path", func(t *testing.T) {
		input := "/path/to/file.txt"
		got := NormalizePath(input)
		assert.Equal(t, input, got)
	})

	t.Run("already straight quotes", func(t *testing.T) {
		input := "'path/to/file'"
		got := NormalizePath(input)
		assert.Equal(t, input, got)
	})

	t.Run("already regular spaces", func(t *testing.T) {
		input := "path to file"
		got := NormalizePath(input)
		assert.Equal(t, input, got)
	})

	t.Run("already NFD", func(t *testing.T) {
		input := "café.txt"
		got := NormalizePath(input)
		assert.Equal(t, input, got)
	})
}
