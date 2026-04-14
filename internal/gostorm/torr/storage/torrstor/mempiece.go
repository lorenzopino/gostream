package torrstor

import (
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// V96: Global buffer pool to reuse memory pieces and reduce GC pressure
var memBufferPool = sync.Pool{
	New: func() interface{} {
		return nil // Return nil so we can handle specific piece length allocations
	},
}

type MemPiece struct {
	piece *Piece

	buffer []byte
	mu     sync.RWMutex
}

func NewMemPiece(p *Piece) *MemPiece {
	return &MemPiece{piece: p}
}

func (p *MemPiece) WriteAt(b []byte, off int64) (n int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.buffer == nil {
		// V227: Non-blocking rate-limited cleanup trigger
		select {
		case p.piece.cache.cleanTrigger <- struct{}{}:
		default:
		}

		// V96: Try to get a buffer from the pool
		if pooledBuf := memBufferPool.Get(); pooledBuf != nil {
			buf := pooledBuf.([]byte)
			// Check if the pooled buffer has the correct capacity
			if int64(len(buf)) == p.piece.cache.pieceLength {
				p.buffer = buf
			}
		}

		// If pool was empty or size mismatch, allocate once
		if p.buffer == nil {
			p.buffer = make([]byte, p.piece.cache.pieceLength, p.piece.cache.pieceLength)
		}
	}
	n = copy(p.buffer[off:], b[:])
	p.piece.Size += int64(n)
	if p.piece.Size > p.piece.cache.pieceLength {
		p.piece.Size = p.piece.cache.pieceLength
	}
	atomic.StoreInt64(&p.piece.Accessed, time.Now().Unix())
	return
}

func (p *MemPiece) ReadAt(b []byte, off int64) (n int, err error) {
	p.mu.RLock()
	// Snapshot buffer while holding RLock so we can release before the slow path.
	buf := p.buffer
	p.mu.RUnlock()

	// V-busyloop-fix: Buffer absent or truncated at the requested offset means the
	// piece is corrupt / was evicted mid-stream. Marking it incomplete forces
	// updatePieceCompletion() to see a state change (true→false) and trigger a
	// re-download from peers instead of spinning in readOnceAt indefinitely.
	if buf == nil || int(off) >= len(buf) {
		p.piece.Complete = false
		return 0, io.EOF
	}

	size := len(b)
	if size+int(off) > len(buf) {
		size = len(buf) - int(off)
		if size < 0 {
			size = 0
		}
	}
	if len(buf) < int(off)+size {
		// Remaining buffer is genuinely shorter than requested: mark incomplete
		// so the engine re-fetches this piece rather than looping on EOF.
		p.piece.Complete = false
		return 0, io.EOF
	}
	n = copy(b, buf[int(off):int(off)+size])
	atomic.StoreInt64(&p.piece.Accessed, time.Now().Unix())
	if int64(len(b))+off >= p.piece.Size {
		// V227: Non-blocking rate-limited cleanup trigger
		select {
		case p.piece.cache.cleanTrigger <- struct{}{}:
		default:
		}
	}
	if n == 0 {
		p.piece.Complete = false
		return 0, io.EOF
	}
	return n, nil
}

func (p *MemPiece) Release() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.buffer != nil {
		// V96: Put the buffer back into the pool for future reuse
		memBufferPool.Put(p.buffer)
		p.buffer = nil
	}
	p.piece.Size = 0
	p.piece.Complete = false
}
