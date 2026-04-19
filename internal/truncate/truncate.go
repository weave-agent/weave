package truncate

import "strings"

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

func Truncate(input string, maxLines, maxBytes int) Result {
	if input == "" {
		return Result{}
	}

	lines := strings.Split(input, "\n")
	lineCount := len(lines)
	byteCount := len(input)

	if lineCount <= maxLines && byteCount <= maxBytes {
		return Result{
			Content:   input,
			Truncated: false,
			Lines:     lineCount,
			Bytes:     byteCount,
		}
	}

	truncated := false

	if lineCount > maxLines {
		lines = lines[:maxLines]
		truncated = true
	}

	content := strings.Join(lines, "\n")

	if len(content) > maxBytes {
		for len(content) > maxBytes && len(lines) > 0 {
			lines = lines[:len(lines)-1]
			content = strings.Join(lines, "\n")
			truncated = true
		}
		if len(lines) == 0 {
			return Result{
				Content:   "",
				Truncated: true,
				Lines:     0,
				Bytes:     0,
			}
		}
	}

	return Result{
		Content:   content,
		Truncated: truncated,
		Lines:     len(lines),
		Bytes:     len(content),
	}
}
