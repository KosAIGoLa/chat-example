package service

import "testing"

func TestParseByteSize(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"1024", 1024},
		{"64K", 64 * 1024},
		{"128m", 128 * 1024 * 1024},
		{"1G", 1024 * 1024 * 1024},
		{" 2 g ", 2 * 1024 * 1024 * 1024},
	}

	for _, tt := range tests {
		got, err := parseByteSize(tt.in)
		if err != nil {
			t.Fatalf("parseByteSize(%q) unexpected error: %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("parseByteSize(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestParseByteSizeRejectsInvalid(t *testing.T) {
	for _, in := range []string{"", "0", "-1", "abc", "1T"} {
		if got, err := parseByteSize(in); err == nil {
			t.Fatalf("parseByteSize(%q) = %d, want error", in, got)
		}
	}
}
