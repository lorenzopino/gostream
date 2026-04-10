package metadb

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := New(path, &testLogger{t})
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		os.Remove(path)
	})
	return db
}

type testLogger struct{ t *testing.T }

func (l *testLogger) Printf(format string, v ...interface{}) { l.t.Logf(format, v...) }

func TestAlternatives_UpsertAndGet(t *testing.T) {
	db := setupTestDB(t)

	alt := TorrentAlternative{
		ContentID:        "tt0903747_s01e01",
		ContentType:      "tv",
		Rank:             1,
		Hash:             "abcdef1234567890abcdef1234567890abcdef12",
		Title:            "Breaking.Bad.S01E01.1080p.BluRay.x264",
		Size:             2500000000,
		Seeders:          150,
		QualityScore:     850,
		Status:           "active",
		LastHealthCheck:  0,
		AvgSpeedKBps:     5000,
		ReplacementCount: 0,
	}

	if err := db.UpsertAlternative(alt); err != nil {
		t.Fatalf("UpsertAlternative failed: %v", err)
	}

	got, found, err := db.GetAlternative(alt.ContentID, alt.Hash)
	if err != nil {
		t.Fatalf("GetAlternative failed: %v", err)
	}
	if !found {
		t.Fatal("expected to find alternative")
	}
	if got.Rank != 1 || got.Seeders != 150 || got.QualityScore != 850 {
		t.Errorf("got rank=%d seeders=%d score=%d, want 1/150/850", got.Rank, got.Seeders, got.QualityScore)
	}
}

func TestAlternatives_UpdateStatus(t *testing.T) {
	db := setupTestDB(t)

	alt := TorrentAlternative{
		ContentID:   "tt0903747_s01e01",
		ContentType: "tv",
		Rank:        1,
		Hash:        "abcdef1234567890abcdef1234567890abcdef12",
		Title:       "Test.Title.S01E01.1080p",
		Size:        2000000000,
		Seeders:     50,
		Status:      "active",
	}
	if err := db.UpsertAlternative(alt); err != nil {
		t.Fatal(err)
	}

	if err := db.UpdateAlternativeStatus(alt.ContentID, alt.Hash, "verified_healthy"); err != nil {
		t.Fatalf("UpdateAlternativeStatus failed: %v", err)
	}

	got, found, _ := db.GetAlternative(alt.ContentID, alt.Hash)
	if !found || got.Status != "verified_healthy" {
		t.Errorf("status not updated: got %q, want verified_healthy", got.Status)
	}
}

func TestAlternatives_GetByContentID(t *testing.T) {
	db := setupTestDB(t)

	// Insert 3 alternatives for same content
	for i := 0; i < 3; i++ {
		alt := TorrentAlternative{
			ContentID:    "tt0903747_movie",
			ContentType:  "movie",
			Rank:         i + 1,
			Hash:         "abcdef1234567890abcdef1234567890abcdef1" + string(rune('0'+i)),
			Title:        "Test Movie " + string(rune('0'+i)),
			Size:         2000000000,
			Seeders:      100 - i*30,
			Status:       "active",
		}
		if err := db.UpsertAlternative(alt); err != nil {
			t.Fatal(err)
		}
	}

	alts, err := db.GetAlternativesByContent("tt0903747_movie")
	if err != nil {
		t.Fatalf("GetAlternativesByContent failed: %v", err)
	}
	if len(alts) != 3 {
		t.Fatalf("expected 3 alternatives, got %d", len(alts))
	}
	// Verify ordered by rank
	if alts[0].Rank != 1 || alts[1].Rank != 2 || alts[2].Rank != 3 {
		t.Errorf("expected ranks 1,2,3, got %d,%d,%d", alts[0].Rank, alts[1].Rank, alts[2].Rank)
	}
}

func TestAlternatives_GetNextBest(t *testing.T) {
	db := setupTestDB(t)

	// Insert alternatives with different test statuses
	entries := []TorrentAlternative{
		{ContentID: "test", ContentType: "movie", Rank: 1, Hash: "aaa", Title: "A", Size: 1e9, Seeders: 100, Status: "active"},
		{ContentID: "test", ContentType: "movie", Rank: 2, Hash: "bbb", Title: "B", Size: 1e9, Seeders: 80, Status: "tested_no_better"},
		{ContentID: "test", ContentType: "movie", Rank: 3, Hash: "ccc", Title: "C", Size: 1e9, Seeders: 60, Status: "active"},
	}
	for _, e := range entries {
		if err := db.UpsertAlternative(e); err != nil {
			t.Fatal(err)
		}
	}

	// GetNextBest should return rank 3 (first untested after rank 1)
	next, found, err := db.GetNextBestAlternative("test", "aaa")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected to find next best alternative")
	}
	if next.Hash != "ccc" {
		t.Errorf("expected hash ccc (rank 3, untested), got %s", next.Hash)
	}
}

func TestAlternatives_SaveAllForContent(t *testing.T) {
	db := setupTestDB(t)

	alts := []TorrentAlternative{
		{ContentID: "bulk_test", ContentType: "tv", Rank: 1, Hash: "hash1", Title: "T1", Size: 2e9, Seeders: 100, Status: "active"},
		{ContentID: "bulk_test", ContentType: "tv", Rank: 2, Hash: "hash2", Title: "T2", Size: 1.8e9, Seeders: 80, Status: "active"},
		{ContentID: "bulk_test", ContentType: "tv", Rank: 3, Hash: "hash3", Title: "T3", Size: 1.5e9, Seeders: 50, Status: "active"},
	}

	if err := db.SaveAlternativesForContent("bulk_test", alts); err != nil {
		t.Fatalf("SaveAlternativesForContent failed: %v", err)
	}

	got, _ := db.GetAlternativesByContent("bulk_test")
	if len(got) != 3 {
		t.Fatalf("expected 3 alternatives, got %d", len(got))
	}
}

func TestAlternatives_DeleteByContentID(t *testing.T) {
	db := setupTestDB(t)

	alts := []TorrentAlternative{
		{ContentID: "del_test", ContentType: "tv", Rank: 1, Hash: "h1", Title: "T", Size: 1e9, Seeders: 10, Status: "active"},
		{ContentID: "del_test", ContentType: "tv", Rank: 2, Hash: "h2", Title: "T", Size: 1e9, Seeders: 10, Status: "active"},
	}
	for _, a := range alts {
		db.UpsertAlternative(a)
	}

	if err := db.DeleteAlternativesByContent("del_test"); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetAlternativesByContent("del_test")
	if len(got) != 0 {
		t.Errorf("expected 0 alternatives after delete, got %d", len(got))
	}
}

func TestAlternatives_GetDeadOrSlow(t *testing.T) {
	db := setupTestDB(t)

	entries := []TorrentAlternative{
		{ContentID: "c1", ContentType: "tv", Rank: 1, Hash: "dead1", Title: "Dead", Size: 1e9, Seeders: 0, Status: "dead"},
		{ContentID: "c2", ContentType: "tv", Rank: 1, Hash: "slow1", Title: "Slow", Size: 1e9, Seeders: 5, Status: "verified_slow"},
		{ContentID: "c3", ContentType: "tv", Rank: 1, Hash: "healthy1", Title: "Healthy", Size: 1e9, Seeders: 200, Status: "verified_healthy"},
	}
	for _, e := range entries {
		db.UpsertAlternative(e)
	}

	unhealthy, err := db.GetUnhealthyAlternatives()
	if err != nil {
		t.Fatal(err)
	}
	if len(unhealthy) != 2 {
		t.Fatalf("expected 2 unhealthy (dead+slow), got %d", len(unhealthy))
	}
	// Healthy should not be included
	for _, a := range unhealthy {
		if a.Status == "verified_healthy" {
			t.Error("verified_healthy should not be in unhealthy list")
		}
	}
}

func TestAlternatives_IncrementReplacementCount(t *testing.T) {
	db := setupTestDB(t)

	alt := TorrentAlternative{
		ContentID: "rep_test", ContentType: "movie", Rank: 2, Hash: "h1", Title: "T", Size: 1e9, Seeders: 50, Status: "active",
	}
	db.UpsertAlternative(alt)

	if err := db.IncrementReplacementCount("rep_test", "h1"); err != nil {
		t.Fatal(err)
	}

	got, found, _ := db.GetAlternative("rep_test", "h1")
	if !found || got.ReplacementCount != 1 {
		t.Errorf("expected replacement_count=1, got %d", got.ReplacementCount)
	}
}
