package quality

// MovieProfile holds all configurable quality parameters for movie torrent selection.
type MovieProfile struct {
	Include4K            *bool              `json:"include_4k"`
	Include1080p         *bool              `json:"include_1080p"`
	Include720p          *bool              `json:"include_720p"`
	SizeFloorGB          map[string]float64 `json:"size_floor_gb,omitempty"`  // keys: "720p", "1080p", "4k"
	SizeCeilingGB        map[string]float64 `json:"size_ceiling_gb,omitempty"` // keys: "720p", "1080p", "4k"
	MinSeeders           *int               `json:"min_seeders"`
	Fallback4KMinSeeders *int               `json:"fallback_4k_min_seeders,omitempty"` // nil = 4K not fallback
	PriorityOrder        []string           `json:"priority_order"`                    // ["720p", "1080p", "4k"]
	ScoreWeights         MovieScoreWeights  `json:"score_weights"`
}

// MovieScoreWeights holds configurable scoring weights for movies.
type MovieScoreWeights struct {
	Resolution4K       *int `json:"resolution_4k,omitempty"`
	Resolution1080p    *int `json:"resolution_1080p,omitempty"`
	Resolution720p     *int `json:"resolution_720p,omitempty"`
	DolbyVision        *int `json:"dolby_vision,omitempty"`
	HDR                *int `json:"hdr,omitempty"`
	HDR10Plus          *int `json:"hdr10_plus,omitempty"`
	Atmos              *int `json:"atmos,omitempty"`
	Audio51            *int `json:"audio_5_1,omitempty"`
	AudioStereo        *int `json:"audio_stereo,omitempty"`
	BluRay             *int `json:"bluray,omitempty"`
	Remux              *int `json:"remux,omitempty"` // -500 is an intentional hard veto to exclude remux releases
	ITA                *int `json:"ita,omitempty"`
	SeederBonus        *int `json:"seeder_bonus,omitempty"`
	SeederThreshold    *int `json:"seeder_threshold,omitempty"`
	UnknownSizePenalty *int `json:"unknown_size_penalty,omitempty"`
	SizeBonusPerGBUnder *int `json:"size_bonus_per_gb_under,omitempty"` // bonus per GB under resolution ceiling
}

// TVProfile holds all configurable quality parameters for TV torrent selection.
type TVProfile struct {
	Include4K       *bool              `json:"include_4k"`
	Include1080p    *bool              `json:"include_1080p"`
	Include720p     *bool              `json:"include_720p"`
	SizeFloorGB     map[string]float64 `json:"size_floor_gb,omitempty"`
	SizeCeilingGB   map[string]float64 `json:"size_ceiling_gb,omitempty"`
	MinSeeders4K    *int               `json:"min_seeders_4k"`
	MinSeeders      *int               `json:"min_seeders"`
	FullpackBonus   *int               `json:"fullpack_bonus"`
	PriorityOrder   []string           `json:"priority_order"`
	ScoreWeights    TVScoreWeights     `json:"score_weights"`
}

// TVScoreWeights holds configurable scoring weights for TV.
type TVScoreWeights struct {
	Resolution4K    *int `json:"resolution_4k,omitempty"`
	Resolution1080p *int `json:"resolution_1080p,omitempty"`
	Resolution720p  *int `json:"resolution_720p,omitempty"`
	HDR             *int `json:"hdr,omitempty"`
	Atmos           *int `json:"atmos,omitempty"`
	Audio51         *int `json:"audio_5_1,omitempty"`
	ITA             *int `json:"ita,omitempty"`
	Seeder100Bonus  *int `json:"seeder_100_bonus,omitempty"`
	Seeder50Bonus   *int `json:"seeder_50_bonus,omitempty"`
	Seeder20Bonus   *int `json:"seeder_20_bonus,omitempty"`
	SizeBonusPerGBUnder *int `json:"size_bonus_per_gb_under,omitempty"` // bonus per GB under resolution ceiling
}

// DefaultQualityFirstMovies returns the "quality-first" movie profile (matches current hardcoded behavior).
func DefaultQualityFirstMovies() MovieProfile {
	return MovieProfile{
		Include4K: ptr(true), Include1080p: ptr(true), Include720p: ptr(false),
		SizeFloorGB:   map[string]float64{"4k": 10, "1080p": 4},
		SizeCeilingGB: map[string]float64{"4k": 60, "1080p": 20},
		MinSeeders:           ptr(15),
		Fallback4KMinSeeders: nil,
		PriorityOrder:        []string{"4k", "1080p", "720p"},
		ScoreWeights: MovieScoreWeights{
			Resolution4K:       ptr(200), Resolution1080p: ptr(50),
			DolbyVision: ptr(150), HDR: ptr(100), HDR10Plus: ptr(100),
			Atmos: ptr(50), Audio51: ptr(25), AudioStereo: ptr(-50),
			BluRay: ptr(10), Remux: ptr(-500), ITA: ptr(60),
			SeederBonus: ptr(5), SeederThreshold: ptr(50), UnknownSizePenalty: ptr(-5),
		},
	}
}

// DefaultSizeFirstMovies returns the "size-first" movie profile.
func DefaultSizeFirstMovies() MovieProfile {
	return MovieProfile{
		Include4K: ptr(true), Include1080p: ptr(true), Include720p: ptr(true),
		SizeFloorGB:   map[string]float64{"720p": 0.5, "1080p": 0.8, "4k": 1},
		SizeCeilingGB: map[string]float64{"720p": 3, "1080p": 10, "4k": 15},
		MinSeeders:           ptr(10),
		Fallback4KMinSeeders: ptr(50),
		PriorityOrder:        []string{"720p", "1080p", "4k"},
		ScoreWeights: MovieScoreWeights{
			Resolution720p: ptr(500), Resolution1080p: ptr(300), Resolution4K: ptr(100),
			DolbyVision: ptr(50), HDR: ptr(40), HDR10Plus: ptr(40),
			Atmos: ptr(25), Audio51: ptr(15), AudioStereo: ptr(10),
			BluRay: ptr(5), Remux: ptr(-500), ITA: ptr(60),
			SeederBonus: ptr(5), SeederThreshold: ptr(30), UnknownSizePenalty: ptr(-5),
			SizeBonusPerGBUnder: ptr(100), // +100 points per GB under ceiling
		},
	}
}

// DefaultQualityFirstTV returns the "quality-first" TV profile.
func DefaultQualityFirstTV() TVProfile {
	return TVProfile{
		Include4K: ptr(true), Include1080p: ptr(true), Include720p: ptr(false),
		SizeFloorGB:   map[string]float64{"4k": 10, "1080p": 1},
		SizeCeilingGB: map[string]float64{"4k": 30, "1080p": 30},
		MinSeeders4K: ptr(10), MinSeeders: ptr(5), FullpackBonus: ptr(500),
		PriorityOrder: []string{"4k", "1080p", "720p"},
		ScoreWeights: TVScoreWeights{
			Resolution4K: ptr(200), Resolution1080p: ptr(50),
			HDR: ptr(100), Atmos: ptr(50), Audio51: ptr(25),
			ITA: ptr(40), Seeder100Bonus: ptr(100), Seeder50Bonus: ptr(50), Seeder20Bonus: ptr(10),
		},
	}
}

// DefaultSizeFirstTV returns the "size-first" TV profile.
func DefaultSizeFirstTV() TVProfile {
	return TVProfile{
		Include4K: ptr(true), Include1080p: ptr(true), Include720p: ptr(true),
		SizeFloorGB:   map[string]float64{"720p": 0.3, "1080p": 0.5, "4k": 0.5},
		SizeCeilingGB: map[string]float64{"720p": 1, "1080p": 2, "4k": 3},
		MinSeeders4K: ptr(50), MinSeeders: ptr(10), FullpackBonus: ptr(300),
		PriorityOrder: []string{"720p", "1080p", "4k"},
		ScoreWeights: TVScoreWeights{
			Resolution720p: ptr(500), Resolution1080p: ptr(300), Resolution4K: ptr(100),
			HDR: ptr(40), Atmos: ptr(25), Audio51: ptr(15),
			ITA: ptr(40), Seeder100Bonus: ptr(100), Seeder50Bonus: ptr(50), Seeder20Bonus: ptr(10),
			SizeBonusPerGBUnder: ptr(100),
		},
	}
}

// ptr returns a pointer to v, allowing inline pointer literals in struct composite expressions.
func ptr[T any](v T) *T { return &v }
