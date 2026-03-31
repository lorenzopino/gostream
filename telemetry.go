package main

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"runtime"
	"time"
)

// SendHeartbeat sends an anonymous heartbeat to the telemetry server.
// Runs in a goroutine to avoid blocking startup.
func SendHeartbeat(cfg Config) {
	if !cfg.EnableTelemetry || cfg.TelemetryURL == "" {
		return
	}

	go func() {
		// Wait a bit after startup to ensure network is ready and not spike CPU
		time.Sleep(10 * time.Second)

		payload := map[string]string{
			"id":      cfg.TelemetryID,
			"version": "V1.4.7", // Update this as needed
			"arch":    runtime.GOARCH,
			"os":      runtime.GOOS,
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			return
		}

		client := &http.Client{
			Timeout: 5 * time.Second,
		}

		resp, err := client.Post(cfg.TelemetryURL, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			// Silent fail: don't disturb the user if telemetry server is down
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			log.Printf("[Telemetry] Anonymous heartbeat sent successfully")
		}
	}()
}
