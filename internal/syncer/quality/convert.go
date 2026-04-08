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
