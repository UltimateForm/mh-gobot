package util

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var rePlayfabID = regexp.MustCompile(`^[A-Z0-9]{14,16}$`)

func IsPlayfabID(s string) bool {
	return rePlayfabID.MatchString(s)
}

func TruncateCodeString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	const suffix = "\n...\n```"
	truncated := s[:maxLen-len(suffix)] + suffix
	return truncated
}

// SplitChunks splits msg into chunks separated by newlines, keeping each chunk
// under limit runes. Lines that individually exceed the limit are kept as-is.
func SplitChunks(msg string, limit int) []string {
	lines := strings.Split(msg, "\n")
	var chunks []string
	for _, line := range lines {
		if len(chunks) == 0 {
			chunks = append(chunks, line)
			continue
		}
		candidate := chunks[len(chunks)-1] + "\n" + line
		if utf8.RuneCountInString(candidate) < limit {
			chunks[len(chunks)-1] = candidate
		} else {
			chunks = append(chunks, line)
		}
	}
	return chunks
}

// TruncateCodeStringByLine is like TruncateCodeString but only cuts at newline
// boundaries, so a row is never split mid-character.
func TruncateCodeStringByLine(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	const suffix = "\n...\n```"
	lines := strings.Split(s, "\n")
	result := ""
	for _, line := range lines {
		candidate := result + line + "\n"
		if len(candidate)+len(suffix) > maxLen {
			break
		}
		result = candidate
	}
	return result + suffix
}
