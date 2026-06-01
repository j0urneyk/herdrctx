package ui

import (
	"strings"
	"unicode/utf8"
)

func sanitizeDisplay(value string) string {
	return sanitizeDisplayText(value, false)
}

func sanitizeMultilineDisplay(value string) string {
	return sanitizeDisplayText(value, true)
}

func sanitizeDisplayText(value string, preserveNewlines bool) string {
	var b strings.Builder
	for i := 0; i < len(value); {
		if value[i] == '\x1b' {
			i = skipEscapeSequence(value, i+1)
			continue
		}

		r, size := utf8.DecodeRuneInString(value[i:])
		if r == utf8.RuneError && size == 0 {
			break
		}
		if preserveNewlines && r == '\n' {
			b.WriteRune(r)
		} else if !isControlRune(r) {
			b.WriteRune(r)
		}
		i += size
	}

	return b.String()
}

func skipEscapeSequence(value string, index int) int {
	if index >= len(value) {
		return index
	}

	switch value[index] {
	case '[':
		return skipUntilFinalByte(value, index+1)
	case ']':
		return skipUntilStringTerminator(value, index+1)
	case 'P', 'X', '^', '_':
		return skipUntilStringTerminator(value, index+1)
	default:
		return index + 1
	}
}

func skipUntilFinalByte(value string, index int) int {
	for index < len(value) {
		char := value[index]
		index++
		if char >= 0x40 && char <= 0x7e {
			return index
		}
	}

	return index
}

func skipUntilStringTerminator(value string, index int) int {
	for index < len(value) {
		if value[index] == '\a' {
			return index + 1
		}
		if value[index] == '\x1b' && index+1 < len(value) && value[index+1] == '\\' {
			return index + 2
		}
		index++
	}

	return index
}

func isControlRune(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}
