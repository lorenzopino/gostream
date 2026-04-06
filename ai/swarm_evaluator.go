package ai

import (
	"fmt"
	"log"
	"strings"

	"gostream/internal/gostorm/torr"
	"gostream/internal/gostorm/torr/state"
)

// SwarmEval holds the AI evaluation result for swarm quality.
type SwarmEval struct {
	QualityScore   int    `json:"quality_score"` // 0-100
	WillBuffer     bool   `json:"will_buffer"`
	Recommendation string `json:"recommendation"` // "keep"|"search_alternative"|"abort"
}

func (e *SwarmEval) Sanitize() {
	if e.QualityScore < 0 {
		e.QualityScore = 0
	}
	if e.QualityScore > 100 {
		e.QualityScore = 100
	}
	e.Recommendation = strings.ToLower(strings.TrimSpace(e.Recommendation))
	if e.Recommendation != "keep" && e.Recommendation != "search_alternative" && e.Recommendation != "abort" {
		e.Recommendation = "keep"
	}
}

// EvaluateSwarmQuality runs a single AI evaluation of the current swarm health.
// Called ~60s after a torrent opens to predict if buffering is imminent.
func EvaluateSwarmQuality(provider AIProvider, torrent *torr.Torrent, stats *state.TorrentStatus, swarmAgeSec int) *SwarmEval {
	if stats == nil {
		return nil
	}

	speedMBs := stats.DownloadSpeed / (1024 * 1024)
	corruptionRate := float64(0)
	if stats.PiecesDirtiedGood+stats.PiecesDirtiedBad > 0 {
		corruptionRate = float64(stats.PiecesDirtiedBad) / float64(stats.PiecesDirtiedGood+stats.PiecesDirtiedBad) * 100
	}
	progress := float64(0)
	if stats.TorrentSize > 0 {
		progress = float64(stats.LoadedSize) / float64(stats.TorrentSize) * 100
	}
	fileSizeGB := float64(stats.TorrentSize) / (1024 * 1024 * 1024)

	prompt := fmt.Sprintf(
		"Swarm Evaluator. Analyze BitTorrent swarm health and predict streaming stability.\n"+
			"Output ONLY JSON: {\"quality_score\": 0-100, \"will_buffer\": true/false, \"recommendation\": \"keep\"|\"search_alternative\"|\"abort\"}\n\n"+
			"Data: Seeders:%d, Peers:%d/%d, HalfOpen:%d, Speed:%.1fMB/s, Corruption:%d/%d (%.1f%%), Progress:%.0f%%, FileSize:%.1fGB, SwarmAge:%ds",
		stats.ConnectedSeeders,
		stats.ActivePeers, stats.TotalPeers,
		stats.HalfOpenPeers,
		speedMBs,
		stats.PiecesDirtiedBad, stats.PiecesDirtiedGood, corruptionRate,
		progress, fileSizeGB, swarmAgeSec,
	)

	eval, err := fetchAIJSON[SwarmEval](provider, prompt)
	if err != nil {
		log.Printf("[AI-Pilot] SwarmEval: AI error: %v", err)
		return nil
	}

	eval.Sanitize()

	if eval.WillBuffer {
		log.Printf("[AI-Pilot] SwarmQuality: ⚠️ BUFFERING RISK — score=%d/100 recommendation=%s",
			eval.QualityScore, eval.Recommendation)
	}

	log.Printf("[AI-Pilot] SwarmQuality: score=%d/100 will_buffer=%v recommendation=%s [Seeders:%d Peers:%d/%d Speed:%.1fMB/s Corruption:%.1f%% Progress:%.0f%%]",
		eval.QualityScore, eval.WillBuffer, eval.Recommendation,
		stats.ConnectedSeeders, stats.ActivePeers, stats.TotalPeers,
		speedMBs, corruptionRate, progress,
	)

	if eval.WillBuffer && eval.Recommendation == "search_alternative" {
		log.Printf("[AI-Pilot] SwarmQuality: ⚠️ Swarm predicted to buffer — searching alternative release recommended")
	}

	return eval
}
