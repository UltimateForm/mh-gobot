package util

import (
	"regexp"
	"strings"
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
