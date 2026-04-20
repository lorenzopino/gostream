package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// setupSpotlightBlocking prevents macOS Spotlight and Finder from scanning the FUSE mount.
// It uses three complementary mechanisms:
//
//  1. .metadata_never_index file — tells Spotlight to skip indexing entirely.
//  2. .hidden file — tells Finder to hide metadata files and reduces browsing overhead.
//  3. mdutil -d — disables the Spotlight indexing service for this volume (macOS only).
func setupSpotlightBlocking(mountPath string, sourcePath string) {
	if !globalConfig.DisableSpotlightIndexing {
		logger.Printf("Spotlight blocking is DISABLED (config: disable_spotlight_indexing=false)")
		return
	}

	// Only applicable on macOS
	if runtime.GOOS != "darwin" {
		logger.Printf("Spotlight blocking: skipped (not on macOS)")
		return
	}

	// 1. Create .metadata_never_index in the source directory.
	// macOS mds scans the underlying filesystem and will detect this marker.
	// It's placed in the physical backing dir since the FUSE mount is read-only.
	markPath := filepath.Join(sourcePath, ".metadata_never_index")
	if _, err := os.Stat(markPath); os.IsNotExist(err) {
		if err := os.WriteFile(markPath, []byte(""), 0644); err != nil {
			logger.Printf("Spotlight blocking: FAILED to create .metadata_never_index: %v", err)
		} else {
			logger.Printf("Spotlight blocking: created .metadata_never_index at %s", sourcePath)
		}
	}

	// 2. Create .hidden file to hide metadata files from Finder browsing.
	// Also placed in the source directory.
	hiddenPath := filepath.Join(sourcePath, ".hidden")
	if _, err := os.Stat(hiddenPath); os.IsNotExist(err) {
		hiddenContent := ".metadata_never_index\n.DS_Store\n.Spotlight-V100\n.fseventsd\n"
		if err := os.WriteFile(hiddenPath, []byte(hiddenContent), 0644); err != nil {
			logger.Printf("Spotlight blocking: FAILED to create .hidden: %v", err)
		} else {
			logger.Printf("Spotlight blocking: created .hidden at %s", sourcePath)
		}
	}

	// 3. Run mdutil -d to disable Spotlight indexing for this volume.
	// This is the nuclear option — it tells the OS indexing service to ignore the volume.
	cmd := exec.Command("mdutil", "-d", mountPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		logger.Printf("Spotlight blocking: mdutil -d failed: %v (output: %s)", err, string(output))
	} else {
		logger.Printf("Spotlight blocking: mdutil -d succeeded for %s", mountPath)
	}
}
