package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnforceDiskCacheQuota(t *testing.T) {
	dir := t.TempDir()

	oldDir := filepath.Join(dir, "old_hash")
	newDir := filepath.Join(dir, "new_hash")
	os.MkdirAll(filepath.Join(oldDir, "pieces"), 0755)
	os.MkdirAll(filepath.Join(newDir, "pieces"), 0755)

	os.WriteFile(filepath.Join(oldDir, "pieces", "0.dat"), make([]byte, 100), 0644)
	os.WriteFile(filepath.Join(newDir, "pieces", "0.dat"), make([]byte, 100), 0644)

	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	os.Chtimes(oldDir, oldTime, oldTime)

	quotaBytes := int64(150)

	removed := enforceDiskCacheQuota(dir, quotaBytes)
	if removed == 0 {
		t.Fatal("Expected some directories to be removed")
	}

	if _, err := os.Stat(oldDir); err == nil {
		t.Fatal("Old torrent dir should have been evicted")
	}

	if _, err := os.Stat(newDir); err != nil {
		t.Fatal("New torrent dir should still exist")
	}
}
