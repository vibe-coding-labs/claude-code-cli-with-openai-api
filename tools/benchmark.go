package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type BenchmarkConfig struct {
	URL         string
	APIKey      string
	Concurrency int
	Requests    int
	Timeout     time.Duration
}

type BenchmarkResult struct {
	TotalRequests  int
	SuccessCount   int64
	ErrorCount     int64
	TotalDuration  time.Duration
	MinDuration    time.Duration
	MaxDuration    time.Duration
	AvgDuration    time.Duration
	RequestsPerSec float64
	Durations      []time.Duration
}

type RequestPayload struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	Stream    bool      `json:"stream"`
	MaxTokens int       `json:"max_tokens"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func main() {
	config := BenchmarkConfig{}
	flag.StringVar(&config.URL, "url", "http://localhost:54988/v1/messages", "API endpoint URL")
	flag.StringVar(&config.APIKey, "key", "", "API key for authentication")
	flag.IntVar(&config.Concurrency, "c", 10, "Number of concurrent requests")
	flag.IntVar(&config.Requests, "n", 100, "Total number of requests")
	flag.DurationVar(&config.Timeout, "t", 30*time.Second, "Request timeout")
	flag.Parse()

	if config.APIKey == "" {
		fmt.Println("❌ Error: API key is required. Use -key flag")
		return
	}

	fmt.Println("🚀 Starting Performance Benchmark")
	fmt.Println("=====================================")
	fmt.Printf("Target URL:     %s\n", config.URL)
	fmt.Printf("Concurrency:    %d\n", config.Concurrency)
	fmt.Printf("Total Requests: %d\n", config.Requests)
	fmt.Printf("Timeout:        %v\n", config.Timeout)
	fmt.Println("=====================================\n")

	result := runBenchmark(config)
	printResults(result)
}

func runBenchmark(config BenchmarkConfig) BenchmarkResult {
	var (
		successCount int64
		errorCount   int64
		wg           sync.WaitGroup
		durations    = make([]time.Duration, 0, config.Requests)
		durationsMu  sync.Mutex
	)

	// Create HTTP client with connection pooling
	client := &http.Client{
		Timeout: config.Timeout,
		Transport: &http.Transport{
			MaxIdleConns:        config.Concurrency * 2,
			MaxIdleConnsPerHost: config.Concurrency * 2,
			MaxConnsPerHost:     0,
			IdleConnTimeout:     90 * time.Second,
			DisableKeepAlives:   false,
		},
	}

	// Prepare request payload
	payload := RequestPayload{
		Model: "claude-sonnet-4-20250514",
		Messages: []Message{
			{
				Role:    "user",
				Content: "Hello! Please respond with 'Hi' only.",
			},
		},
		Stream:    false,
		MaxTokens: 50,
	}
	payloadBytes, _ := json.Marshal(payload)

	// Channel for controlling concurrency
	semaphore := make(chan struct{}, config.Concurrency)
	startTime := time.Now()

	fmt.Printf("⏳ Running benchmark...\n")
	progressInterval := config.Requests / 20
	if progressInterval < 1 {
		progressInterval = 1
	}

	for i := 0; i < config.Requests; i++ {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire semaphore

		go func(reqNum int) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore

			reqStart := time.Now()
			success := sendRequest(client, config.URL, config.APIKey, payloadBytes)
			reqDuration := time.Since(reqStart)

			durationsMu.Lock()
			durations = append(durations, reqDuration)
			durationsMu.Unlock()

			if success {
				atomic.AddInt64(&successCount, 1)
			} else {
				atomic.AddInt64(&errorCount, 1)
			}

			// Print progress
			if reqNum%progressInterval == 0 {
				completed := atomic.LoadInt64(&successCount) + atomic.LoadInt64(&errorCount)
				fmt.Printf("  Progress: %d/%d (%.1f%%)\n", completed, config.Requests, float64(completed)/float64(config.Requests)*100)
			}
		}(i)
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	// Calculate statistics
	var minDuration, maxDuration, totalDuration2 time.Duration
	minDuration = durations[0]

	for _, d := range durations {
		totalDuration2 += d
		if d < minDuration {
			minDuration = d
		}
		if d > maxDuration {
			maxDuration = d
		}
	}

	avgDuration := totalDuration2 / time.Duration(len(durations))
	requestsPerSec := float64(config.Requests) / totalDuration.Seconds()

	return BenchmarkResult{
		TotalRequests:  config.Requests,
		SuccessCount:   successCount,
		ErrorCount:     errorCount,
		TotalDuration:  totalDuration,
		MinDuration:    minDuration,
		MaxDuration:    maxDuration,
		AvgDuration:    avgDuration,
		RequestsPerSec: requestsPerSec,
		Durations:      durations,
	}
}

func sendRequest(client *http.Client, url, apiKey string, payload []byte) bool {
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return false
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Read and discard body
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode == http.StatusOK
}

func printResults(result BenchmarkResult) {
	fmt.Println("\n📊 Benchmark Results")
	fmt.Println("=====================================")
	fmt.Printf("Total Requests:    %d\n", result.TotalRequests)
	fmt.Printf("✅ Successful:      %d (%.1f%%)\n", result.SuccessCount, float64(result.SuccessCount)/float64(result.TotalRequests)*100)
	fmt.Printf("❌ Failed:          %d (%.1f%%)\n", result.ErrorCount, float64(result.ErrorCount)/float64(result.TotalRequests)*100)
	fmt.Println("-------------------------------------")
	fmt.Printf("Total Duration:    %v\n", result.TotalDuration)
	fmt.Printf("Requests/sec:      %.2f\n", result.RequestsPerSec)
	fmt.Println("-------------------------------------")
	fmt.Printf("Min Response Time: %v\n", result.MinDuration)
	fmt.Printf("Max Response Time: %v\n", result.MaxDuration)
	fmt.Printf("Avg Response Time: %v\n", result.AvgDuration)
	fmt.Println("-------------------------------------")

	// Calculate percentiles
	if len(result.Durations) > 0 {
		sorted := make([]time.Duration, len(result.Durations))
		copy(sorted, result.Durations)
		// Simple bubble sort for small datasets
		for i := 0; i < len(sorted); i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[i] > sorted[j] {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}

		p50 := sorted[len(sorted)*50/100]
		p90 := sorted[len(sorted)*90/100]
		p95 := sorted[len(sorted)*95/100]
		p99 := sorted[len(sorted)*99/100]

		fmt.Println("Response Time Percentiles:")
		fmt.Printf("  50th (median): %v\n", p50)
		fmt.Printf("  90th:          %v\n", p90)
		fmt.Printf("  95th:          %v\n", p95)
		fmt.Printf("  99th:          %v\n", p99)
	}

	fmt.Println("=====================================")

	// Performance evaluation
	fmt.Println("\n🎯 Performance Evaluation:")
	successRate := float64(result.SuccessCount) / float64(result.TotalRequests) * 100
	if successRate >= 99 {
		fmt.Println("✅ Success Rate: Excellent (≥99%)")
	} else if successRate >= 95 {
		fmt.Println("⚠️  Success Rate: Good (≥95%)")
	} else {
		fmt.Println("❌ Success Rate: Needs Improvement (<95%)")
	}

	if result.RequestsPerSec >= 50 {
		fmt.Println("✅ Throughput: Excellent (≥50 req/s)")
	} else if result.RequestsPerSec >= 20 {
		fmt.Println("⚠️  Throughput: Good (≥20 req/s)")
	} else {
		fmt.Println("❌ Throughput: Needs Improvement (<20 req/s)")
	}

	if result.AvgDuration <= 500*time.Millisecond {
		fmt.Println("✅ Latency: Excellent (≤500ms)")
	} else if result.AvgDuration <= 1*time.Second {
		fmt.Println("⚠️  Latency: Good (≤1s)")
	} else {
		fmt.Println("❌ Latency: Needs Improvement (>1s)")
	}
}
