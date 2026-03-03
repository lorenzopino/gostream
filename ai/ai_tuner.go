package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gostream/internal/gostorm/torr"
)

// Track last values to show changes in logs
var lastConns = 30
var lastTimeout = 30
var metricsHistory []string

type AITweak struct {
	ConnectionsLimit int    `json:"ConnectionsLimit"`
	PeerTimeout      int    `json:"PeerTimeout"`
}

func (t *AITweak) Sanitize() {
	if t.ConnectionsLimit < 15 { t.ConnectionsLimit = 15 }
	if t.ConnectionsLimit > 60 { t.ConnectionsLimit = 60 }
	if t.PeerTimeout < 10 { t.PeerTimeout = 10 }
	if t.PeerTimeout > 60 { t.PeerTimeout = 60 }
}

func StartAITuner(ctx context.Context, aiURL string) {
	if aiURL == "" { aiURL = "http://localhost:8085" }
	log.Printf("[AI-Tuner] Machine-Mode Controller Active")
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			runTuningCycle(aiURL)
		case <-ctx.Done():
			return
		}
	}
}

func runTuningCycle(aiURL string) {
	torrents := torr.ListTorrent()
	if len(torrents) == 0 { return }

	var activeT *torr.Torrent
	maxSpeed := float64(-1)
	for _, t := range torrents {
		st := t.StatHighFreq()
		if st.DownloadSpeed > maxSpeed {
			maxSpeed = st.DownloadSpeed
			activeT = t
		}
	}
	if activeT == nil || activeT.Torrent == nil { return }

	st := activeT.StatHighFreq()
	cpu := getCPUUsage()
	buffer := 100
	if activeT.GetCache() != nil {
		cs := activeT.GetCache().GetState()
		if cs.Capacity > 0 { buffer = int(cs.Filled * 100 / cs.Capacity) }
	}

	currentSnap := fmt.Sprintf("[CPU:%d%%, Buf:%d%%, Peers:%d, Speed:%.1fMB/s]", cpu, buffer, st.ActivePeers, st.DownloadSpeed/(1024*1024))
	metricsHistory = append(metricsHistory, currentSnap)
	if len(metricsHistory) > 3 { metricsHistory = metricsHistory[1:] }
	historyStr := strings.Join(metricsHistory, " -> ")

	// V1.4.15: Minimalist Prompt - NO PROSE, NO TEXT, ONLY JSON
	prompt := fmt.Sprintf("<|im_start|>system\nYou are a BitTorrent Engine Tuning unit for Raspberry Pi 4 (4K Fiber context).\nObjective: 100%% Buffer, Stable CPU.\nControls: ConnectionsLimit (15-60), PeerTimeout (10-60).\nTrend: %s\nRespond ONLY compact JSON: {\"ConnectionsLimit\":N, \"PeerTimeout\":N}<|im_end|>\n<|im_start|>user\nDECIDE.<|im_end|>\n<|im_start|>assistant\n{\"ConnectionsLimit\":", historyStr)

	tweak, err := callAI(aiURL, prompt)
	if err != nil {
		log.Printf("[AI-Tuner] AI unreachable or slow: %v", err)
		return
	}

	tweak.Sanitize()

	// Atomic Memory Update & Log
	oldConns := lastConns
	oldTimeout := lastTimeout
	
	if activeT.Torrent != nil {
		activeT.Torrent.SetMaxEstablishedConns(tweak.ConnectionsLimit)
		activeT.AddExpiredTime(time.Duration(tweak.PeerTimeout) * time.Second)
		
		lastConns = tweak.ConnectionsLimit
		lastTimeout = tweak.PeerTimeout

		log.Printf("[AI-Tuner] RAM_UPDATE: Conns(%d->%d) Timeout(%ds->%ds) [Metrics: %s]", 
			oldConns, lastConns, oldTimeout, lastTimeout, currentSnap)
	}
}

func callAI(url string, prompt string) (*AITweak, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"prompt": prompt, "n_predict": 32, "temperature": 0.1, // Fixed precision
		"stop": []string{"<|im_end|>", "}"},
	})
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Post(url+"/completion", "application/json", bytes.NewBuffer(reqBody))
	if err != nil { return nil, err }
	defer resp.Body.Close()
	var aiResp struct { Content string `json:"content"` }
	json.NewDecoder(resp.Body).Decode(&aiResp)
	var tweak AITweak
	content := "{\"ConnectionsLimit\":" + strings.TrimSpace(aiResp.Content)
	if !strings.HasSuffix(content, "}") { content += "}" }
	if err := json.Unmarshal([]byte(content), &tweak); err != nil { return nil, err }
	return &tweak, nil
}

func getCPUUsage() int {
	t1Total, t1Idle := readCPUSample()
	time.Sleep(300 * time.Millisecond)
	t2Total, t2Idle := readCPUSample()
	totalDiff := t2Total - t1Total
	idleDiff := t2Idle - t1Idle
	if totalDiff == 0 { return 0 }
	return int(100 * (totalDiff - idleDiff) / totalDiff)
}

func readCPUSample() (uint64, uint64) {
	data, _ := os.ReadFile("/proc/stat")
	fields := strings.Fields(strings.Split(string(data), "\n")[0])
	var total uint64
	for i := 1; i < len(fields); i++ {
		val, _ := strconv.ParseUint(fields[i], 10, 64)
		total += val
	}
	idle, _ := strconv.ParseUint(fields[4], 10, 64)
	return total, idle
}
