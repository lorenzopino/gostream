package quality

import "testing"

func TestDefaultProfilesAreValid(t *testing.T) {
	tests := []struct {
		name string
		prof MovieProfile
	}{
		{"quality-first", DefaultQualityFirstMovies()},
		{"size-first", DefaultSizeFirstMovies()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.prof.PriorityOrder == nil || len(tt.prof.PriorityOrder) == 0 {
				t.Error("PriorityOrder must not be empty")
			}
			if tt.prof.MinSeeders <= 0 {
				t.Errorf("MinSeeders must be > 0, got %d", tt.prof.MinSeeders)
			}
		})
	}
}

func TestDefaultTVProfilesAreValid(t *testing.T) {
	tests := []struct {
		name string
		prof TVProfile
	}{
		{"quality-first", DefaultQualityFirstTV()},
		{"size-first", DefaultSizeFirstTV()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.prof.PriorityOrder == nil || len(tt.prof.PriorityOrder) == 0 {
				t.Error("PriorityOrder must not be empty")
			}
			if tt.prof.FullpackBonus < 0 {
				t.Errorf("FullpackBonus must be >= 0, got %d", tt.prof.FullpackBonus)
			}
		})
	}
}
