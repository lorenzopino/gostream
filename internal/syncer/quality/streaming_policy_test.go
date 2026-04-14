package quality

import "testing"

func TestStreamingMoviePrefersSub1GBWebRipOverLarge4K(t *testing.T) {
	candidates := []StreamingCandidate{
		{
			Hash:       "large4k",
			Title:      "Example.Movie.2025.2160p.WEB-DL.DDP5.1.HDR",
			MediaType:  MediaMovie,
			Resolution: Resolution4K,
			SizeGB:     7.8,
			Seeders:    120,
		},
		{
			Hash:       "small720",
			Title:      "Example.Movie.2025.720p.WEBRip.x265.YTS",
			MediaType:  MediaMovie,
			Resolution: Resolution720p,
			SizeGB:     0.82,
			Seeders:    28,
		},
	}

	ranked := RankStreamingCandidates(candidates, MovieStreamingPolicy())
	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked candidates, got %d", len(ranked))
	}
	if ranked[0].Hash != "small720" {
		t.Fatalf("expected sub-1GB WEBRip first, got %s", ranked[0].Hash)
	}
}

func TestStreamingTVPrefersSubHalfGBEpisode(t *testing.T) {
	candidates := []StreamingCandidate{
		{
			Hash:       "large1080",
			Title:      "Show.S01E01.1080p.WEB-DL.DDP5.1",
			MediaType:  MediaTV,
			Resolution: Resolution1080p,
			SizeGB:     1.4,
			Seeders:    80,
		},
		{
			Hash:       "tiny480",
			Title:      "Show.S01E01.480p.WEBRip.x265",
			MediaType:  MediaTV,
			Resolution: Resolution480p,
			SizeGB:     0.28,
			Seeders:    22,
		},
	}

	ranked := RankStreamingCandidates(candidates, TVStreamingPolicy())
	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked candidates, got %d", len(ranked))
	}
	if ranked[0].Hash != "tiny480" {
		t.Fatalf("expected sub-0.5GB episode first, got %s", ranked[0].Hash)
	}
}

func TestStreamingPolicyRejectsRemuxCamAndOversized(t *testing.T) {
	candidates := []StreamingCandidate{
		{Hash: "remux", Title: "Movie.2025.1080p.BDRemux", MediaType: MediaMovie, Resolution: Resolution1080p, SizeGB: 18, Seeders: 200},
		{Hash: "cam", Title: "Movie.2025.CAM.720p", MediaType: MediaMovie, Resolution: Resolution720p, SizeGB: 0.7, Seeders: 200},
		{Hash: "oversized", Title: "Movie.2025.1080p.WEB-DL", MediaType: MediaMovie, Resolution: Resolution1080p, SizeGB: 12, Seeders: 200},
	}

	ranked := RankStreamingCandidates(candidates, MovieStreamingPolicy())
	if len(ranked) != 0 {
		t.Fatalf("expected all bad candidates rejected, got %d", len(ranked))
	}
}

func TestStreamingPackUsesEstimatedEpisodeSizeBeforeMetadata(t *testing.T) {
	candidate := StreamingCandidate{
		Hash:                  "pack",
		Title:                 "Show.S01.720p.WEBRip.x265.COMPLETE",
		MediaType:             MediaTV,
		Resolution:            Resolution720p,
		SizeGB:                4.0,
		Seeders:               45,
		IsPack:                true,
		EstimatedEpisodeCount: 10,
	}

	rank, ok := RankStreamingCandidate(candidate, TVStreamingPolicy())
	if !ok {
		t.Fatal("expected 4GB/10 episode pack to be accepted before metadata")
	}
	if rank.EffectiveSizeGB >= 0.5 {
		t.Fatalf("expected estimated per-episode size below 0.5GB, got %.2f", rank.EffectiveSizeGB)
	}
}

func TestStreamingPolicyAllowsTinySDWhenSeeded(t *testing.T) {
	candidates := []StreamingCandidate{
		{
			Hash:       "sd",
			Title:      "Rare.Movie.1972.SD.WEB.x265",
			MediaType:  MediaMovie,
			Resolution: ResolutionSD,
			SizeGB:     0.42,
			Seeders:    18,
		},
	}

	ranked := RankStreamingCandidates(candidates, MovieStreamingPolicy())
	if len(ranked) != 1 || ranked[0].Hash != "sd" {
		t.Fatalf("expected tiny SD candidate to be accepted, got %#v", ranked)
	}
}
