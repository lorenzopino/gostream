package torr

import (
	"testing"
)

func TestTorrent_SetUploadLimit(t *testing.T) {
	torrent := &Torrent{}
	torrent.SetUploadLimit(0)

	if torrent.uploadLimit != 0 {
		t.Fatalf("expected upload limit 0, got %d", torrent.uploadLimit)
	}

	torrent.SetUploadLimit(1024 * 1024) // 1 MB/s
	if torrent.uploadLimit != 1024*1024 {
		t.Fatalf("expected upload limit 1048576, got %d", torrent.uploadLimit)
	}
}

func TestTorrent_SetSeedMode(t *testing.T) {
	torrent := &Torrent{}
	torrent.SetSeedMode(false)

	if torrent.seedMode {
		t.Fatal("expected seedMode to be false")
	}

	torrent.SetSeedMode(true)
	if !torrent.seedMode {
		t.Fatal("expected seedMode to be true")
	}
}
