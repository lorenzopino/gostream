//go:build scratch

package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gostream/internal/prowlarr"
	"gostream/internal/syncer/quality"
)

var (
	reExcludedLangs = regexp.MustCompile(`🇪🇸|🇫🇷|🇩🇪|🇷🇺|🇨🇳|🇯🇵|🇰🇷|🇹🇭|🇵🇹|🇧🇷|🇺🇦|🇵🇱|🇳🇱|🇹🇷|🇸🇦|🇮🇳|🇨🇿|🇭🇺|🇷🇴`)
	reExcludedDubs  = regexp.MustCompile(`(?i)\b(Ukr|Ukrainian|Ger|German|Fra|French|Spa|Spanish|Por|Portuguese|Rus|Russian|Chi|Chinese|Pol|Polish|Tur|Turkish|Ara|Arabic|Hin|Hindi|Cze|Czech|Hun|Hungarian)\s+Dub\b`)
	reGarbage       = regexp.MustCompile(`(?i)webscreener|screener|\bscr\b|\bcam\b|camrip|hdcam|telesync|\bts\b|telecine|\btc\b`)
	reSeeders       = regexp.MustCompile(`👤\s*(\d+)`)
	reSize          = regexp.MustCompile(`(?i)💾\s*([\d.]+)\s*(GB|MB)`)
)

func extractGB(title string) float64 {
	m := reSize.FindStringSubmatch(title)
	if len(m) >= 3 {
		var f float64
		fmt.Sscanf(m[1], "%f", &f)
		if strings.EqualFold(m[2], "GB") {
			return f
		}
		return f / 1000.0
	}
	return 0.0
}

func extractSeeders(title string) int {
	m := reSeeders.FindStringSubmatch(title)
	if len(m) > 1 {
		var i int
		fmt.Sscanf(m[1], "%d", &i)
		return i
	}
	return 0
}

func main() {
	cfg := prowlarr.ConfigProwlarr{
		Enabled: true,
		APIKey:  "1536fcf962c14c06803c0addec8f6d5b",
		URL:     "http://localhost:9696",
	}
	client := prowlarr.NewClient(cfg)

	movies := []struct {
		Title  string
		IMDBID string
	}{
		{"Dune: Part Two", "tt15239678"},
		{"Interstellar", "tt0816692"},
		{"The Matrix", "tt0133093"},
		{"Furiosa: A Mad Max Saga", "tt12037194"},
		{"Oppenheimer", "tt15398776"},
	}

	shows := []struct {
		Title  string
		IMDBID string
	}{
		{"Breaking Bad", "tt0903747"},
		{"Game of Thrones", "tt0944947"},
		{"Severance", "tt11280740"},
		{"The Last of Us", "tt3581920"},
		{"Fallout", "tt12637874"},
	}

	fmt.Println("# Prowlarr Streaming Engine Evaluation")
	fmt.Println()

	fmt.Println("## Movies Selection Test")
	for _, m := range movies {
		fmt.Printf("### %s\n", m.Title)
		streams := client.FetchTorrents(m.IMDBID, "movie", m.Title, nil)
		processStreams(m.Title, streams, quality.MovieStreamingPolicy(), quality.MediaMovie)
	}

	fmt.Println("## TV Shows Selection Test")
	fmt.Println("*Note: Searching for recent/first season formats typical of general series queries.*")
	for _, s := range shows {
		fmt.Printf("### %s\n", s.Title)
		// Usually S01E01 or Season 1
		queryStr := s.Title + " S01E01"
		streams := client.FetchTorrents(s.IMDBID, "tv", queryStr, []string{"5000", "5030", "5040"})
		processStreams(s.Title, streams, quality.TVStreamingPolicy(), quality.MediaTV)
	}
}

type scored struct {
	stream prowlarr.Stream
	rank   quality.StreamingRank
	reason string
	valid  bool
}

func processStreams(mediaTitle string, streams []prowlarr.Stream, policy quality.StreamingPolicy, mediaType quality.MediaType) {
	var results []scored
	for _, s := range streams {
		titleLine := s.Title + " " + s.Name
		
		gb := extractGB(titleLine)
		if gb == 0 && s.SizeGB > 0 {
			gb = s.SizeGB
		}

		reso := quality.DetectResolution(titleLine)
		seeders := extractSeeders(titleLine)

		// Hard filters applied in typical syncer engines
		if reExcludedLangs.MatchString(titleLine) || reExcludedDubs.MatchString(titleLine) || reGarbage.MatchString(titleLine) {
			results = append(results, scored{
				stream: s,
				reason: "Filtered (Lang/Dub/Garbage)",
				valid:  false,
			})
			continue
		}

		candidate := quality.StreamingCandidate{
			Hash:       s.InfoHash,
			Title:      titleLine,
			MediaType:  mediaType,
			Resolution: reso,
			SizeGB:     gb,
			Seeders:    seeders,
		}

		rank, ok := quality.RankStreamingCandidate(candidate, policy)
		results = append(results, scored{
			stream: s,
			rank:   rank,
			valid:  ok,
			reason: fmt.Sprintf("OK (Score: %d)", rank.Score),
		})
	}

	// Dump top 50 raw (ordered by seeders primarily from prowlarr output usually, 
	// here we just take the first 50 returned by API)
	max := 50
	if len(results) < 50 {
		max = len(results)
	}

	fmt.Printf("#### Top %d Raw Results from Prowlarr\n", max)
	fmt.Println("| # | Torrent Title | Size | Seeders | Validation |")
	fmt.Println("|---|---|---|---|---|")
	for i := 0; i < max; i++ {
		r := results[i]
		seeds := extractSeeders(r.stream.Name + " " + r.stream.Title)
		fmt.Printf("| %d | `%s` | %.1f GB | %d | %s |\n", i+1, strings.ReplaceAll(r.stream.Title+" "+r.stream.Name, "|", " "), r.stream.SizeGB, seeds, r.reason)
	}

	// Sort valid by score
	var valids []scored
	for _, r := range results {
		if r.valid {
			valids = append(valids, r)
		}
	}

	sort.SliceStable(valids, func(i, j int) bool {
		return valids[i].rank.BetterThan(valids[j].rank)
	})

	fmt.Printf("#### 🏆 Algorithm Selection\n")
	if len(valids) > 0 {
		best := valids[0]
		reso := quality.DetectResolution(best.stream.Title + " " + best.stream.Name)
		seeds := extractSeeders(best.stream.Name + " " + best.stream.Title)
		
		fmt.Printf("**Winner:** `%s`\n", best.stream.Title+" "+best.stream.Name)
		fmt.Printf("- **Resolution:** %s\n", string(reso))
		fmt.Printf("- **Size:** %.2f GB\n", best.stream.SizeGB)
		fmt.Printf("- **Seeders:** %d\n", seeds)
		fmt.Printf("- **Score Details:** Total: `%d` (Size Tier: %d, Peer Tier: %d, CompactTagScore: %d, Res. Tier: %d)\n\n", 
			best.rank.Score, best.rank.SizeTier, best.rank.PeerTier, best.rank.CompactTagScore, best.rank.ResolutionTier)
	} else {
		fmt.Printf("**Result:** NO VALID CANDIDATES passed the quality policy engine.\n\n")
	}
}
