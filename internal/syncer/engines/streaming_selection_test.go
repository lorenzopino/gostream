package engines

import (
	"testing"

	"gostream/internal/prowlarr"
	"gostream/internal/syncer/quality"
)

func TestMovieFilterUsesStreamingFirstPolicy(t *testing.T) {
	engine := &MovieGoEngine{
		qualityProfile: quality.DefaultSizeFirstMovies(),
		blacklist:      BlacklistData{Hashes: map[string]string{}},
	}
	streams := []prowlarr.Stream{
		{
			Title:    "Example.Movie.2025.2160p.WEB-DL.DDP5.1\n👤 150\n💾 7.80GB",
			Name:     "Example.Movie.2025.2160p.WEB-DL.DDP5.1",
			InfoHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SizeGB:   7.8,
		},
		{
			Title:    "Example.Movie.2025.480p.WEBRip.x265.YTS\n👤 24\n💾 0.62GB",
			Name:     "Example.Movie.2025.480p.WEBRip.x265.YTS",
			InfoHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			SizeGB:   0.62,
		},
	}

	candidates := engine.filterMovieStreams(streams)
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].Hash != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("expected tiny WEBRip first, got %s", candidates[0].Hash)
	}
}

func TestWatchlistUsesSameStreamingFirstPolicyAsMovies(t *testing.T) {
	engine := &WatchlistGoEngine{qualityProfile: quality.DefaultSizeFirstMovies()}
	streams := []prowlarr.Stream{
		{
			Title:    "Example.Movie.2025.1080p.WEB-DL.DDP5.1\n👤 80\n💾 3.20GB",
			Name:     "Example.Movie.2025.1080p.WEB-DL.DDP5.1",
			InfoHash: "cccccccccccccccccccccccccccccccccccccccc",
			SizeGB:   3.2,
		},
		{
			Title:    "Example.Movie.2025.480p.WEBRip.x265\n👤 25\n💾 0.55GB",
			Name:     "Example.Movie.2025.480p.WEBRip.x265",
			InfoHash: "dddddddddddddddddddddddddddddddddddddddd",
			SizeGB:   0.55,
		},
	}

	best := engine.pickBestStream(streams)
	if len(best) != 2 {
		t.Fatalf("expected 2 streams, got %d", len(best))
	}
	if best[0].InfoHash != "dddddddddddddddddddddddddddddddddddddddd" {
		t.Fatalf("expected tiny WEBRip first, got %s", best[0].InfoHash)
	}
}

func TestTVPackExactFileSizeValidationRejectsOversizedEpisode(t *testing.T) {
	engine := &TVGoEngine{qualityProfile: quality.DefaultSizeFirstTV()}
	stream := TVStream{IsFullpack: true, SizeGB: 4, Seeders: 40, QualityScore: 100}
	files := []FileStat{
		{ID: 1, Path: "Show.S01E01.720p.WEBRip.mkv", Length: int64(1400 * 1024 * 1024)},
	}

	if engine.filterPackVideoFilesByStreamingPolicy(files, stream) != nil {
		t.Fatal("expected pack with oversized exact episode file to be rejected")
	}
}
