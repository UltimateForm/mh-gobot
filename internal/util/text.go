package util

func TruncateCodeString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Account for "..." and closing code block
	truncated := s[:maxLen-10] + "\n...\n```"
	return truncated
}
