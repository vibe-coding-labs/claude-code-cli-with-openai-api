package database

import (
	"database/sql"
	"fmt"
	stdlog "log"
	"strings"
	"sync"
	"time"
)

// SessionInterruption represents a typed, persisted session-interruption event.
// It complements proxy_errors (which only records upstream/protocol errors) by
// additionally covering *client-initiated* disconnects/cancels and upstream stalls
// that terminate a session but never produce an HTTP error.
type SessionInterruption struct {
	ID                int64  `json:"id"`
	SessionID         string `json:"session_id,omitempty"`
	RequestID         string `json:"request_id,omitempty"`
	ConfigID          string `json:"config_id,omitempty"`
	ConfigName        string `json:"config_name,omitempty"`
	Model             string `json:"model,omitempty"`
	UserID            int64  `json:"user_id,omitempty"`
	ClientIP          string `json:"client_ip,omitempty"`
	InterruptionCause string `json:"interruption_cause"`
	Dimension         string `json:"dimension"` // 'subjective' | 'infrastructure'
	Stage             string `json:"stage"`
	Detail            string `json:"detail,omitempty"`
	DurationMs        int64  `json:"duration_ms,omitempty"`
	CreatedAt         string `json:"created_at"`
}

// Interruption dimensions
const (
	DimensionSubjective     = "subjective"     // 用户主动停：不上告警、不触发重启
	DimensionInfrastructure = "infrastructure" // 代理/上游故障：参与告警与自动自愈
)

// Detail truncation cap
const maxInterruptionDetailLen = 500

// InterruptionWriter is a dedicated async writer for session_interruptions.
// It mirrors the AsyncLogger channel+worker pattern (backend/database/async_logger.go)
// so the SSE hot path never blocks on a DB insert.
type InterruptionWriter struct {
	queue   chan *SessionInterruption
	wg      sync.WaitGroup
	workers int
}

var (
	interruptionWriter     *InterruptionWriter
	interruptionWriterOnce sync.Once
)

// GetInterruptionWriter returns the global async writer (lazy singleton).
func GetInterruptionWriter() *InterruptionWriter {
	interruptionWriterOnce.Do(func() {
		interruptionWriter = &InterruptionWriter{
			queue:   make(chan *SessionInterruption, 1000),
			workers: 2,
		}
		interruptionWriter.Start()
	})
	return interruptionWriter
}

// Start launches the writer workers.
func (w *InterruptionWriter) Start() {
	for i := 0; i < w.workers; i++ {
		w.wg.Add(1)
		go w.worker()
	}
}

// worker drains the queue and inserts rows.
func (w *InterruptionWriter) worker() {
	defer w.wg.Done()
	for e := range w.queue {
		if err := LogInterruptionSync(e); err != nil {
			// Don't block; the event is already lost, just log it.
			stdlog.Printf("Failed to log session interruption: %v", err)
		}
	}
}

// LogInterruptionAsync queues an interruption event for async persistence.
// Falls back to a synchronous insert when the queue is full.
func (w *InterruptionWriter) LogInterruptionAsync(e *SessionInterruption) {
	select {
	case w.queue <- e:
		// queued
	default:
		stdlog.Printf("Warning: interruption queue full, logging synchronously")
		if err := LogInterruptionSync(e); err != nil {
			stdlog.Printf("Failed to log session interruption: %v", err)
		}
	}
}

// Shutdown drains the queue gracefully.
func (w *InterruptionWriter) Shutdown() {
	close(w.queue)
	w.wg.Wait()
}

// LogInterruptionSync inserts a single interruption row (used by worker + fallback).
func LogInterruptionSync(e *SessionInterruption) error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}
	if len(e.Detail) > maxInterruptionDetailLen {
		e.Detail = e.Detail[:maxInterruptionDetailLen]
	}
	_, err := DB.Exec(`INSERT INTO session_interruptions
		(session_id, request_id, config_id, config_name, model, user_id, client_ip,
		 interruption_cause, dimension, stage, detail, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nullableStr(e.SessionID), nullableStr(e.RequestID), nullableStr(e.ConfigID),
		nullableStr(e.ConfigName), nullableStr(e.Model), e.UserID, nullableStr(e.ClientIP),
		e.InterruptionCause, e.Dimension, e.Stage, nullableStr(e.Detail), e.DurationMs,
		time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// LogInterruptionAsync queues an interruption event via the global writer.
func LogInterruptionAsync(e *SessionInterruption) {
	GetInterruptionWriter().LogInterruptionAsync(e)
}

// nullableStr returns a NULL-able interface for a possibly-empty string.
func nullableStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// InterruptionStats is an aggregated count for a cause/dimension/config combo.
type InterruptionStats struct {
	ConfigID          string `json:"config_id,omitempty"`
	ConfigName        string `json:"config_name,omitempty"`
	InterruptionCause string `json:"interruption_cause"`
	Dimension         string `json:"dimension"`
	Count             int    `json:"count"`
}

// GetInterruptionStats returns interruption counts grouped by dimension and cause
// (and optionally per config) within a time window. It is the query the monitoring
// script and the REST endpoint both consume.
//
// `configID` empty => aggregate across all configs. `byConfig` true => also group by
// config_id/config_name (per-config threshold signal).
func GetInterruptionStats(since time.Time, configID string, byConfig bool) ([]InterruptionStats, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	groupBy := "interruption_cause, dimension"
	sel := "interruption_cause, dimension"
	if byConfig {
		groupBy = "COALESCE(config_id,''), COALESCE(config_name,''), interruption_cause, dimension"
		sel = "COALESCE(config_id,''), COALESCE(config_name,''), interruption_cause, dimension"
	}

	query := `SELECT ` + sel + `, COUNT(*) FROM session_interruptions WHERE created_at >= ?`
	args := []interface{}{since.UTC().Format(time.RFC3339)}
	if configID != "" {
		query += " AND config_id = ?"
		args = append(args, configID)
	}
	query += " GROUP BY " + groupBy + " ORDER BY COUNT(*) DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query interruption stats: %w", err)
	}
	defer rows.Close()

	var stats []InterruptionStats
	for rows.Next() {
		var s InterruptionStats
		var cnt int
		if byConfig {
			if err := rows.Scan(&s.ConfigID, &s.ConfigName, &s.InterruptionCause, &s.Dimension, &cnt); err != nil {
				continue
			}
		} else {
			if err := rows.Scan(&s.InterruptionCause, &s.Dimension, &cnt); err != nil {
				continue
			}
		}
		s.Count = cnt
		stats = append(stats, s)
	}
	return stats, nil
}

// GetRecentInterruptions returns the most recent interruption rows, optionally
// filtered by config and dimension. Backs GET /api/monitor/interruptions.
func GetRecentInterruptions(limit int, configID, dimension string) ([]SessionInterruption, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var conds []string
	var args []interface{}
	if configID != "" {
		conds = append(conds, "config_id = ?")
		args = append(args, configID)
	}
	if dimension != "" {
		conds = append(conds, "dimension = ?")
		args = append(args, dimension)
	}

	query := `SELECT id, COALESCE(session_id,''), COALESCE(request_id,''), COALESCE(config_id,''),
		COALESCE(config_name,''), COALESCE(model,''), COALESCE(user_id,0), COALESCE(client_ip,''),
		interruption_cause, dimension, stage, COALESCE(detail,''), COALESCE(duration_ms,0), created_at
		FROM session_interruptions`
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent interruptions: %w", err)
	}
	defer rows.Close()

	var items []SessionInterruption
	for rows.Next() {
		var e SessionInterruption
		var createdAt string
		if err := rows.Scan(&e.ID, &e.SessionID, &e.RequestID, &e.ConfigID, &e.ConfigName,
			&e.Model, &e.UserID, &e.ClientIP, &e.InterruptionCause, &e.Dimension, &e.Stage,
			&e.Detail, &e.DurationMs, &createdAt); err != nil {
			continue
		}
		e.CreatedAt = createdAt
		items = append(items, e)
	}
	return items, nil
}

// FlappingSession describes a session interrupted multiple times in a window.
type FlappingSession struct {
	SessionID  string `json:"session_id"`
	ConfigID   string `json:"config_id,omitempty"`
	ConfigName string `json:"config_name,omitempty"`
	Count      int    `json:"count"`
	LastAt     string `json:"last_at,omitempty"`
}

// GetFlappingSessions returns sessions interrupted >= threshold times within a window.
// This targets a single misbehaving session (e.g. a client stuck retrying a bad request)
// without paging on the global aggregate. Any cause counts (subjective included).
func GetFlappingSessions(since time.Time, threshold int) ([]FlappingSession, error) {
	if DB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT COALESCE(session_id,''), COALESCE(config_id,''), COALESCE(config_name,''),
		COUNT(*), MAX(created_at) FROM session_interruptions
		WHERE session_id IS NOT NULL AND session_id != '' AND created_at >= ?
		GROUP BY session_id HAVING COUNT(*) >= ? ORDER BY COUNT(*) DESC`

	rows, err := DB.Query(query, since.UTC().Format(time.RFC3339), threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to query flapping sessions: %w", err)
	}
	defer rows.Close()

	var sessions []FlappingSession
	for rows.Next() {
		var s FlappingSession
		var lastAt string
		if err := rows.Scan(&s.SessionID, &s.ConfigID, &s.ConfigName, &s.Count, &lastAt); err != nil {
			continue
		}
		s.LastAt = lastAt
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// GetInterruptionCount returns the raw count of interruptions matching a dimension
// within a window (optionally scoped to a config). Used by the ratio signal.
func GetInterruptionCount(since time.Time, dimension, configID string) (int, error) {
	if DB == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	var count int
	query := "SELECT COUNT(*) FROM session_interruptions WHERE created_at >= ? AND dimension = ?"
	args := []interface{}{since.UTC().Format(time.RFC3339), dimension}
	if configID != "" {
		query += " AND config_id = ?"
		args = append(args, configID)
	}
	err := DB.QueryRow(query, args...).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return count, err
}

// CleanupOldSessionInterruptions removes interruption rows older than the duration.
func CleanupOldSessionInterruptions(olderThan time.Duration) (int64, error) {
	if DB == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	result, err := DB.Exec("DELETE FROM session_interruptions WHERE created_at < ?", cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup session interruptions: %w", err)
	}
	return result.RowsAffected()
}
