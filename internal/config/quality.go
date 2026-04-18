package config

// QualityConfig holds the active quality profile and custom overrides.
// Mirrors the "quality" key in config.json.
type QualityConfig struct {
	Profile  string                     `json:"profile"`      // "quality-first" | "size-first"
	Profiles map[string]QualityProfileSet `json:"profiles,omitempty"` // custom overrides per profile name
}

// QualityProfileSet holds movie + TV profiles for a named preset.
type QualityProfileSet struct {
	Movies *MovieQualityProfile `json:"movies,omitempty"`
	TV     *TVQualityProfile    `json:"tv,omitempty"`
}

// MovieQualityProfile holds configurable quality parameters for movie torrent selection.
type MovieQualityProfile struct {
	Include4K            *bool              `json:"include_4k,omitempty"`
	Include1080p         *bool              `json:"include_1080p,omitempty"`
	Include720p          *bool              `json:"include_720p,omitempty"`
	SizeFloorGB          map[string]float64 `json:"size_floor_gb,omitempty"`
	SizeCeilingGB        map[string]float64 `json:"size_ceiling_gb,omitempty"`
	MinSeeders           *int               `json:"min_seeders,omitempty"`
	Fallback4KMinSeeders *int               `json:"fallback_4k_min_seeders,omitempty"`
	PriorityOrder        []string           `json:"priority_order,omitempty"`
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
	Remux              *int `json:"remux,omitempty"`
	ITA                *int `json:"ita,omitempty"`
	SeederBonus        *int `json:"seeder_bonus,omitempty"`
	SeederThreshold    *int `json:"seeder_threshold,omitempty"`
	UnknownSizePenalty *int `json:"unknown_size_penalty,omitempty"`
}

// TVQualityProfile holds configurable quality parameters for TV torrent selection.
type TVQualityProfile struct {
	Include4K       *bool              `json:"include_4k,omitempty"`
	Include1080p    *bool              `json:"include_1080p,omitempty"`
	Include720p     *bool              `json:"include_720p,omitempty"`
	SizeFloorGB     map[string]float64 `json:"size_floor_gb,omitempty"`
	SizeCeilingGB   map[string]float64 `json:"size_ceiling_gb,omitempty"`
	MinSeeders4K    *int               `json:"min_seeders_4k,omitempty"`
	MinSeeders      *int               `json:"min_seeders,omitempty"`
	FullpackBonus   *int               `json:"fullpack_bonus,omitempty"`
	PriorityOrder   []string           `json:"priority_order,omitempty"`
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
}

// TMDBDiscoveryConfig holds configurable TMDB discovery endpoints.
type TMDBDiscoveryConfig struct {
	Movies *TMDBEndpointGroup `json:"movies,omitempty"`
	TV     *TVDiscoveryConfig `json:"tv,omitempty"`
}

// TMDBEndpointGroup holds a list of discovery endpoints.
type TMDBEndpointGroup struct {
	Endpoints []TMDBEndpoint `json:"endpoints"`
}

// TMDBEndpoint defines a single TMDB discovery query.
type TMDBEndpoint struct {
	Name         string  `json:"name"`
	Enabled      bool    `json:"enabled"`
	EndpointType string  `json:"type"` // "discover" | "trending" | "list"

	// Common
	Language  *string `json:"language,omitempty"`
	SortBy    *string `json:"sort_by,omitempty"`
	Pages     *int    `json:"pages,omitempty"`

	// Discover shared
	VoteAverageGte       *float64 `json:"vote_average_gte,omitempty"`
	VoteCountGte         *int     `json:"vote_count_gte,omitempty"`
	WithGenres           *string  `json:"with_genres,omitempty"`
	WithoutGenres        *string  `json:"without_genres,omitempty"`
	WithKeywords         *string  `json:"with_keywords,omitempty"`
	WithoutKeywords      *string  `json:"without_keywords,omitempty"`
	WithOriginalLanguage *string  `json:"with_original_language,omitempty"`
	WithOriginCountry    *string  `json:"with_origin_country,omitempty"`
	WithRuntimeGte       *int     `json:"with_runtime_gte,omitempty"`
	WithRuntimeLte       *int     `json:"with_runtime_lte,omitempty"`
	WatchRegion          *string  `json:"watch_region,omitempty"`
	IncludeAdult         *bool    `json:"include_adult,omitempty"`

	// Movie-specific
	PrimaryReleaseDateGte *string `json:"primary_release_date_gte,omitempty"`
	PrimaryReleaseDateLte *string `json:"primary_release_date_lte,omitempty"`
	PrimaryReleaseYear    *int    `json:"primary_release_year,omitempty"`
	WithReleaseType       *string `json:"with_release_type,omitempty"`
	Region                *string `json:"region,omitempty"`
	IncludeVideo          *bool   `json:"include_video,omitempty"`

	// TV-specific
	FirstAirDateGte          *string `json:"first_air_date_gte,omitempty"`
	FirstAirDateLte          *string `json:"first_air_date_lte,omitempty"`
	FirstAirDateYear         *int    `json:"first_air_date_year,omitempty"`
	WithStatus               *string `json:"with_status,omitempty"`
	WithType                 *string `json:"with_type,omitempty"`
	WithNetworks             *string `json:"with_networks,omitempty"`
	IncludeNullFirstAirDates *bool   `json:"include_null_first_air_dates,omitempty"`

	// List endpoint
	EndpointURL *string `json:"endpoint,omitempty"`

	// Trending endpoint
	TimeWindow *string `json:"time_window,omitempty"`
}

// TVChannelConfig represents a single TV sync channel.
// A channel can operate in "discovery" mode (dynamic TMDB queries)
// or "manual" mode (explicit list of TMDB IDs) or "demand" mode (on-demand single series).
type TVChannelConfig struct {
	Enabled             bool            `json:"enabled"`
	Name                string          `json:"name"`
	Mode                string          `json:"mode"` // "discovery" | "manual" | "demand"
	Schedule            ChannelSchedule `json:"schedule"`
	Endpoints           []TMDBEndpoint  `json:"endpoints,omitempty"`           // only for mode=discovery
	TMDBIDs             []int           `json:"tmdb_ids,omitempty"`            // only for mode=manual or mode=demand
	SkipCompleteSeasons bool            `json:"skip_complete_seasons"`
	JellyfinItemID      string          `json:"jellyfin_item_id,omitempty"` // for demand mode refresh
}

// ChannelSchedule defines when a TV channel sync runs.
type ChannelSchedule struct {
	Hour       int   `json:"hour"`
	Minute     int   `json:"minute"`
	DaysOfWeek []int `json:"days_of_week"`
}

// TVDiscoveryConfig wraps the channels array under tmdb_discovery.tv.
// It supports both legacy endpoints (for backward compat) and new channels.
type TVDiscoveryConfig struct {
	Endpoints []TMDBEndpoint    `json:"endpoints,omitempty"` // legacy, backward compat
	Channels  []TVChannelConfig `json:"channels,omitempty"`  // new multi-channel
}
