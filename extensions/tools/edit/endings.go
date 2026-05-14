package edit

import "bytes"

const (
	crlf = "\r\n"
	lf   = "\n"
)

// detectLineEndings scans content sequentially and returns the first line
// ending encountered. It returns "\r\n" if a CRLF sequence appears before
// any standalone LF, otherwise "\n" if any LF is found. If no line endings
// are present, it defaults to "\n".
func detectLineEndings(content []byte) string {
	for i := range content {
		if content[i] == '\r' && i+1 < len(content) && content[i+1] == '\n' {
			return crlf
		}

		if content[i] == '\n' {
			return lf
		}
	}

	return lf
}

// normalizeToLF replaces all "\r\n" sequences with "\n" and returns the
// LF-normalized content along with the original line ending detected.
func normalizeToLF(content []byte) ([]byte, string) {
	ending := detectLineEndings(content)

	if len(content) == 0 {
		return []byte{}, ending
	}

	return bytes.ReplaceAll(content, []byte(crlf), []byte(lf)), ending
}

// restoreLineEndings converts all "\n" sequences in content to the specified
// ending. If ending is "\n", content is returned unchanged.
func restoreLineEndings(content []byte, ending string) []byte {
	if len(content) == 0 || ending == lf {
		return content
	}

	return bytes.ReplaceAll(content, []byte(lf), []byte(ending))
}
