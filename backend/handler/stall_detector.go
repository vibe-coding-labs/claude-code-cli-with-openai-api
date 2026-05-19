package handler

import (
	"bytes"
	"context"
	"fmt"
	"io"
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
	// Reader is an io.Reader that replays FirstData followed by the remaining stream.
	Reader io.Reader
	// Err is set if the read failed (including stall timeout).
	Err error
}

// WaitForFirstData reads the first chunk from upstream with a timeout.
// If no data arrives within stallTimeout, returns ErrUpstreamStalled.
// On success, returns a combined reader that replays the peeked data
// followed by the remaining stream, so no data is lost.
func WaitForFirstData(ctx context.Context, reader io.ReadCloser, stallTimeout time.Duration) StallReadResult {
	logger := utils.GetLogger()
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
		if result.err != nil {
			logger.Warn("  [stall-detector] Read error before first data: %v", result.err)
			return StallReadResult{Err: fmt.Errorf("read error during pre-stream check: %w", result.err)}
		}
		logger.Info("  [stall-detector] First data received (%d bytes), upstream is responsive", len(result.data))
		combinedReader := io.MultiReader(bytes.NewReader(result.data), reader)
		return StallReadResult{
			FirstData: result.data,
			Reader:    combinedReader,
		}
	case <-time.After(stallTimeout):
		logger.Warn("  [stall-detector] Upstream stalled! No data for %v, will retry", stallTimeout)
		return StallReadResult{Err: ErrUpstreamStalled}
	case <-ctx.Done():
		logger.Info("  [stall-detector] Client disconnected during pre-stream check")
		return StallReadResult{Err: ctx.Err()}
	}
}
