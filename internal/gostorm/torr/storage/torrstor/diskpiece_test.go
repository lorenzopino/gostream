package torrstor

import (
	"os"
	"testing"

	"gostream/internal/gostorm/settings"

	"github.com/anacrolix/torrent/metainfo"
)

func init() {
	if settings.BTsets == nil {
		settings.BTsets = &settings.BTSets{
			TorrentsSavePath: os.TempDir(),
		}
	}
}

func mustHashDisk(s string) metainfo.Hash {
	return metainfo.NewHashFromHex(s)
}

func TestDiskPiece_WriteAndRead(t *testing.T) {
	dir := t.TempDir()
	cache := &Cache{
		pieceLength: 4096,
		pieces:      make(map[int]*Piece),
	}
	p := &Piece{Id: 0, cache: cache}
	dp := NewDiskPiece(p, dir)

	data := []byte("hello disk piece world")
	n, err := dp.WriteAt(data, 0)
	if err != nil {
		t.Fatalf("WriteAt error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("WriteAt wrote %d bytes, expected %d", n, len(data))
	}

	buf := make([]byte, len(data))
	n, err = dp.ReadAt(buf, 0)
	if err != nil {
		t.Fatalf("ReadAt error: %v", err)
	}
	if string(buf[:n]) != string(data) {
		t.Fatalf("ReadAt got %q, expected %q", buf[:n], data)
	}
}

func TestDiskPiece_HasData(t *testing.T) {
	dir := t.TempDir()
	cache := &Cache{pieceLength: 4096, pieces: make(map[int]*Piece)}
	p := &Piece{Id: 0, cache: cache}
	dp := NewDiskPiece(p, dir)

	if dp.HasData() {
		t.Fatal("HasData should be false for empty piece")
	}

	dp.WriteAt([]byte("test"), 0)
	if !dp.HasData() {
		t.Fatal("HasData should be true after write")
	}
}

func TestDiskPiece_Release_Persists(t *testing.T) {
	dir := t.TempDir()
	cache := &Cache{pieceLength: 4096, pieces: make(map[int]*Piece)}
	p := &Piece{Id: 0, cache: cache}
	dp := NewDiskPiece(p, dir)

	dp.WriteAt([]byte("persist me"), 0)
	dp.Release()

	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Fatal("Release should persist file on disk")
	}
}

func TestPiece_UsesDiskPiece(t *testing.T) {
	cache := &Cache{
		pieceLength: 4096,
		pieces:      make(map[int]*Piece),
		hash:        mustHashDisk("0102030000000000000000000000000000000000"),
	}
	p := NewPiece(0, cache)

	if p.dPiece == nil {
		t.Fatal("NewPiece should create DiskPiece by default")
	}

	data := []byte("test piece data")
	n, err := p.WriteAt(data, 0)
	if err != nil {
		t.Fatalf("Piece.WriteAt error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Piece.WriteAt wrote %d bytes, expected %d", n, len(data))
	}

	buf := make([]byte, len(data))
	n, err = p.ReadAt(buf, 0)
	if err != nil {
		t.Fatalf("Piece.ReadAt error: %v", err)
	}
	if string(buf[:n]) != string(data) {
		t.Fatalf("Piece.ReadAt got %q, expected %q", buf[:n], data)
	}
}

func TestCache_Init_RestoresPiecesFromDisk(t *testing.T) {
	dir := t.TempDir()
	settings.BTsets.TorrentsSavePath = dir

	// Create first cache, write a piece, release (persist to disk)
	hash1 := mustHashDisk("0102030000000000000000000000000000000000")
	cache1 := &Cache{
		pieceLength: 4096,
		pieces:      make(map[int]*Piece),
		hash:        hash1,
		cleanStop:   make(chan struct{}),
	}
	cache1.pieces[0] = NewPiece(0, cache1)

	// Write data to piece 0
	p := cache1.pieces[0]
	p.WriteAt([]byte("persistent data"), 0)
	p.dPiece.Release() // persist to disk via DiskPiece directly (skip torrent priority update)

	// Simulate "restart" - create new Cache instance for same hash
	cache2 := &Cache{
		pieceLength: 4096,
		pieces:      make(map[int]*Piece),
		hash:        hash1,
		cleanStop:   make(chan struct{}),
	}
	cache2.pieces[0] = NewPiece(0, cache2)

	// Init should restore piece 0 from disk
	cache2.restorePiecesFromDisk()

	if cache2.pieces[0] == nil {
		t.Fatal("Cache.Init should restore piece 0")
	}

	if !cache2.pieces[0].dPiece.HasData() {
		t.Fatal("Restored piece should have data")
	}

	// Verify data persisted
	buf := make([]byte, 15)
	n, err := cache2.pieces[0].ReadAt(buf, 0)
	if err != nil {
		t.Fatalf("ReadAt after restore: %v", err)
	}
	if string(buf[:n]) != "persistent data" {
		t.Fatalf("Restored data mismatch: got %q", buf[:n])
	}
}
