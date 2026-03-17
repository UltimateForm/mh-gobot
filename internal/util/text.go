package util

import "regexp"

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
