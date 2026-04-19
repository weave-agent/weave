package truncate

import (
	"fmt"
	"strings"
)

const (
	DefaultMaxLines = 2000
	DefaultMaxBytes = 50 * 1024 // 50KB
)

type Result struct {
	Content   string
	Truncated bool
	Lines     int
	Bytes     int
}

// Format returns the content with a truncation notice appended if truncation occurred.
func (r Result) Format() string {
	if !r.Truncated {
		return r.Content
	}
	return fmt.Sprintf("%s\n[output truncated: %d lines, %d bytes]", r.Content, r.Lines, r.Bytes)
}

func Truncate(input string, maxLines, maxBytes int) Result {
	if input == "" {
		return Result{}
	}

	lines := strings.Split(input, "\n")
	origLines := len(lines)
	origBytes := len(input)

	if origLines <= maxLines && origBytes <= maxBytes {
		return Result{
			Content:   input,
			Truncated: false,
			Lines:     origLines,
			Bytes:     origBytes,
		}
	}

	truncated := false

	if len(lines) > maxLines {
		lines = lines[:maxLines]
		truncated = true
	}

	// Check byte limit using cumulative computation (avoids quadratic re-joining)
	cumSize := 0
	cutoff := len(lines)
	for i, line := range lines {
		if i > 0 {
			cumSize++ // newline separator
		}
		cumSize += len(line)
		if cumSize > maxBytes {
			cutoff = i
			truncated = true
			break
		}
	}
	lines = lines[:cutoff]

	if len(lines) == 0 {
		return Result{
			Content:   "",
			Truncated: true,
			Lines:     origLines,
			Bytes:     origBytes,
		}
	}

	return Result{
		Content:   strings.Join(lines, "\n"),
		Truncated: truncated,
		Lines:     origLines,
		Bytes:     origBytes,
	}
}
