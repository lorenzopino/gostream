package torrstor

import (
	"os"
	"testing"
)

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
