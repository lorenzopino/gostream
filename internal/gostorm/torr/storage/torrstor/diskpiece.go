package torrstor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

// DiskPiece stores torrent piece data as a mmap-backed file on disk.
// Unlike MemPiece, data persists across process restarts.
type DiskPiece struct {
	piece *Piece
	file  *os.File
	data  []byte // mmap view
	path  string
	size  int64
	mu    sync.Mutex
}

// NewDiskPiece creates a new disk-backed piece.
func NewDiskPiece(p *Piece, piecesDir string) *DiskPiece {
	path := filepath.Join(piecesDir, fmt.Sprintf("%d.dat", p.Id))
	return &DiskPiece{
		piece: p,
		path:  path,
	}
}

// ensureFile opens the backing file and mmaps it if not already done.
func (dp *DiskPiece) ensureFile() error {
	if dp.file != nil {
		return nil
	}

	dp.mu.Lock()
	defer dp.mu.Unlock()

	if dp.file != nil {
		return nil
	}

	dir := filepath.Dir(dp.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(dp.path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}

	pieceLen := dp.piece.cache.pieceLength
	if err := f.Truncate(int64(pieceLen)); err != nil {
		f.Close()
		return err
	}

	data, err := unix.Mmap(int(f.Fd()), 0, int(pieceLen),
		unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		f.Close()
		return err
	}

	dp.file = f
	dp.data = data
	dp.size = 0
	return nil
}

func (dp *DiskPiece) WriteAt(b []byte, off int64) (int, error) {
	if err := dp.ensureFile(); err != nil {
		return 0, err
	}

	dp.mu.Lock()
	defer dp.mu.Unlock()

	if int(off)+len(b) > len(dp.data) {
		return 0, fmt.Errorf("write beyond piece boundary")
	}

	n := copy(dp.data[int(off):], b)
	written := int64(n)
	atomic.AddInt64(&dp.size, written)
	atomic.AddInt64(&dp.piece.Size, written)
	pl := int64(dp.piece.cache.pieceLength)
	if dp.piece.Size > pl {
		atomic.StoreInt64(&dp.piece.Size, pl)
	}
	atomic.StoreInt64(&dp.piece.Accessed, time.Now().Unix())
	return n, nil
}

func (dp *DiskPiece) ReadAt(b []byte, off int64) (int, error) {
	if err := dp.ensureFile(); err != nil {
		return 0, err
	}

	dp.mu.Lock()
	defer dp.mu.Unlock()

	if dp.data == nil {
		dp.piece.Complete = false
		return 0, io.EOF
	}

	currentSize := atomic.LoadInt64(&dp.size)
	if int(off) >= int(currentSize) {
		dp.piece.Complete = false
		return 0, io.EOF
	}

	size := len(b)
	if int64(int(off)+size) > currentSize {
		size = int(currentSize) - int(off)
	}
	if size <= 0 {
		return 0, io.EOF
	}

	n := copy(b, dp.data[int(off):int(off)+size])
	atomic.StoreInt64(&dp.piece.Accessed, time.Now().Unix())
	return n, nil
}

func (dp *DiskPiece) Release() {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	if dp.data != nil {
		unix.Msync(dp.data, unix.MS_SYNC)
		unix.Munmap(dp.data)
		dp.data = nil
	}
	if dp.file != nil {
		dp.file.Close()
		dp.file = nil
	}
	atomic.StoreInt64(&dp.size, 0)
	dp.piece.Complete = false
}

func (dp *DiskPiece) HasData() bool {
	return atomic.LoadInt64(&dp.size) > 0
}

func (dp *DiskPiece) Size() int64 {
	return atomic.LoadInt64(&dp.size)
}

// Delete removes the backing file from disk.
func (dp *DiskPiece) Delete() error {
	dp.Release()
	return os.Remove(dp.path)
}
