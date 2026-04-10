package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"gostream/internal/syncer/engines"
)

func TestGoStormTorrentTester_CurrentTorrentStatus_NoServer(t *testing.T) {
	// With no server, should return error/false
	tester := &goStormTorrentTester{
		client: engines.NewGoStormClient("http://127.0.0.1:1"),
	}

	speed, seeders, active := tester.CurrentTorrentStatus(context.Background(), "abc123")
	if active {
		t.Error("expected active=false when server is unreachable")
	}
	if speed != 0 || seeders != 0 {
		t.Errorf("expected 0 speed/seeders, got %d/%d", speed, seeders)
	}
}

func TestGoStormTorrentTester_CurrentTorrentStatus_MockServer(t *testing.T) {
	// Mock GoStorm API response for GetTorrent (single torrent)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"hash":"abc123","title":"Test","length":1000000,"active_peers":5,"download_speed":512000,"file_stats":[{"id":1,"path":"test.mkv","length":1000000}]}`))
	}))
	defer srv.Close()

	tester := &goStormTorrentTester{
		client: engines.NewGoStormClient(srv.URL),
	}

	speed, seeders, active := tester.CurrentTorrentStatus(context.Background(), "abc123")
	if !active {
		t.Error("expected active=true")
	}
	if seeders != 5 {
		t.Errorf("expected 5 seeders, got %d", seeders)
	}
	if speed != 500 { // 512000 bytes/sec / 1024 = 500 KBps
		t.Errorf("expected 500 KBps, got %d", speed)
	}
}

func TestGoStormTorrentReplacer_AddFails(t *testing.T) {
	replacer := &goStormTorrentReplacer{
		client: engines.NewGoStormClient("http://127.0.0.1:1"),
	}

	ok, err := replacer.ReplaceTorrent(context.Background(), "test", "old", "new", "magnet:?xt=urn:btih:abcdef1234567890abcdef1234567890abcdef12", "Test Title")
	if ok {
		t.Error("expected replace to fail when server is unreachable")
	}
	if err == nil {
		t.Error("expected error when server is unreachable")
	}
}

func TestGoStormTorrentReplacer_MockServer(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if callCount == 0 {
			// Add torrent response
			w.Write([]byte(`{"hash":"newhash123"}`))
		} else {
			// Get torrent info response — has file_stats
			w.Write([]byte(`{"hash":"newhash123","title":"New Torrent","length":1000000,"active_peers":10,"download_speed":800000,"file_stats":[{"id":1,"path":"new.mkv","length":1000000}]}`))
		}
		callCount++
	}))
	defer srv.Close()

	replacer := &goStormTorrentReplacer{
		client: engines.NewGoStormClient(srv.URL),
	}

	ok, err := replacer.ReplaceTorrent(context.Background(), "test", "old", "new", "magnet:?xt=urn:btih:abcdef1234567890abcdef1234567890abcdef12", "Test Title")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected replace to succeed")
	}
}
