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
	log.Printf("[AI-Tuner] Machine-Mode V1.4.19 (Active-Native)")
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
	// NEW: Use the native ListActiveTorrent() which only returns RAM torrents
	activeTorrents := torr.ListActiveTorrent()
	if len(activeTorrents) == 0 { return }

	var activeT *torr.Torrent
	var totalSpeed float64
	realActiveCount := 0
	maxSpeed := float64(-1)
	
	// Double check for engine-level activity (t.Torrent != nil)
	for _, t := range activeTorrents {
		if t.Torrent == nil { continue }
		
		st := t.StatHighFreq()
		realActiveCount++
		totalSpeed += st.DownloadSpeed
		
		if st.DownloadSpeed > maxSpeed {
			maxSpeed = st.DownloadSpeed
			activeT = t
		}
	}
	
	if activeT == nil { return }

	st := activeT.StatHighFreq()
	cpu := getCPUUsage()
	buffer := 100
	if activeT.GetCache() != nil {
		cs := activeT.GetCache().GetState()
		if cs.Capacity > 0 { buffer = int(cs.Filled * 100 / cs.Capacity) }
	}

	// Precise file size from active engine
	fSize := activeT.Size
	if fSize == 0 { fSize = activeT.Torrent.Length() }
	fileSizeGB := float64(fSize) / (1024 * 1024 * 1024)
	
	totalDL := totalSpeed / (1024 * 1024)
	currDL := st.DownloadSpeed / (1024 * 1024)

	currentSnap := fmt.Sprintf("[CPU:%d%%, Buf:%d%%, Peers:%d, Speed:%.1fMB/s]", 
		cpu, buffer, st.ActivePeers, currDL)
	
	metricsHistory = append(metricsHistory, currentSnap)
	if len(metricsHistory) > 3 { metricsHistory = metricsHistory[1:] }
	historyStr := strings.Join(metricsHistory, " -> ")

	contextStr := fmt.Sprintf("Fiber, 4K, File:%.1fGB, ActiveTorr:%d, TotalDL:%.1fMB/s", 
		fileSizeGB, realActiveCount, totalDL)

	prompt := fmt.Sprintf("<|im_start|>system\nYou are a BitTorrent Tuning unit for Raspberry Pi 4.\nContext: %s\nObjective: 100%% Buffer, Fast Performance, Stable CPU.\nControls: ConnectionsLimit (15-60), PeerTimeout (10-60).\nTrend: %s\nRespond ONLY compact JSON: {\"ConnectionsLimit\":N, \"PeerTimeout\":N}<|im_end|>\n<|im_start|>user\nDECIDE.<|im_end|>\n<|im_start|>assistant\n{\"ConnectionsLimit\":", 
		contextStr, historyStr)

	tweak, err := callAI(aiURL, prompt)
	if err != nil {
		log.Printf("[AI-Tuner] AI Error: %v", err)
		return
	}

	tweak.Sanitize()

	oldConns := lastConns
	oldTimeout := lastTimeout
	
	activeT.Torrent.SetMaxEstablishedConns(tweak.ConnectionsLimit)
	activeT.AddExpiredTime(time.Duration(tweak.PeerTimeout) * time.Second)
	
	lastConns = tweak.ConnectionsLimit
	lastTimeout = tweak.PeerTimeout

	log.Printf("[AI-Tuner] RAM_UPDATE: Conns(%d->%d) Timeout(%ds->%ds) [Metrics: %s] [Ctx: %s]", 
		oldConns, lastConns, oldTimeout, lastTimeout, currentSnap, contextStr)
}

func callAI(url string, prompt string) (*AITweak, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"prompt": prompt, "n_predict": 32, "temperature": 0.1,
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
