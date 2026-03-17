package util

import (
	"strings"
	"testing"
)

func TestIsPlayfabID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// valid — 14 to 16 uppercase alphanumeric
		{"AAAAAAAAAAAAAAAA", true},  // 16 chars
		{"AAAAAAAAAAAAAAA", true},   // 15 chars
		{"AAAAAAAAAAAAAA", true},    // 14 chars
		{"A1B2C3D4E5F6G7H8", true},  // 16 chars, all valid A-Z0-9
		{"ABCDEF1234567890", true},  // 16 chars mixed
		// invalid
		{"AAAAAAAAAAAAA", false},    // 13 chars — too short
		{"AAAAAAAAAAAAAAAAA", false}, // 17 chars — too long
		{"aaaaaaaaaaaaaaaa", false}, // lowercase
		{"AAAAAAAAAAAAAA!!", false}, // special chars
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsPlayfabID(tt.input)
			if got != tt.want {
				t.Errorf("IsPlayfabID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncateCodeString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		// either exact expected output or a check
		wantLen    int
		wantSuffix string
		unchanged  bool
	}{
		{
			name:      "short string unchanged",
			input:     "```\nhello\n```",
			maxLen:    100,
			unchanged: true,
		},
		{
			name:      "exact length unchanged",
			input:     "```\nhello\n```",
			maxLen:    13,
			unchanged: true,
		},
		{
			name:       "long string truncated",
			input:      "```\n" + strings.Repeat("x", 2000) + "\n```",
			maxLen:     1024,
			wantLen:    1024,
			wantSuffix: "\n...\n```",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateCodeString(tt.input, tt.maxLen)
			if tt.unchanged {
				if got != tt.input {
					t.Errorf("expected unchanged, got %q", got)
				}
				return
			}
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
			if tt.wantSuffix != "" && !strings.HasSuffix(got, tt.wantSuffix) {
				t.Errorf("expected suffix %q, got %q", tt.wantSuffix, got)
			}
		})
	}
}
