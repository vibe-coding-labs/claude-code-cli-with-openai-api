package database

import (
	"log"
	"sync"
	"time"
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
		if err := LogRequestSync(logEntry, logEntry.SessionID); err != nil {
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
		if err := LogRequestSync(logEntry, logEntry.SessionID); err != nil {
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
func LogRequestSync(log *RequestLog, sessionID *string) error {
	log.SessionID = sessionID

	query := `
		INSERT INTO request_logs (
			config_id, user_id, session_id, model, input_tokens, output_tokens, total_tokens,
			duration_ms, status, error_message, request_body, response_body,
			request_body_path, response_body_path,
			request_summary, response_preview, client_ip, user_agent, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`

	result, err := DB.Exec(query,
		log.ConfigID, log.UserID, log.SessionID, log.Model, log.InputTokens, log.OutputTokens, log.TotalTokens,
		log.DurationMs, log.Status, log.ErrorMessage, log.RequestBody, log.ResponseBody,
		log.RequestBodyPath, log.ResponseBodyPath,
		log.RequestSummary, log.ResponsePreview, log.ClientIP, log.UserAgent,
	)

	if err != nil {
		return err
	}

	// Get the autoincrement ID and store bodies to files
	if shouldStoreBodyToFile() && (log.RequestBody != "" || log.ResponseBody != "") {
		id, _ := result.LastInsertId()
		storage := GetLogStorage()
		createdAt := log.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}

		// Store request body
		if log.RequestBody != "" {
			reqPath, err := storage.StoreBody(id, log.RequestBody, "req", createdAt)
			if err == nil && reqPath != "" {
				DB.Exec("UPDATE request_logs SET request_body = NULL, request_body_path = ? WHERE id = ?", reqPath, id)
			}
		}

		// Store response body
		if log.ResponseBody != "" {
			respPath, err := storage.StoreBody(id, log.ResponseBody, "resp", createdAt)
			if err == nil && respPath != "" {
				DB.Exec("UPDATE request_logs SET response_body = NULL, response_body_path = ? WHERE id = ?", respPath, id)
			}
		}
	}

	// Update aggregated statistics
	return updateTokenStats(log)
}

// LogRequestAsync queues a request log for async processing (non-blocking)
func LogRequestAsync(log *RequestLog, sessionID *string) {
	log.SessionID = sessionID
	GetAsyncLogger().LogAsync(log)
}
