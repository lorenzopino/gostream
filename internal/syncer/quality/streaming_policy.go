package quality

import (
	"regexp"
	"sort"
	"strings"
)

type MediaType string

const (
	MediaMovie MediaType = "movie"
	MediaTV    MediaType = "tv"
)

type Resolution string

const (
	Resolution4K      Resolution = "4k"
	Resolution1080p   Resolution = "1080p"
	Resolution720p    Resolution = "720p"
	Resolution480p    Resolution = "480p"
	ResolutionSD      Resolution = "sd"
	ResolutionUnknown Resolution = "unknown"
)

type StreamingCandidate struct {
	Hash                  string
	Title                 string
	MediaType             MediaType
	Resolution            Resolution
	SizeGB                float64
	Seeders               int
	IsPack                bool
	EstimatedEpisodeCount int
}

type StreamingPolicy struct {
	MediaType       MediaType
	IdealGB         float64
	CompactGB       float64
	FallbackGB      float64
	StrictFileMaxGB float64
	MinSeeders      int
	GoodSeeders     int
	StrongSeeders   int
}

type StreamingRank struct {
	StreamingCandidate
	EffectiveSizeGB float64
	SizeTier        int
	PeerTier        int
	CompactTagScore int
	ResolutionTier  int
	Score           int
}

var (
	reStreaming4K      = regexp.MustCompile(`(?i)(2160p|4k|uhd)`)
	reStreaming1080p   = regexp.MustCompile(`(?i)(1080p|1080i|fhd)`)
	reStreaming720p    = regexp.MustCompile(`(?i)(720p|720i)`)
	reStreaming480p    = regexp.MustCompile(`(?i)\b(480p|576p|sd)\b`)
	reBadStreamingTags = regexp.MustCompile(`(?i)\b(bd[-_. ]?remux|remux|cam|camrip|hdcam|hdts|telesync|telecine|webscreener|screener|scr)\b`)
	reBadLanguageTags  = regexp.MustCompile(`🇪🇸|🇫🇷|🇩🇪|🇷🇺|🇨🇳|🇯🇵|🇰🇷|🇹🇭|🇵🇹|🇧🇷|🇺🇦|🇵🇱|🇳🇱|🇹🇷|🇸🇦|🇮🇳|🇨🇿|🇭🇺|🇷🇴`)
	reBadDubTags       = regexp.MustCompile(`(?i)\b(Ukr|Ukrainian|Ger|German|Fra|French|Spa|Spanish|Por|Portuguese|Rus|Russian|Chi|Chinese|Pol|Polish|Tur|Turkish|Ara|Arabic|Hin|Hindi|Cze|Czech|Hun|Hungarian)\s+Dub\b`)
	reCompactSource    = regexp.MustCompile(`(?i)\b(web[-_. ]?rip|web[-_. ]?dl|web)\b`)
	reCompactCodec     = regexp.MustCompile(`(?i)\b(x265|h265|hevc|av1)\b`)
	reCompactGroup     = regexp.MustCompile(`(?i)\b(yts|yify|rarbg|pahe|psa)\b`)
)

func MovieStreamingPolicy() StreamingPolicy {
	return StreamingPolicy{
		MediaType:       MediaMovie,
		IdealGB:         1.0,
		CompactGB:       2.0,
		FallbackGB:      8.0,
		StrictFileMaxGB: 2.0,
		MinSeeders:      5,
		GoodSeeders:     15,
		StrongSeeders:   50,
	}
}

func TVStreamingPolicy() StreamingPolicy {
	return StreamingPolicy{
		MediaType:       MediaTV,
		IdealGB:         0.5,
		CompactGB:       1.0,
		FallbackGB:      1.5,
		StrictFileMaxGB: 1.0,
		MinSeeders:      5,
		GoodSeeders:     15,
		StrongSeeders:   40,
	}
}

func DetectResolution(title string) Resolution {
	switch {
	case reStreaming4K.MatchString(title):
		return Resolution4K
	case reStreaming1080p.MatchString(title):
		return Resolution1080p
	case reStreaming720p.MatchString(title):
		return Resolution720p
	case reStreaming480p.MatchString(title):
		if strings.Contains(strings.ToLower(title), "480p") {
			return Resolution480p
		}
		return ResolutionSD
	default:
		return ResolutionUnknown
	}
}

func RankStreamingCandidates(candidates []StreamingCandidate, policy StreamingPolicy) []StreamingRank {
	ranked := make([]StreamingRank, 0, len(candidates))
	for _, candidate := range candidates {
		rank, ok := RankStreamingCandidate(candidate, policy)
		if ok {
			ranked = append(ranked, rank)
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].BetterThan(ranked[j])
	})
	return ranked
}

func RankStreamingCandidate(candidate StreamingCandidate, policy StreamingPolicy) (StreamingRank, bool) {
	candidate.Hash = strings.ToLower(strings.TrimSpace(candidate.Hash))
	if candidate.Hash == "" {
		return StreamingRank{}, false
	}
	if policy.MediaType != "" && candidate.MediaType != "" && candidate.MediaType != policy.MediaType {
		return StreamingRank{}, false
	}

	title := candidate.Title
	if reBadStreamingTags.MatchString(title) || reBadLanguageTags.MatchString(title) || reBadDubTags.MatchString(title) {
		return StreamingRank{}, false
	}
	if candidate.Resolution == ResolutionUnknown || candidate.Resolution == "" {
		candidate.Resolution = DetectResolution(title)
	}
	if candidate.Resolution == ResolutionUnknown {
		return StreamingRank{}, false
	}

	effectiveSizeGB := candidate.SizeGB
	if candidate.IsPack && candidate.EstimatedEpisodeCount > 0 {
		effectiveSizeGB = candidate.SizeGB / float64(candidate.EstimatedEpisodeCount)
	}
	if effectiveSizeGB <= 0 {
		return StreamingRank{}, false
	}
	if candidate.Seeders > 0 && candidate.Seeders < policy.MinSeeders {
		return StreamingRank{}, false
	}
	if effectiveSizeGB > policy.FallbackGB {
		return StreamingRank{}, false
	}
	if candidate.Resolution == Resolution4K && effectiveSizeGB > policy.CompactGB && candidate.Seeders < policy.StrongSeeders {
		return StreamingRank{}, false
	}

	rank := StreamingRank{
		StreamingCandidate: candidate,
		EffectiveSizeGB:    effectiveSizeGB,
		SizeTier:           streamingSizeTier(effectiveSizeGB, policy),
		PeerTier:           streamingPeerTier(candidate.Seeders, policy),
		CompactTagScore:    streamingCompactTagScore(title),
		ResolutionTier:     streamingResolutionTier(candidate.Resolution),
	}
	rank.Score = streamingScore(rank)
	return rank, true
}

func RankExactStreamingFile(candidate StreamingCandidate, policy StreamingPolicy) (StreamingRank, bool) {
	if candidate.SizeGB > policy.StrictFileMaxGB {
		return StreamingRank{}, false
	}
	return RankStreamingCandidate(candidate, policy)
}

func (r StreamingRank) BetterThan(other StreamingRank) bool {
	if r.SizeTier != other.SizeTier {
		return r.SizeTier < other.SizeTier
	}
	if r.PeerTier != other.PeerTier {
		return r.PeerTier > other.PeerTier
	}
	if r.EffectiveSizeGB != other.EffectiveSizeGB {
		return r.EffectiveSizeGB < other.EffectiveSizeGB
	}
	if r.CompactTagScore != other.CompactTagScore {
		return r.CompactTagScore > other.CompactTagScore
	}
	if r.ResolutionTier != other.ResolutionTier {
		return r.ResolutionTier < other.ResolutionTier
	}
	if r.Seeders != other.Seeders {
		return r.Seeders > other.Seeders
	}
	return r.Hash < other.Hash
}

func streamingSizeTier(sizeGB float64, policy StreamingPolicy) int {
	switch {
	case sizeGB <= policy.IdealGB:
		return 0
	case sizeGB <= policy.CompactGB:
		return 1
	default:
		return 2
	}
}

func streamingPeerTier(seeders int, policy StreamingPolicy) int {
	switch {
	case seeders >= policy.StrongSeeders:
		return 3
	case seeders >= policy.GoodSeeders:
		return 2
	case seeders >= policy.MinSeeders:
		return 1
	default:
		return 0
	}
}

func streamingCompactTagScore(title string) int {
	score := 0
	if reCompactSource.MatchString(title) {
		score += 30
	}
	if reCompactCodec.MatchString(title) {
		score += 20
	}
	if reCompactGroup.MatchString(title) {
		score += 10
	}
	return score
}

func streamingResolutionTier(res Resolution) int {
	switch res {
	case Resolution480p, ResolutionSD:
		return 0
	case Resolution720p:
		return 1
	case Resolution1080p:
		return 2
	case Resolution4K:
		return 3
	default:
		return 4
	}
}

func streamingScore(rank StreamingRank) int {
	sizePoints := int(rank.EffectiveSizeGB * 1000)
	return (3-rank.SizeTier)*100000 +
		rank.PeerTier*10000 +
		rank.CompactTagScore*100 -
		rank.ResolutionTier*100 -
		sizePoints +
		rank.Seeders
}
