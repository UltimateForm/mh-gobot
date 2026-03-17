package util

import "testing"

func TestHumanFormat(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{500, "500"},
		{999, "999"},
		{1000, "1K"},
		{1500, "1.5K"},
		{1100, "1.1K"},
		{9999, "10K"},
		{10000, "10K"},
		{100000, "100K"},
		{999999, "1000K"},
		{1000000, "1M"},
		{2500000, "2.5M"},
		{1000000000, "1G"},
		{-1000, "-1K"},
		{-500, "-500"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := HumanFormat(tt.input)
			if got != tt.expected {
				t.Errorf("HumanFormat(%d) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
