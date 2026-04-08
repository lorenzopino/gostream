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
			if tt.prof.MinSeeders == nil || *tt.prof.MinSeeders <= 0 {
				t.Errorf("MinSeeders must be > 0, got %v", tt.prof.MinSeeders)
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
			if tt.prof.FullpackBonus == nil || *tt.prof.FullpackBonus < 0 {
				t.Errorf("FullpackBonus must be >= 0, got %v", tt.prof.FullpackBonus)
			}
		})
	}
}

func TestSizeFloorNotExceedCeiling(t *testing.T) {
	movieProfiles := []struct {
		name string
		prof MovieProfile
	}{
		{"quality-first-movies", DefaultQualityFirstMovies()},
		{"size-first-movies", DefaultSizeFirstMovies()},
	}
	for _, mp := range movieProfiles {
		t.Run(mp.name, func(t *testing.T) {
			for res, floor := range mp.prof.SizeFloorGB {
				ceiling, ok := mp.prof.SizeCeilingGB[res]
				if !ok {
					continue
				}
				if floor > ceiling {
					t.Errorf("SizeFloorGB[%q]=%.1f exceeds SizeCeilingGB[%q]=%.1f", res, floor, res, ceiling)
				}
			}
		})
	}

	tvProfiles := []struct {
		name string
		prof TVProfile
	}{
		{"quality-first-tv", DefaultQualityFirstTV()},
		{"size-first-tv", DefaultSizeFirstTV()},
	}
	for _, tp := range tvProfiles {
		t.Run(tp.name, func(t *testing.T) {
			for res, floor := range tp.prof.SizeFloorGB {
				ceiling, ok := tp.prof.SizeCeilingGB[res]
				if !ok {
					continue
				}
				if floor > ceiling {
					t.Errorf("SizeFloorGB[%q]=%.1f exceeds SizeCeilingGB[%q]=%.1f", res, floor, res, ceiling)
				}
			}
		})
	}
}

func TestTVProfileMinSeeders4K(t *testing.T) {
	profiles := []struct {
		name string
		prof TVProfile
	}{
		{"quality-first", DefaultQualityFirstTV()},
		{"size-first", DefaultSizeFirstTV()},
	}
	for _, tp := range profiles {
		t.Run(tp.name, func(t *testing.T) {
			if tp.prof.MinSeeders4K == nil || *tp.prof.MinSeeders4K <= 0 {
				t.Errorf("MinSeeders4K must be > 0 for TV profile %s, got %v", tp.name, tp.prof.MinSeeders4K)
			}
		})
	}
}

func TestRemuxPenaltyIsNegative(t *testing.T) {
	profiles := []struct {
		name string
		prof MovieProfile
	}{
		{"quality-first", DefaultQualityFirstMovies()},
		{"size-first", DefaultSizeFirstMovies()},
	}
	for _, mp := range profiles {
		t.Run(mp.name, func(t *testing.T) {
			if mp.prof.ScoreWeights.Remux == nil {
				t.Fatal("Remux weight must not be nil")
			}
			if *mp.prof.ScoreWeights.Remux >= 0 {
				t.Errorf("Remux penalty must be negative (intentional exclusion), got %d", *mp.prof.ScoreWeights.Remux)
			}
		})
	}
}

func TestResolveMovies_NoConfig(t *testing.T) {
	var qc *QualityConfig
	prof := qc.ResolveMovies()
	if prof.MinSeeders == nil || *prof.MinSeeders != 15 {
		t.Errorf("expected MinSeeders=15, got %v", prof.MinSeeders)
	}
	if prof.Include720p != nil && *prof.Include720p {
		t.Error("quality-first should not include 720p by default")
	}
}

func TestResolveMovies_SizeFirst(t *testing.T) {
	qc := &QualityConfig{Profile: "size-first"}
	prof := qc.ResolveMovies()
	if prof.Include720p == nil || !*prof.Include720p {
		t.Error("size-first should include 720p")
	}
	if prof.PriorityOrder[0] != "720p" {
		t.Errorf("expected priority_order[0]=720p, got %s", prof.PriorityOrder[0])
	}
}

func TestResolveMovies_CustomOverrides(t *testing.T) {
	minSeeders := 25
	qc := &QualityConfig{
		Profile: "size-first",
		Profiles: map[string]ProfileSet{
			"size-first": {
				Movies: &MovieProfile{MinSeeders: &minSeeders},
			},
		},
	}
	prof := qc.ResolveMovies()
	if prof.MinSeeders == nil || *prof.MinSeeders != 25 {
		t.Errorf("expected MinSeeders=25 (custom override), got %v", prof.MinSeeders)
	}
	// Other fields should come from default
	if prof.Include720p == nil || !*prof.Include720p {
		t.Error("custom override should preserve default Include720p")
	}
}

func TestResolveMovies_OverrideBoolToFalse(t *testing.T) {
	include4K := false
	qc := &QualityConfig{
		Profile: "size-first",
		Profiles: map[string]ProfileSet{
			"size-first": {
				Movies: &MovieProfile{Include4K: &include4K},
			},
		},
	}
	prof := qc.ResolveMovies()
	if prof.Include4K == nil || *prof.Include4K {
		t.Error("custom Include4K=false should override size-first default")
	}
	// Other fields should still come from default
	if prof.Include720p == nil || !*prof.Include720p {
		t.Error("custom override should preserve default Include720p")
	}
}

func TestResolveTV_NoConfig(t *testing.T) {
	var qc *QualityConfig
	prof := qc.ResolveTV()
	if prof.MinSeeders == nil || *prof.MinSeeders != 5 {
		t.Errorf("expected MinSeeders=5, got %v", prof.MinSeeders)
	}
	if prof.Include720p != nil && *prof.Include720p {
		t.Error("quality-first should not include 720p by default")
	}
}

func TestResolveTV_SizeFirst(t *testing.T) {
	qc := &QualityConfig{Profile: "size-first"}
	prof := qc.ResolveTV()
	if prof.Include720p == nil || !*prof.Include720p {
		t.Error("size-first should include 720p")
	}
	if prof.PriorityOrder[0] != "720p" {
		t.Errorf("expected priority_order[0]=720p, got %s", prof.PriorityOrder[0])
	}
}

func TestResolveTV_CustomOverrides(t *testing.T) {
	minSeeders := 20
	fullpackBonus := 100
	qc := &QualityConfig{
		Profile: "size-first",
		Profiles: map[string]ProfileSet{
			"size-first": {
				TV: &TVProfile{MinSeeders: &minSeeders, FullpackBonus: &fullpackBonus},
			},
		},
	}
	prof := qc.ResolveTV()
	if prof.MinSeeders == nil || *prof.MinSeeders != 20 {
		t.Errorf("expected MinSeeders=20, got %v", prof.MinSeeders)
	}
	if prof.FullpackBonus == nil || *prof.FullpackBonus != 100 {
		t.Errorf("expected FullpackBonus=100, got %v", prof.FullpackBonus)
	}
	if prof.Include720p == nil || !*prof.Include720p {
		t.Error("custom override should preserve default Include720p")
	}
}

func TestResolveMovies_UnknownProfileName(t *testing.T) {
	qc := &QualityConfig{
		Profile:  "unknown-preset",
		Profiles: map[string]ProfileSet{},
	}
	prof := qc.ResolveMovies()
	// Should fall back to quality-first default
	if prof.MinSeeders == nil || *prof.MinSeeders != 15 {
		t.Errorf("unknown profile should fall back to quality-first, got MinSeeders=%v", prof.MinSeeders)
	}
}
