package quality

import "gostream/internal/config"

// MovieProfileFromConfig converts a config.MovieQualityProfile to a quality.MovieProfile.
func MovieProfileFromConfig(cp config.MovieQualityProfile) MovieProfile {
	mp := MovieProfile{
		SizeFloorGB:   cp.SizeFloorGB,
		SizeCeilingGB: cp.SizeCeilingGB,
		PriorityOrder: cp.PriorityOrder,
		ScoreWeights: MovieScoreWeights{
			Resolution4K:       cp.ScoreWeights.Resolution4K,
			Resolution1080p:    cp.ScoreWeights.Resolution1080p,
			Resolution720p:     cp.ScoreWeights.Resolution720p,
			DolbyVision:        cp.ScoreWeights.DolbyVision,
			HDR:                cp.ScoreWeights.HDR,
			HDR10Plus:          cp.ScoreWeights.HDR10Plus,
			Atmos:              cp.ScoreWeights.Atmos,
			Audio51:            cp.ScoreWeights.Audio51,
			AudioStereo:        cp.ScoreWeights.AudioStereo,
			BluRay:             cp.ScoreWeights.BluRay,
			Remux:              cp.ScoreWeights.Remux,
			ITA:                cp.ScoreWeights.ITA,
			SeederBonus:        cp.ScoreWeights.SeederBonus,
			SeederThreshold:    cp.ScoreWeights.SeederThreshold,
			UnknownSizePenalty: cp.ScoreWeights.UnknownSizePenalty,
		},
	}
	if cp.Include4K != nil {
		v := *cp.Include4K
		mp.Include4K = &v
	}
	if cp.Include1080p != nil {
		v := *cp.Include1080p
		mp.Include1080p = &v
	}
	if cp.Include720p != nil {
		v := *cp.Include720p
		mp.Include720p = &v
	}
	if cp.MinSeeders != nil {
		v := *cp.MinSeeders
		mp.MinSeeders = &v
	}
	if cp.Fallback4KMinSeeders != nil {
		mp.Fallback4KMinSeeders = cp.Fallback4KMinSeeders
	}
	return mp
}

// TVProfileFromConfig converts a config.TVQualityProfile to a quality.TVProfile.
func TVProfileFromConfig(cp config.TVQualityProfile) TVProfile {
	tp := TVProfile{
		SizeFloorGB:   cp.SizeFloorGB,
		SizeCeilingGB: cp.SizeCeilingGB,
		PriorityOrder: cp.PriorityOrder,
		ScoreWeights: TVScoreWeights{
			Resolution4K:    cp.ScoreWeights.Resolution4K,
			Resolution1080p: cp.ScoreWeights.Resolution1080p,
			Resolution720p:  cp.ScoreWeights.Resolution720p,
			HDR:             cp.ScoreWeights.HDR,
			Atmos:           cp.ScoreWeights.Atmos,
			Audio51:         cp.ScoreWeights.Audio51,
			ITA:             cp.ScoreWeights.ITA,
			Seeder100Bonus:  cp.ScoreWeights.Seeder100Bonus,
			Seeder50Bonus:   cp.ScoreWeights.Seeder50Bonus,
			Seeder20Bonus:   cp.ScoreWeights.Seeder20Bonus,
		},
	}
	if cp.Include4K != nil {
		v := *cp.Include4K
		tp.Include4K = &v
	}
	if cp.Include1080p != nil {
		v := *cp.Include1080p
		tp.Include1080p = &v
	}
	if cp.Include720p != nil {
		v := *cp.Include720p
		tp.Include720p = &v
	}
	if cp.MinSeeders4K != nil {
		v := *cp.MinSeeders4K
		tp.MinSeeders4K = &v
	}
	if cp.MinSeeders != nil {
		v := *cp.MinSeeders
		tp.MinSeeders = &v
	}
	if cp.FullpackBonus != nil {
		v := *cp.FullpackBonus
		tp.FullpackBonus = &v
	}
	return tp
}

// ResolveMovieProfile converts a config.QualityConfig to a resolved quality.MovieProfile.
// Handles the full chain: config.QualityConfig -> quality.QualityConfig -> ResolveMovies().
func ResolveMovieProfile(cfg config.QualityConfig) MovieProfile {
	qc := toQualityConfig(cfg)
	return qc.ResolveMovies()
}

// ResolveTVProfile converts a config.QualityConfig to a resolved quality.TVProfile.
func ResolveTVProfile(cfg config.QualityConfig) TVProfile {
	qc := toQualityConfig(cfg)
	return qc.ResolveTV()
}

// toQualityConfig converts config.QualityConfig to quality.QualityConfig.
// Returns nil when no profile is configured, which triggers defaults in ResolveMovies/ResolveTV.
func toQualityConfig(cfg config.QualityConfig) *QualityConfig {
	if cfg.Profile == "" && len(cfg.Profiles) == 0 {
		return nil // will use defaults
	}
	qc := &QualityConfig{
		Profile:  cfg.Profile,
		Profiles: make(map[string]ProfileSet),
	}
	for name, ps := range cfg.Profiles {
		entry := ProfileSet{}
		if ps.Movies != nil {
			entry.Movies = &MovieProfile{}
			mp := ps.Movies
			if mp.Include4K != nil { entry.Movies.Include4K = mp.Include4K }
			if mp.Include1080p != nil { entry.Movies.Include1080p = mp.Include1080p }
			if mp.Include720p != nil { entry.Movies.Include720p = mp.Include720p }
			entry.Movies.SizeFloorGB = mp.SizeFloorGB
			entry.Movies.SizeCeilingGB = mp.SizeCeilingGB
			if mp.MinSeeders != nil { entry.Movies.MinSeeders = mp.MinSeeders }
			if mp.Fallback4KMinSeeders != nil { entry.Movies.Fallback4KMinSeeders = mp.Fallback4KMinSeeders }
			entry.Movies.PriorityOrder = mp.PriorityOrder
			sw := mp.ScoreWeights
			entry.Movies.ScoreWeights = MovieScoreWeights{
				Resolution4K: sw.Resolution4K, Resolution1080p: sw.Resolution1080p,
				Resolution720p: sw.Resolution720p, DolbyVision: sw.DolbyVision,
				HDR: sw.HDR, HDR10Plus: sw.HDR10Plus, Atmos: sw.Atmos,
				Audio51: sw.Audio51, AudioStereo: sw.AudioStereo,
				BluRay: sw.BluRay, Remux: sw.Remux, ITA: sw.ITA,
				SeederBonus: sw.SeederBonus, SeederThreshold: sw.SeederThreshold,
				UnknownSizePenalty: sw.UnknownSizePenalty,
			}
		}
		if ps.TV != nil {
			entry.TV = &TVProfile{}
			tp := ps.TV
			if tp.Include4K != nil { entry.TV.Include4K = tp.Include4K }
			if tp.Include1080p != nil { entry.TV.Include1080p = tp.Include1080p }
			if tp.Include720p != nil { entry.TV.Include720p = tp.Include720p }
			entry.TV.SizeFloorGB = tp.SizeFloorGB
			entry.TV.SizeCeilingGB = tp.SizeCeilingGB
			if tp.MinSeeders4K != nil { entry.TV.MinSeeders4K = tp.MinSeeders4K }
			if tp.MinSeeders != nil { entry.TV.MinSeeders = tp.MinSeeders }
			if tp.FullpackBonus != nil { entry.TV.FullpackBonus = tp.FullpackBonus }
			entry.TV.PriorityOrder = tp.PriorityOrder
			tw := tp.ScoreWeights
			entry.TV.ScoreWeights = TVScoreWeights{
				Resolution4K: tw.Resolution4K, Resolution1080p: tw.Resolution1080p,
				Resolution720p: tw.Resolution720p, HDR: tw.HDR,
				Atmos: tw.Atmos, Audio51: tw.Audio51, ITA: tw.ITA,
				Seeder100Bonus: tw.Seeder100Bonus, Seeder50Bonus: tw.Seeder50Bonus, Seeder20Bonus: tw.Seeder20Bonus,
			}
		}
		qc.Profiles[name] = entry
	}
	return qc
}
