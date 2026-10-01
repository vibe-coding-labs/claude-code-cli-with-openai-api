package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

type blockingReadCloser struct {
	started chan struct{}
	once    sync.Once
	closed  chan struct{}
}

func newBlockingReadCloser() *blockingReadCloser {
	return &blockingReadCloser{started: make(chan struct{}), closed: make(chan struct{})}
}

func (r *blockingReadCloser) Read([]byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-r.closed
	return 0, io.ErrClosedPipe
}

func (r *blockingReadCloser) Close() error {
	select {
	case <-r.closed:
	default:
		close(r.closed)
	}
	return nil
}

func TestWaitForFirstDataPreservesDataWithEOF(t *testing.T) {
	reader := io.NopCloser(bytes.NewReader([]byte("data: {}\n\n")))
	result := WaitForFirstData(context.Background(), reader, time.Second)
	if result.Err != nil {
		t.Fatalf("WaitForFirstData() error = %v", result.Err)
	}
	got, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("reading replayed stream: %v", err)
	}
	if string(got) != "data: {}\n\n" {
		t.Fatalf("replayed stream = %q", got)
	}
}

func TestWaitForFirstDataClosesReaderOnTimeout(t *testing.T) {
	reader := newBlockingReadCloser()
	result := WaitForFirstData(context.Background(), reader, 10*time.Millisecond)
	if !errors.Is(result.Err, ErrUpstreamStalled) {
		t.Fatalf("error = %v, want ErrUpstreamStalled", result.Err)
	}
	select {
	case <-reader.closed:
	case <-time.After(time.Second):
		t.Fatal("reader was not closed after timeout")
	}
	select {
	case <-reader.started:
	case <-time.After(time.Second):
		t.Fatal("read goroutine did not start")
	}
}

func TestWaitForFirstDataClosesReaderOnCancel(t *testing.T) {
	reader := newBlockingReadCloser()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := WaitForFirstData(ctx, reader, time.Second)
	if !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", result.Err)
	}
	select {
	case <-reader.closed:
	case <-time.After(time.Second):
		t.Fatal("reader was not closed after cancellation")
	}
}

type readOnceCloser struct {
	io.Reader
	target *blockingReadCloser
}

func (r *readOnceCloser) Close() error { return r.target.Close() }

func TestWaitForFirstDataReturnsClosableReplayReader(t *testing.T) {
	reader := newBlockingReadCloser()
	result := WaitForFirstData(context.Background(), &readOnceCloser{
		Reader: bytes.NewReader([]byte("first\nrest")),
		target: reader,
	}, time.Second)
	if result.Err != nil {
		t.Fatalf("WaitForFirstData() error = %v", result.Err)
	}
	got, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("reading replay reader: %v", err)
	}
	if string(got) != "first\nrest" {
		t.Fatalf("replayed stream = %q", got)
	}
	if err := result.Reader.Close(); err != nil {
		t.Fatalf("closing replay reader: %v", err)
	}
	select {
	case <-reader.closed:
	case <-time.After(time.Second):
		t.Fatal("closing replay reader did not close upstream")
	}
}

func TestWaitForFirstDataUsesDefaultForNonPositiveTimeout(t *testing.T) {
	reader := io.NopCloser(bytes.NewReader([]byte("ok")))
	result := WaitForFirstData(context.Background(), reader, 0)
	if result.Err != nil {
		t.Fatalf("WaitForFirstData() error = %v", result.Err)
	}
}
