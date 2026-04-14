package health

import (
	"context"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"gostream/internal/metadb"
)

type testLogger struct{ t *testing.T }

func (l *testLogger) Printf(format string, v ...interface{}) { l.t.Logf(format, v...) }

// mockTester implements TorrentTester for testing.
type mockTester struct {
	mu       sync.Mutex
	results  map[string]mockResult // hash → result
	statuses map[string]mockStatus // hash → current status
}

type mockResult struct {
	speedKBps int64
	seeders   int
	err       error
}

type mockStatus struct {
	speedKBps int64
	seeders   int
	active    bool
}

func (m *mockTester) TestTorrent(ctx context.Context, hash, magnet, title string) (int64, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.results[hash]; ok {
		return r.speedKBps, r.seeders, r.err
	}
	return 0, 0, context.Canceled
}

func (m *mockTester) CurrentTorrentStatus(ctx context.Context, hash string) (int64, int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.statuses[hash]; ok {
		return s.speedKBps, s.seeders, s.active
	}
	return 0, 0, false
}

func (m *mockTester) SetResult(hash string, speed int64, seeders int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results[hash] = mockResult{speedKBps: speed, seeders: seeders, err: err}
}

func (m *mockTester) SetActive(hash string, speed int64, seeders int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statuses[hash] = mockStatus{speedKBps: speed, seeders: seeders, active: true}
}

// mockReplacer implements TorrentReplacer for testing.
type mockReplacer struct {
	mu           sync.Mutex
	replacements []replacement
	failOn       map[string]bool // contentID → fail
}

type replacement struct {
	contentID string
	oldHash   string
	newHash   string
}

func (m *mockReplacer) ReplaceTorrent(ctx context.Context, contentID, oldHash, newHash, newMagnet, newTitle string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failOn[contentID] {
		return false, nil
	}
	m.replacements = append(m.replacements, replacement{contentID, oldHash, newHash})
	return true, nil
}

func (m *mockReplacer) ReplacementCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.replacements)
}

func (m *mockReplacer) FailOn(contentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failOn == nil {
		m.failOn = make(map[string]bool)
	}
	m.failOn[contentID] = true
}

func setupIntegrationTest(t *testing.T) (*metadb.DB, *mockTester, *mockReplacer, *OfflineHealthChecker) {
	t.Helper()
	dir := t.TempDir()
	dbPath := dir + "/test.db"
	db, err := metadb.New(dbPath, &testLogger{t})
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	tester := &mockTester{
		results:  make(map[string]mockResult),
		statuses: make(map[string]mockStatus),
	}
	replacer := &mockReplacer{replacements: []replacement{}}
	logger := log.New(os.Stderr, "[TEST] ", 0)

	cfg := DefaultConfig()
	cfg.MaxReplacements = 10
	checker := New(db, tester, replacer, cfg, logger)

	return db, tester, replacer, checker
}

// TestHealthChecker_CheckAndUpdate_Dead verifies that a dead torrent is marked as dead.
func TestHealthChecker_CheckAndUpdate_Dead(t *testing.T) {
	db, tester, _, checker := setupIntegrationTest(t)

	alt := metadb.TorrentAlternative{
		ContentID:   "tt123_s01e01",
		ContentType: "tv",
		Rank:        1,
		Hash:        "deadhash1234567890abcdef1234567890abcd",
		Title:       "Dead Torrent Title",
		Status:      "dead",
	}
	db.UpsertAlternative(alt)

	// Torrent test fails → dead
	tester.SetResult(alt.Hash, 0, 0, context.Canceled)

	checker.checkAndUpdate(context.Background(), &alt, time.Now().Unix())

	got, found, _ := db.GetAlternative(alt.ContentID, alt.Hash)
	if !found {
		t.Fatal("alternative not found")
	}
	if got.Status != "dead" {
		t.Errorf("expected status dead, got %q", got.Status)
	}
}

// TestHealthChecker_CheckAndUpdate_Healthy verifies that a healthy torrent is marked correctly.
func TestHealthChecker_CheckAndUpdate_Healthy(t *testing.T) {
	db, tester, _, checker := setupIntegrationTest(t)

	alt := metadb.TorrentAlternative{
		ContentID:   "tt456_s01e01",
		ContentType: "tv",
		Rank:        2,
		Hash:        "healthyhash1234567890abcdef1234567890ab",
		Title:       "Healthy Torrent Title",
		Status:      "active",
	}
	db.UpsertAlternative(alt)

	// Torrent test returns good speed and seeders
	tester.SetResult(alt.Hash, 600, 50, nil)

	checker.checkAndUpdate(context.Background(), &alt, time.Now().Unix())

	got, found, _ := db.GetAlternative(alt.ContentID, alt.Hash)
	if !found {
		t.Fatal("alternative not found")
	}
	if got.Status != "verified_healthy" {
		t.Errorf("expected verified_healthy, got %q", got.Status)
	}
	if got.AvgSpeedKBps != 600 {
		t.Errorf("expected speed 600, got %d", got.AvgSpeedKBps)
	}
}

// TestHealthChecker_TryReplace_Success verifies that a dead torrent is replaced with a better alternative.
func TestHealthChecker_TryReplace_Success(t *testing.T) {
	db, tester, replacer, checker := setupIntegrationTest(t)

	dead := metadb.TorrentAlternative{
		ContentID:    "tt789",
		ContentType:  "movie",
		Rank:         1,
		Hash:         "deadhash0000000000000000000000000000ab",
		Title:        "Dead Movie",
		Status:       "dead",
		AvgSpeedKBps: 50,
	}
	db.UpsertAlternative(dead)

	// Alternative with much better speed
	alt := metadb.TorrentAlternative{
		ContentID:   "tt789",
		ContentType: "movie",
		Rank:        2,
		Hash:        "besthash0000000000000000000000000000ab",
		Title:       "Best Alternative",
		Status:      "active",
	}
	db.UpsertAlternative(alt)

	// Alternative test returns 3x better speed
	tester.SetResult(alt.Hash, 800, 30, nil)

	replaced := checker.tryReplace(context.Background(), &dead)
	if !replaced {
		t.Fatal("expected replacement to succeed")
	}
	if replacer.ReplacementCount() != 1 {
		t.Errorf("expected 1 replacement, got %d", replacer.ReplacementCount())
	}

	// Verify statuses updated
	newAlt, _, _ := db.GetAlternative(alt.ContentID, alt.Hash)
	if newAlt.Status != "active" {
		t.Errorf("replacement should be active, got %q", newAlt.Status)
	}
}

// TestHealthChecker_TryReplace_NoBetter verifies that no replacement occurs when alternative isn't better.
func TestHealthChecker_TryReplace_NoBetter(t *testing.T) {
	db, tester, replacer, checker := setupIntegrationTest(t)

	dead := metadb.TorrentAlternative{
		ContentID:    "tt999",
		ContentType:  "movie",
		Rank:         1,
		Hash:         "slowhash0000000000000000000000000000ab",
		Title:        "Slow Movie",
		Status:       "verified_slow",
		AvgSpeedKBps: 300,
	}
	db.UpsertAlternative(dead)

	// Alternative with similar (not better) speed
	alt := metadb.TorrentAlternative{
		ContentID:   "tt999",
		ContentType: "movie",
		Rank:        2,
		Hash:        "mehash000000000000000000000000000000ab",
		Title:       "Meh Alternative",
		Status:      "active",
	}
	db.UpsertAlternative(alt)

	// Alternative returns similar speed (not 1.5x better)
	tester.SetResult(alt.Hash, 350, 20, nil)

	replaced := checker.tryReplace(context.Background(), &dead)
	if replaced {
		t.Error("expected replacement to fail (not significantly better)")
	}
	if replacer.ReplacementCount() != 0 {
		t.Errorf("expected 0 replacements, got %d", replacer.ReplacementCount())
	}

	// Alternative should be marked as tested_no_better
	got, found, _ := db.GetAlternative(alt.ContentID, alt.Hash)
	if !found || got.Status != "tested_no_better" {
		t.Errorf("alternative should be tested_no_better, got %q", got.Status)
	}
}

// TestHealthChecker_TryReplace_NoAlternatives verifies behavior when no alternatives exist.
func TestHealthChecker_TryReplace_NoAlternatives(t *testing.T) {
	db, _, _, checker := setupIntegrationTest(t)

	dead := metadb.TorrentAlternative{
		ContentID:   "tt000",
		ContentType: "movie",
		Rank:        1,
		Hash:        "onlyhash000000000000000000000000000ab",
		Title:       "Only Torrent",
		Status:      "dead",
	}
	db.UpsertAlternative(dead)

	replaced := checker.tryReplace(context.Background(), &dead)
	if replaced {
		t.Error("expected replacement to fail (no alternatives)")
	}
}

func TestHealthChecker_TryReplace_SlowTorrentCanPreferSmallerStreamableAlternative(t *testing.T) {
	db, tester, replacer, checker := setupIntegrationTest(t)

	slow := metadb.TorrentAlternative{
		ContentID:    "ttsmall",
		ContentType:  "movie",
		Rank:         1,
		Hash:         "slowlarge0000000000000000000000000000ab",
		Title:        "Movie.2025.1080p.WEB-DL",
		Size:         int64(5 * 1024 * 1024 * 1024),
		Seeders:      4,
		QualityScore: 900,
		Status:       "verified_slow",
		AvgSpeedKBps: 300,
	}
	db.UpsertAlternative(slow)

	compact := metadb.TorrentAlternative{
		ContentID:    "ttsmall",
		ContentType:  "movie",
		Rank:         2,
		Hash:         "compact000000000000000000000000000000ab",
		Title:        "Movie.2025.480p.WEBRip.x265",
		Size:         int64(650 * 1024 * 1024),
		Seeders:      25,
		QualityScore: 500,
		Status:       "active",
	}
	db.UpsertAlternative(compact)
	tester.SetResult(compact.Hash, 360, 25, nil)

	replaced := checker.tryReplace(context.Background(), &slow)
	if !replaced {
		t.Fatal("expected slow large torrent to be replaced by smaller streamable alternative")
	}
	if replacer.ReplacementCount() != 1 {
		t.Fatalf("expected one replacement, got %d", replacer.ReplacementCount())
	}
}
