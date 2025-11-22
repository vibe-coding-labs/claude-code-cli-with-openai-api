package database

import (
	"log"
	"sync"
)

// AsyncLogger provides non-blocking request logging
type AsyncLogger struct {
	logQueue chan *RequestLog
	wg       sync.WaitGroup
	workers  int
}

var (
	asyncLogger     *AsyncLogger
	asyncLoggerOnce sync.Once
)

// GetAsyncLogger returns the global async logger instance
func GetAsyncLogger() *AsyncLogger {
	asyncLoggerOnce.Do(func() {
		asyncLogger = &AsyncLogger{
			logQueue: make(chan *RequestLog, 1000), // Buffer up to 1000 logs
			workers:  5,                            // 5 concurrent workers
		}
		asyncLogger.Start()
	})
	return asyncLogger
}

// Start starts the async logger workers
func (al *AsyncLogger) Start() {
	for i := 0; i < al.workers; i++ {
		al.wg.Add(1)
		go al.worker()
	}
}

// worker processes log entries from the queue
func (al *AsyncLogger) worker() {
	defer al.wg.Done()
	for logEntry := range al.logQueue {
		if err := LogRequestSync(logEntry); err != nil {
			// Log errors but don't block
			log.Printf("Failed to log request: %v", err)
		}
	}
}

// LogAsync queues a log entry for async processing
func (al *AsyncLogger) LogAsync(logEntry *RequestLog) {
	select {
	case al.logQueue <- logEntry:
		// Successfully queued
	default:
		// Queue is full, log synchronously as fallback
		log.Printf("Warning: Log queue full, logging synchronously")
		if err := LogRequestSync(logEntry); err != nil {
			log.Printf("Failed to log request: %v", err)
		}
	}
}

// Shutdown gracefully shuts down the async logger
func (al *AsyncLogger) Shutdown() {
	close(al.logQueue)
	al.wg.Wait()
}

// LogRequestSync is the original synchronous logging function
func LogRequestSync(log *RequestLog) error {
	query := `
		INSERT INTO request_logs (
			config_id, model, input_tokens, output_tokens, total_tokens,
			duration_ms, status, error_message, request_body, response_body,
			request_summary, response_preview, client_ip, user_agent, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`

	_, err := DB.Exec(query,
		log.ConfigID, log.Model, log.InputTokens, log.OutputTokens, log.TotalTokens,
		log.DurationMs, log.Status, log.ErrorMessage, log.RequestBody, log.ResponseBody,
		log.RequestSummary, log.ResponsePreview, log.ClientIP, log.UserAgent,
	)

	if err != nil {
		return err
	}

	// Update aggregated statistics
	return updateTokenStats(log)
}

// LogRequestAsync queues a request log for async processing (non-blocking)
func LogRequestAsync(log *RequestLog) {
	GetAsyncLogger().LogAsync(log)
}
