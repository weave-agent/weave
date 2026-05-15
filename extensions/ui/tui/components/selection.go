package components

import "unicode"

// findWordBounds returns the start and end indices (rune offsets) of the word
// at the given column in the rune slice. If col lands on whitespace, it scans
// left then right for the nearest non-whitespace character. Returns (-1, -1) if
// no word is found.
func findWordBounds(runes []rune, col int) (start, end int) {
	if len(runes) == 0 {
		return -1, -1
	}

	if col < 0 {
		col = 0
	}

	if col >= len(runes) {
		col = len(runes) - 1
	}

	col = skipWhitespace(runes, col)
	if col < 0 {
		return -1, -1
	}

	start = col
	for start > 0 && !unicode.IsSpace(runes[start-1]) {
		start--
	}

	end = col + 1
	for end < len(runes) && !unicode.IsSpace(runes[end]) {
		end++
	}

	return start, end
}

// skipWhitespace moves from col to the nearest non-whitespace rune, scanning
// left first, then right. Returns -1 if all whitespace.
func skipWhitespace(runes []rune, col int) int {
	if !unicode.IsSpace(runes[col]) {
		return col
	}

	for i := col - 1; i >= 0; i-- {
		if !unicode.IsSpace(runes[i]) {
			return i
		}
	}

	for i := col + 1; i < len(runes); i++ {
		if !unicode.IsSpace(runes[i]) {
			return i
		}
	}

	return -1
}
