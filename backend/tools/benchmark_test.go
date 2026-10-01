package main

import (
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// withFlagsAndArgs resets the package-level flag.CommandLine (main() defines
// its flags on it every call, which panics on redefinition otherwise) and
// swaps os.Args for the duration of fn, restoring both afterwards.
func withFlagsAndArgs(t *testing.T, args []string, fn func()) {
	t.Helper()

	origArgs := os.Args
	origCommandLine := flag.CommandLine
	defer func() {
		os.Args = origArgs
		flag.CommandLine = origCommandLine
	}()

	flag.CommandLine = flag.NewFlagSet(args[0], flag.ContinueOnError)
	os.Args = args

	fn()
}

func TestMain_MissingAPIKey(t *testing.T) {
	withFlagsAndArgs(t, []string{"benchmark", "-url", "http://127.0.0.1:0"}, func() {
		// Should print an error and return without panicking or exiting.
		main()
	})
}

func TestMain_FullRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	withFlagsAndArgs(t, []string{
		"benchmark",
		"-url", srv.URL,
		"-key", "test-key",
		"-c", "2",
		"-n", "4",
		"-t", "2s",
	}, func() {
		main()
	})
}

func TestSendRequest(t *testing.T) {
	t.Run("returns true on 200 OK", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("x-api-key") != "test-key" {
				t.Errorf("expected x-api-key header to be set, got %q", r.Header.Get("x-api-key"))
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("expected Content-Type application/json, got %q", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer srv.Close()

		client := &http.Client{Timeout: 5 * time.Second}
		ok := sendRequest(client, srv.URL, "test-key", []byte(`{}`))
		if !ok {
			t.Errorf("expected sendRequest to return true for 200 response")
		}
	})

	t.Run("returns false on non-200 status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		client := &http.Client{Timeout: 5 * time.Second}
		ok := sendRequest(client, srv.URL, "test-key", []byte(`{}`))
		if ok {
			t.Errorf("expected sendRequest to return false for 500 response")
		}
	})

	t.Run("returns false when request construction fails", func(t *testing.T) {
		client := &http.Client{Timeout: 5 * time.Second}
		// A control character in the URL makes http.NewRequest fail.
		ok := sendRequest(client, "http://\x7f", "test-key", []byte(`{}`))
		if ok {
			t.Errorf("expected sendRequest to return false for invalid URL")
		}
	})

	t.Run("returns false when the connection fails", func(t *testing.T) {
		client := &http.Client{Timeout: 1 * time.Second}
		ok := sendRequest(client, "http://127.0.0.1:1", "test-key", []byte(`{}`))
		if ok {
			t.Errorf("expected sendRequest to return false when the server is unreachable")
		}
	})
}

func TestRunBenchmark(t *testing.T) {
	var successResponses = true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if successResponses {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	cfg := BenchmarkConfig{
		URL:         srv.URL,
		APIKey:      "test-key",
		Concurrency: 4,
		Requests:    10,
		Timeout:     5 * time.Second,
	}

	result := runBenchmark(cfg)

	if result.TotalRequests != 10 {
		t.Errorf("expected TotalRequests=10, got %d", result.TotalRequests)
	}
	if result.SuccessCount != 10 {
		t.Errorf("expected all 10 requests to succeed, got %d", result.SuccessCount)
	}
	if result.ErrorCount != 0 {
		t.Errorf("expected 0 errors, got %d", result.ErrorCount)
	}
	if len(result.Durations) != 10 {
		t.Errorf("expected 10 recorded durations, got %d", len(result.Durations))
	}
	if result.MinDuration > result.MaxDuration {
		t.Errorf("expected MinDuration <= MaxDuration, got min=%v max=%v", result.MinDuration, result.MaxDuration)
	}
	if result.AvgDuration <= 0 {
		t.Errorf("expected AvgDuration > 0, got %v", result.AvgDuration)
	}
	if result.RequestsPerSec <= 0 {
		t.Errorf("expected RequestsPerSec > 0, got %f", result.RequestsPerSec)
	}
}

func TestRunBenchmark_WithFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := BenchmarkConfig{
		URL:         srv.URL,
		APIKey:      "test-key",
		Concurrency: 2,
		Requests:    5,
		Timeout:     5 * time.Second,
	}

	result := runBenchmark(cfg)

	if result.SuccessCount != 0 {
		t.Errorf("expected 0 successes, got %d", result.SuccessCount)
	}
	if result.ErrorCount != 5 {
		t.Errorf("expected 5 errors, got %d", result.ErrorCount)
	}
}

// TestPrintResults exercises all branches of the performance evaluation
// printouts (excellent/good/needs-improvement for success rate, throughput,
// and latency) plus the percentile computation. It only asserts that the
// function does not panic on each shape of input, since its output is
// human-readable console text rather than a return value.
func TestPrintResults(t *testing.T) {
	cases := []BenchmarkResult{
		{
			TotalRequests:  100,
			SuccessCount:   100,
			ErrorCount:     0,
			TotalDuration:  1 * time.Second,
			MinDuration:    10 * time.Millisecond,
			MaxDuration:    100 * time.Millisecond,
			AvgDuration:    200 * time.Millisecond,
			RequestsPerSec: 100,
			Durations:      makeDurations(100),
		},
		{
			TotalRequests:  100,
			SuccessCount:   96,
			ErrorCount:     4,
			TotalDuration:  2 * time.Second,
			MinDuration:    10 * time.Millisecond,
			MaxDuration:    900 * time.Millisecond,
			AvgDuration:    700 * time.Millisecond,
			RequestsPerSec: 30,
			Durations:      makeDurations(50),
		},
		{
			TotalRequests:  100,
			SuccessCount:   50,
			ErrorCount:     50,
			TotalDuration:  5 * time.Second,
			MinDuration:    10 * time.Millisecond,
			MaxDuration:    2 * time.Second,
			AvgDuration:    1500 * time.Millisecond,
			RequestsPerSec: 10,
			Durations:      nil,
		},
	}

	for i, tc := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("case %d: printResults panicked: %v", i, r)
				}
			}()
			printResults(tc)
		}()
	}
}

// makeDurations returns durations in descending order so printResults'
// bubble sort actually performs swaps (an ascending input would never take
// the swap branch), exercising the full percentile computation.
func makeDurations(n int) []time.Duration {
	d := make([]time.Duration, n)
	for i := range d {
		d[i] = time.Duration(n-i) * time.Millisecond
	}
	return d
}
