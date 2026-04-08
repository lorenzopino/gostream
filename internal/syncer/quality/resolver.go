package quality

// QualityConfig mirrors the top-level "quality" key in config.json.
type QualityConfig struct {
	Profile  string                `json:"profile"`   // "quality-first" | "size-first"
	Profiles map[string]ProfileSet `json:"profiles,omitempty"`
}

// ProfileSet holds movie + TV profiles for a named preset.
type ProfileSet struct {
	Movies *MovieProfile `json:"movies,omitempty"`
	TV     *TVProfile    `json:"tv,omitempty"`
}

// ResolveMovies returns the active movie profile. Falls back to default if not configured.
func (qc *QualityConfig) ResolveMovies() MovieProfile {
	if qc == nil {
		return DefaultQualityFirstMovies()
	}
	if qc.Profiles == nil {
		return defaultProfileByName(qc.Profile)
	}
	if ps, ok := qc.Profiles[qc.Profile]; ok && ps.Movies != nil {
		return mergeMovieProfile(*ps.Movies, defaultProfileByName(qc.Profile))
	}
	return defaultProfileByName(qc.Profile)
}

// ResolveTV returns the active TV profile. Falls back to default if not configured.
func (qc *QualityConfig) ResolveTV() TVProfile {
	if qc == nil {
		return DefaultQualityFirstTV()
	}
	if qc.Profiles == nil {
		return defaultProfileByNameTV(qc.Profile)
	}
	if ps, ok := qc.Profiles[qc.Profile]; ok && ps.TV != nil {
		return mergeTVProfile(*ps.TV, defaultProfileByNameTV(qc.Profile))
	}
	return defaultProfileByNameTV(qc.Profile)
}

func defaultProfileByName(name string) MovieProfile {
	switch name {
	case "size-first":
		return DefaultSizeFirstMovies()
	default:
		return DefaultQualityFirstMovies()
	}
}

func defaultProfileByNameTV(name string) TVProfile {
	switch name {
	case "size-first":
		return DefaultSizeFirstTV()
	default:
		return DefaultQualityFirstTV()
	}
}

// setIfNotNil sets *dst to src if src is not nil.
// dst is a pointer to the destination pointer field (e.g., **bool),
// src is the source pointer value (e.g., *bool).
func setIfNotNil[T any](dst **T, src *T) {
	if src != nil {
		*dst = src
	}
}

// mergeMovieProfile fills nil pointer fields in custom with values from defaults.
func mergeMovieProfile(custom, def MovieProfile) MovieProfile {
	result := def
	setIfNotNil(&result.Include4K, custom.Include4K)
	setIfNotNil(&result.Include1080p, custom.Include1080p)
	setIfNotNil(&result.Include720p, custom.Include720p)
	if len(custom.SizeFloorGB) > 0 { result.SizeFloorGB = custom.SizeFloorGB }
	if len(custom.SizeCeilingGB) > 0 { result.SizeCeilingGB = custom.SizeCeilingGB }
	setIfNotNil(&result.MinSeeders, custom.MinSeeders)
	setIfNotNil(&result.Fallback4KMinSeeders, custom.Fallback4KMinSeeders)
	if len(custom.PriorityOrder) > 0 { result.PriorityOrder = custom.PriorityOrder }
	mergeMovieWeights(&result.ScoreWeights, &custom.ScoreWeights)
	return result
}

func mergeMovieWeights(result, custom *MovieScoreWeights) {
	setIfNotNil(&result.Resolution4K, custom.Resolution4K)
	setIfNotNil(&result.Resolution1080p, custom.Resolution1080p)
	setIfNotNil(&result.Resolution720p, custom.Resolution720p)
	setIfNotNil(&result.DolbyVision, custom.DolbyVision)
	setIfNotNil(&result.HDR, custom.HDR)
	setIfNotNil(&result.HDR10Plus, custom.HDR10Plus)
	setIfNotNil(&result.Atmos, custom.Atmos)
	setIfNotNil(&result.Audio51, custom.Audio51)
	setIfNotNil(&result.AudioStereo, custom.AudioStereo)
	setIfNotNil(&result.BluRay, custom.BluRay)
	setIfNotNil(&result.Remux, custom.Remux)
	setIfNotNil(&result.ITA, custom.ITA)
	setIfNotNil(&result.SeederBonus, custom.SeederBonus)
	setIfNotNil(&result.SeederThreshold, custom.SeederThreshold)
	setIfNotNil(&result.UnknownSizePenalty, custom.UnknownSizePenalty)
}

func mergeTVProfile(custom, def TVProfile) TVProfile {
	result := def
	setIfNotNil(&result.Include4K, custom.Include4K)
	setIfNotNil(&result.Include1080p, custom.Include1080p)
	setIfNotNil(&result.Include720p, custom.Include720p)
	if len(custom.SizeFloorGB) > 0 { result.SizeFloorGB = custom.SizeFloorGB }
	if len(custom.SizeCeilingGB) > 0 { result.SizeCeilingGB = custom.SizeCeilingGB }
	setIfNotNil(&result.MinSeeders4K, custom.MinSeeders4K)
	setIfNotNil(&result.MinSeeders, custom.MinSeeders)
	setIfNotNil(&result.FullpackBonus, custom.FullpackBonus)
	if len(custom.PriorityOrder) > 0 { result.PriorityOrder = custom.PriorityOrder }
	mergeTVWeights(&result.ScoreWeights, &custom.ScoreWeights)
	return result
}

func mergeTVWeights(result, custom *TVScoreWeights) {
	setIfNotNil(&result.Resolution4K, custom.Resolution4K)
	setIfNotNil(&result.Resolution1080p, custom.Resolution1080p)
	setIfNotNil(&result.Resolution720p, custom.Resolution720p)
	setIfNotNil(&result.HDR, custom.HDR)
	setIfNotNil(&result.Atmos, custom.Atmos)
	setIfNotNil(&result.Audio51, custom.Audio51)
	setIfNotNil(&result.ITA, custom.ITA)
	setIfNotNil(&result.Seeder100Bonus, custom.Seeder100Bonus)
	setIfNotNil(&result.Seeder50Bonus, custom.Seeder50Bonus)
	setIfNotNil(&result.Seeder20Bonus, custom.Seeder20Bonus)
}
