package main

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func resetPlaybackRegistryForTest(t *testing.T) {
	t.Helper()
	playbackRegistry = sync.Map{}
}

func postWebhookForTest(t *testing.T, payload string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	handlePlexWebhook(httptest.NewRecorder(), req)
}

func TestHandlePlexWebhook_DoesNotConfirmWrongLocalizedEpisode(t *testing.T) {
	resetPlaybackRegistryForTest(t)

	wrongPath := "/mount/tv/The_Miniature_Wife (2026)/Season.01/The_Miniature_Wife_S01E08_f52f803f.mkv"
	wrongState := &PlaybackState{Path: wrongPath}
	playbackRegistry.Store(wrongPath, wrongState)

	postWebhookForTest(t, `{
		"event":"PlaybackProgress",
		"Metadata":{
			"title":"Episodio 3",
			"grandparentTitle":"The Miniature Wife",
			"librarySectionType":"Episode"
		}
	}`)

	if wrongState.GetStatus() {
		t.Fatalf("localized episode webhook confirmed wrong file: %s", wrongPath)
	}
}

func TestHandlePlexWebhook_ConfirmsLocalizedEpisodeBySeasonEpisodeFilename(t *testing.T) {
	resetPlaybackRegistryForTest(t)

	wrongPath := "/mount/tv/The_Miniature_Wife (2026)/Season.01/The_Miniature_Wife_S01E08_f52f803f.mkv"
	rightPath := "/mount/tv/The_Miniature_Wife (2026)/Season.01/The_Miniature_Wife_S01E03_b64d753a.mkv"
	wrongState := &PlaybackState{Path: wrongPath}
	rightState := &PlaybackState{Path: rightPath}
	playbackRegistry.Store(wrongPath, wrongState)
	playbackRegistry.Store(rightPath, rightState)

	postWebhookForTest(t, `{
		"event":"PlaybackProgress",
		"Metadata":{
			"title":"Episodio 3",
			"grandparentTitle":"The Miniature Wife",
			"librarySectionType":"Episode"
		}
	}`)

	if !rightState.GetStatus() {
		t.Fatalf("localized episode webhook did not confirm expected file: %s", rightPath)
	}
	if wrongState.GetStatus() {
		t.Fatalf("localized episode webhook also confirmed wrong file: %s", wrongPath)
	}
}
