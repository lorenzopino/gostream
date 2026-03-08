package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// EpisodeEntry matches the Python registry format
type EpisodeEntry struct {
	QualityScore int    `json:"quality_score"`
	Hash         string `json:"hash"`
	FilePath     string `json:"file_path"`
	Source       string `json:"source"`
	Created      int64  `json:"created"`
}

var registryMutex sync.Mutex

// GetRegistryPath returns the path to tv_episode_registry.json
func GetRegistryPath() string {
	return filepath.Join(GetStateDir(), "tv_episode_registry.json")
}

// StartRegistryWatchdog runs the self-healing check at startup and then every 24 hours
func StartRegistryWatchdog(stopChan chan struct{}) {
	// 1. Initial check at startup
	SyncRegistryWithDisk()

	// 2. Periodic check every 24 hours
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	logger.Printf("[Registry] Watchdog active (interval: 24h)")

	for {
		select {
		case <-ticker.C:
			SyncRegistryWithDisk()
		case <-stopChan:
			logger.Printf("[Registry] Watchdog stopping")
			return
		}
	}
}

// SyncRegistryWithDisk cleans up orphaned entries in tv_episode_registry.json (V1.4.6-Fix)
func SyncRegistryWithDisk() {
	registryMutex.Lock()
	defer registryMutex.Unlock()

	path := GetRegistryPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	logger.Printf("[Registry] Starting self-healing check: %s", path)

	// 1. Read Registry
	data, err := ioutil.ReadFile(path)
	if err != nil {
		logger.Printf("[Registry] ERROR: could not read registry: %v", err)
		return
	}

	var registry map[string]EpisodeEntry
	if err := json.Unmarshal(data, &registry); err != nil {
		logger.Printf("[Registry] ERROR: could not parse registry: %v", err)
		return
	}

	// 2. Filter missing files
	initialCount := len(registry)
	newRegistry := make(map[string]EpisodeEntry)
	removed := 0

	for key, entry := range registry {
		if _, err := os.Stat(entry.FilePath); err == nil {
			newRegistry[key] = entry
		} else {
			removed++
		}
	}

	if removed == 0 {
		logger.Printf("[Registry] Audit complete: 0 ghost entries found (Total: %d)", initialCount)
		return
	}

	// 3. Save Registry (with lock to be Python-friendly)
	if err := saveRegistryLocked(path, newRegistry); err != nil {
		logger.Printf("[Registry] ERROR: could not save cleaned registry: %v", err)
	} else {
		logger.Printf("[Registry] Self-Healing: Purged %d ghost entries. Remaining: %d", removed, len(newRegistry))
	}
}

// RemoveFromRegistry removes a specific file path from the registry (V1.4.6-Fix)
func RemoveFromRegistry(filePath string) {
	registryMutex.Lock()
	defer registryMutex.Unlock()

	path := GetRegistryPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}

	var registry map[string]EpisodeEntry
	if err := json.Unmarshal(data, &registry); err != nil {
		return
	}

	removed := false
	for key, entry := range registry {
		if entry.FilePath == filePath {
			delete(registry, key)
			removed = true
			logger.Printf("[Registry] Real-time: Removed deleted file from registry: %s", filepath.Base(filePath))
			break
		}
	}

	if removed {
		saveRegistryLocked(path, registry)
	}
}

// saveRegistryLocked saves the registry using syscall.Flock to coordinate with Python scripts
func saveRegistryLocked(path string, registry map[string]EpisodeEntry) error {
	// 1. Open file
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	// 2. Lock file (Exclusive, blocking) - Matches Python's fcntl.flock(f, fcntl.LOCK_EX)
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("could not acquire lock: %v", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	// 3. Encode JSON
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}

	// 4. Write (Truncate then write)
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}
