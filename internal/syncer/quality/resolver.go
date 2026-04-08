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

// mergeMovieProfile fills nil pointer fields in custom with values from defaults.
func mergeMovieProfile(custom, def MovieProfile) MovieProfile {
	result := def
	if custom.Include4K != nil { result.Include4K = custom.Include4K }
	if custom.Include1080p != nil { result.Include1080p = custom.Include1080p }
	if custom.Include720p != nil { result.Include720p = custom.Include720p }
	if len(custom.SizeFloorGB) > 0 { result.SizeFloorGB = custom.SizeFloorGB }
	if len(custom.SizeCeilingGB) > 0 { result.SizeCeilingGB = custom.SizeCeilingGB }
	if custom.MinSeeders != nil { result.MinSeeders = custom.MinSeeders }
	if custom.Fallback4KMinSeeders != nil { result.Fallback4KMinSeeders = custom.Fallback4KMinSeeders }
	if len(custom.PriorityOrder) > 0 { result.PriorityOrder = custom.PriorityOrder }
	mergeMovieWeights(&result.ScoreWeights, &custom.ScoreWeights)
	return result
}

func mergeMovieWeights(result, custom *MovieScoreWeights) {
	if custom.Resolution4K != nil { result.Resolution4K = custom.Resolution4K }
	if custom.Resolution1080p != nil { result.Resolution1080p = custom.Resolution1080p }
	if custom.Resolution720p != nil { result.Resolution720p = custom.Resolution720p }
	if custom.DolbyVision != nil { result.DolbyVision = custom.DolbyVision }
	if custom.HDR != nil { result.HDR = custom.HDR }
	if custom.HDR10Plus != nil { result.HDR10Plus = custom.HDR10Plus }
	if custom.Atmos != nil { result.Atmos = custom.Atmos }
	if custom.Audio51 != nil { result.Audio51 = custom.Audio51 }
	if custom.AudioStereo != nil { result.AudioStereo = custom.AudioStereo }
	if custom.BluRay != nil { result.BluRay = custom.BluRay }
	if custom.Remux != nil { result.Remux = custom.Remux }
	if custom.ITA != nil { result.ITA = custom.ITA }
	if custom.SeederBonus != nil { result.SeederBonus = custom.SeederBonus }
	if custom.SeederThreshold != nil { result.SeederThreshold = custom.SeederThreshold }
	if custom.UnknownSizePenalty != nil { result.UnknownSizePenalty = custom.UnknownSizePenalty }
}

func mergeTVProfile(custom, def TVProfile) TVProfile {
	result := def
	if custom.Include4K != nil { result.Include4K = custom.Include4K }
	if custom.Include1080p != nil { result.Include1080p = custom.Include1080p }
	if custom.Include720p != nil { result.Include720p = custom.Include720p }
	if len(custom.SizeFloorGB) > 0 { result.SizeFloorGB = custom.SizeFloorGB }
	if len(custom.SizeCeilingGB) > 0 { result.SizeCeilingGB = custom.SizeCeilingGB }
	if custom.MinSeeders4K != nil { result.MinSeeders4K = custom.MinSeeders4K }
	if custom.MinSeeders != nil { result.MinSeeders = custom.MinSeeders }
	if custom.FullpackBonus != nil { result.FullpackBonus = custom.FullpackBonus }
	if len(custom.PriorityOrder) > 0 { result.PriorityOrder = custom.PriorityOrder }
	mergeTVWeights(&result.ScoreWeights, &custom.ScoreWeights)
	return result
}

func mergeTVWeights(result, custom *TVScoreWeights) {
	if custom.Resolution4K != nil { result.Resolution4K = custom.Resolution4K }
	if custom.Resolution1080p != nil { result.Resolution1080p = custom.Resolution1080p }
	if custom.Resolution720p != nil { result.Resolution720p = custom.Resolution720p }
	if custom.HDR != nil { result.HDR = custom.HDR }
	if custom.Atmos != nil { result.Atmos = custom.Atmos }
	if custom.Audio51 != nil { result.Audio51 = custom.Audio51 }
	if custom.ITA != nil { result.ITA = custom.ITA }
	if custom.Seeder100Bonus != nil { result.Seeder100Bonus = custom.Seeder100Bonus }
	if custom.Seeder50Bonus != nil { result.Seeder50Bonus = custom.Seeder50Bonus }
	if custom.Seeder20Bonus != nil { result.Seeder20Bonus = custom.Seeder20Bonus }
}
