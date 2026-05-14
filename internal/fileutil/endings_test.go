package fileutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectLineEndings(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    string
	}{
		{"CRLF", []byte("line1\r\nline2\r\n"), "\r\n"},
		{"LF", []byte("line1\nline2\n"), "\n"},
		{"mixed CRLF first", []byte("line1\r\nline2\n"), "\r\n"},
		{"mixed LF first", []byte("line1\nline2\r\n"), "\n"},
		{"no endings", []byte("single line"), "\n"},
		{"empty", []byte{}, "\n"},
		{"CR only", []byte("line1\rline2\r"), "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectLineEndings(tt.content)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeToLF(t *testing.T) {
	tests := []struct {
		name        string
		content     []byte
		wantContent []byte
		wantEnding  string
	}{
		{"CRLF", []byte("line1\r\nline2\r\n"), []byte("line1\nline2\n"), "\r\n"},
		{"LF unchanged", []byte("line1\nline2\n"), []byte("line1\nline2\n"), "\n"},
		{"mixed", []byte("line1\r\nline2\n"), []byte("line1\nline2\n"), "\r\n"},
		{"empty", []byte{}, []byte{}, "\n"},
		{"no endings", []byte("single"), []byte("single"), "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotContent, gotEnding := NormalizeToLF(tt.content)
			assert.Equal(t, tt.wantContent, gotContent)
			assert.Equal(t, tt.wantEnding, gotEnding)
		})
	}
}

func TestRestoreLineEndings(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		ending  string
		want    []byte
	}{
		{"to CRLF", []byte("line1\nline2\n"), "\r\n", []byte("line1\r\nline2\r\n")},
		{"to LF no-op", []byte("line1\nline2\n"), "\n", []byte("line1\nline2\n")},
		{"empty", []byte{}, "\r\n", []byte{}},
		{"no endings", []byte("single"), "\r\n", []byte("single")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RestoreLineEndings(tt.content, tt.ending)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRoundTrip(t *testing.T) {
	original := []byte("line1\r\nline2\r\nline3\r\n")

	normalized, ending := NormalizeToLF(original)
	assert.Equal(t, []byte("line1\nline2\nline3\n"), normalized)
	assert.Equal(t, "\r\n", ending)

	restored := RestoreLineEndings(normalized, ending)
	assert.Equal(t, original, restored)
}
