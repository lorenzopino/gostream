package tmdb

import (
	"testing"
	"time"
)

func TestParseRelativeDate(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"negative months", "-6months"},
		{"positive months", "+12months"},
		{"negative year", "-1y"},
		{"positive days", "+30d"},
		{"negative days", "-30d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRelativeDate(tt.input)
			_, err := time.Parse("2006-01-02", result)
			if err != nil {
				t.Errorf("parseRelativeDate(%q) = %q, not a valid date: %v", tt.input, result, err)
			}
		})
	}
}

func TestParseRelativeDate_AbsolutePassthrough(t *testing.T) {
	got := parseRelativeDate("2024-06-15")
	if got != "2024-06-15" {
		t.Errorf("expected 2024-06-15, got %s", got)
	}
}

func TestParseRelativeDate_EmptyString(t *testing.T) {
	got := parseRelativeDate("")
	if got != "" {
		t.Errorf("expected empty string, got %s", got)
	}
}

func TestPagesOrDefault(t *testing.T) {
	v := 5
	if got := pagesOrDefault(&v, 1); got != 5 {
		t.Errorf("expected 5, got %d", got)
	}
	if got := pagesOrDefault(nil, 3); got != 3 {
		t.Errorf("expected 3 (default), got %d", got)
	}
	zero := 0
	if got := pagesOrDefault(&zero, 5); got != 5 {
		t.Errorf("expected 5 (default for zero), got %d", got)
	}
}
