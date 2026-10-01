package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

// ErrUpstreamStalled indicates the upstream provider did not send any data
// within the configured stall timeout during pre-stream verification.
var ErrUpstreamStalled = fmt.Errorf("upstream stalled: no data received within stall timeout")

// StallReadResult contains the result of a pre-stream read attempt.
type StallReadResult struct {
	// FirstData contains the first chunk of data read from upstream.
	FirstData []byte
	// Reader replays FirstData and then reads from the original upstream body.
	// It remains closable so downstream converters can interrupt the stream.
	Reader io.ReadCloser
	// Err is set if the read failed (including stall timeout).
	Err error
}

type replayReadCloser struct {
	reader io.Reader
	closer io.Closer
	once   sync.Once
	err    error
}

func (r *replayReadCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *replayReadCloser) Close() error {
	r.once.Do(func() {
		r.err = r.closer.Close()
	})
	return r.err
}

// WaitForFirstData reads the first chunk from upstream with a timeout.
// If no data arrives within stallTimeout, returns ErrUpstreamStalled.
// On success, returns a combined reader that replays the peeked data
// followed by the remaining stream, so no data is lost.
func WaitForFirstData(ctx context.Context, reader io.ReadCloser, stallTimeout time.Duration) StallReadResult {
	logger := utils.GetLogger()
	if stallTimeout <= 0 {
		stallTimeout = 60 * time.Second
	}
	logger.Info("  [stall-detector] Waiting for first data from upstream (timeout: %v)", stallTimeout)

	type readResult struct {
		data []byte
		err  error
	}

	ch := make(chan readResult, 1)
	buf := make([]byte, 64*1024)

	go func() {
		n, err := reader.Read(buf)
		ch <- readResult{buf[:n], err}
	}()

	select {
	case result := <-ch:
		if len(result.data) > 0 {
			logger.Info("  [stall-detector] First data received (%d bytes), upstream is responsive", len(result.data))
			combinedReader := &replayReadCloser{
				reader: io.MultiReader(bytes.NewReader(result.data), reader),
				closer: reader,
			}
			return StallReadResult{
				FirstData: result.data,
				Reader:    combinedReader,
			}
		}
		if result.err != nil {
			logger.Warn("  [stall-detector] Read error before first data: %v", result.err)
			return StallReadResult{Err: fmt.Errorf("read error during pre-stream check: %w", result.err)}
		}
		return StallReadResult{Err: io.ErrNoProgress}
	case <-time.After(stallTimeout):
		_ = reader.Close()
		logger.Warn("  [stall-detector] Upstream stalled! No data for %v, will retry", stallTimeout)
		return StallReadResult{Err: ErrUpstreamStalled}
	case <-ctx.Done():
		_ = reader.Close()
		logger.Info("  [stall-detector] Client disconnected during pre-stream check")
		return StallReadResult{Err: ctx.Err()}
	}
}
