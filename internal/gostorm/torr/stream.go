package torr

import (
	// "context"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/anacrolix/missinggo/v2/httptoo"

	mt "gostream/internal/gostorm/mimetype"
	sets "gostream/internal/gostorm/settings"
	"gostream/internal/gostorm/torr/storage/torrstor"
)

// Add atomic counter for concurrent streams
var activeStreams int32

type contextResponseWriter struct {
	http.ResponseWriter
	ctx context.Context
}

func (w *contextResponseWriter) Write(p []byte) (n int, err error) {
	// Check context before each write
	select {
	case <-w.ctx.Done():
		return 0, w.ctx.Err()
	default:
		return w.ResponseWriter.Write(p)
	}
}

// contextReader wraps an io.ReadSeeker to abort reads when context is cancelled.
// Prevents torrent reader from continuing after client disconnects.
type contextReader struct {
	r   io.ReadSeeker
	ctx context.Context
}

func (cr *contextReader) Read(p []byte) (n int, err error) {
	select {
	case <-cr.ctx.Done():
		return 0, cr.ctx.Err()
	default:
		return cr.r.Read(p)
	}
}

func (cr *contextReader) Seek(offset int64, whence int) (int64, error) {
	select {
	case <-cr.ctx.Done():
		return 0, cr.ctx.Err()
	default:
		return cr.r.Seek(offset, whence)
	}
}

func (t *Torrent) Stream(fileID int, req *http.Request, resp http.ResponseWriter) error {
	// Increment active streams counter
	streamID := atomic.AddInt32(&activeStreams, 1)
	defer atomic.AddInt32(&activeStreams, -1)
	// Stream disconnect timeout (same as torrent)
	streamTimeout := sets.BTsets.TorrentDisconnectTimeout

	if !t.GotInfo() {
		http.NotFound(resp, req)
		return errors.New("torrent doesn't have info yet")
	}
	// Get file information directly without Status() overhead.
	file := t.getFileByID(fileID)
	if file == nil {
		return fmt.Errorf("file with id %v not found", fileID)
	}
	// Check file size limit
	if int64(sets.MaxSize) > 0 && file.Length() > int64(sets.MaxSize) {
		err := fmt.Errorf("file size exceeded max allowed %d bytes", sets.MaxSize)
		log.Printf("File %s size (%d) exceeded max allowed %d bytes", file.DisplayPath(), file.Length(), sets.MaxSize)
		http.Error(resp, err.Error(), http.StatusForbidden)
		return err
	}
	// Create reader with context for timeout
	reader := t.NewReader(file)
	if reader == nil {
		return errors.New("cannot create torrent reader")
	}

	// Ensure reader is always closed
	defer t.CloseReader(reader)

	// V303: Use thread-safe AdaptiveShield check
	if torrstor.IsResponsive() {
		reader.SetResponsive()
	}
	// Log connection
	host, port, clerr := net.SplitHostPort(req.RemoteAddr)

	if sets.BTsets.EnableDebug {
		if clerr != nil {
			log.Printf("[Stream:%d] Connect client (Active streams: %d)", streamID, atomic.LoadInt32(&activeStreams))
		} else {
			log.Printf("[Stream:%d] Connect client %s:%s (Active streams: %d)",
				streamID, host, port, atomic.LoadInt32(&activeStreams))
		}
	}

	// Set response headers
	resp.Header().Set("Connection", "close")
	// Add timeout header if configured
	if streamTimeout > 0 {
		resp.Header().Set("X-Stream-Timeout", fmt.Sprintf("%d", streamTimeout))
	}
	// Add ETag
	etag := hex.EncodeToString([]byte(fmt.Sprintf("%s/%s", t.Hash().HexString(), file.Path())))
	resp.Header().Set("ETag", httptoo.EncodeQuotedString(etag))
	// DLNA headers
	resp.Header().Set("transferMode.dlna.org", "Streaming")
	// add MimeType
	mime, err := mt.MimeTypeByPath(file.Path())
	if err == nil && mime.IsMedia() {
		resp.Header().Set("content-type", mime.String())
	}
	// DLNA Seek (hardcoded to drop anacrolix/dms dependency)
	if req.Header.Get("getContentFeatures.dlna.org") != "" {
		resp.Header().Set("contentFeatures.dlna.org",
			"DLNA.ORG_OP=11;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01700000000000000000000000000000")
	}
	// Add support for range requests
	if req.Header.Get("Range") != "" {
		resp.Header().Set("Accept-Ranges", "bytes")
	}
	// Create a context with timeout if configured
	ctx := req.Context()
	if streamTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(streamTimeout)*time.Second)
		defer cancel()
	}
	// Update request with new context
	req = req.WithContext(ctx)
	// Handle client disconnections better
	wrappedResp := &contextResponseWriter{
		ResponseWriter: resp,
		ctx:            ctx,
	}
	// V320: Wrap reader to abort reads on context cancellation (client disconnect).
	wrappedReader := &contextReader{r: reader, ctx: ctx}
	http.ServeContent(wrappedResp, req, file.Path(), time.Unix(t.Timestamp, 0), wrappedReader)

	if sets.BTsets.EnableDebug {
		if clerr != nil {
			log.Printf("[Stream:%d] Disconnect client", streamID)
		} else {
			log.Printf("[Stream:%d] Disconnect client %s:%s", streamID, host, port)
		}
	}
	return nil
}

// GetActiveStreams returns number of currently active streams
func GetActiveStreams() int32 {
	return atomic.LoadInt32(&activeStreams)
}
