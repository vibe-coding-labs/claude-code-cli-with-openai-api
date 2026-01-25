package security

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// AuditEvent represents a security event to be logged
type AuditEvent struct {
	ID        string    `json:"id" db:"id"`
	TenantID  string    `json:"tenant_id" db:"tenant_id"`
	EventType string    `json:"event_type" db:"event_type"`
	Actor     string    `json:"actor" db:"actor"`         // User or system component
	Resource  string    `json:"resource" db:"resource"`   // What was affected
	Action    string    `json:"action" db:"action"`       // What happened
	Result    string    `json:"result" db:"result"`       // success, failure
	Details   string    `json:"details" db:"details"`     // JSON blob
	IPAddress string    `json:"ip_address" db:"ip_address"`
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
}

// AuditFilters defines filters for querying audit events
type AuditFilters struct {
	TenantID  string
	EventType string
	Actor     string
	Result    string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
	Offset    int
}

// AuditLogger interface for recording security events
type AuditLogger interface {
	LogEvent(ctx context.Context, event *AuditEvent) error
	QueryEvents(ctx context.Context, filters AuditFilters) ([]*AuditEvent, error)
	PurgeOldEvents(ctx context.Context, before time.Time) error
	Start(ctx context.Context) error
	Stop() error
}

// auditLogger implements AuditLogger interface
type auditLogger struct {
	db          *sql.DB
	eventChan   chan *AuditEvent
	batchSize   int
	flushPeriod time.Duration
	wg          sync.WaitGroup
	stopChan    chan struct{}
	mu          sync.RWMutex
	stopped     bool
}

// AuditLoggerConfig holds configuration for audit logger
type AuditLoggerConfig struct {
	DB          *sql.DB
	BufferSize  int           // Size of event buffer channel
	BatchSize   int           // Number of events to batch before writing
	FlushPeriod time.Duration // Maximum time to wait before flushing batch
}

// NewAuditLogger creates a new audit logger instance
func NewAuditLogger(config AuditLoggerConfig) AuditLogger {
	if config.BufferSize == 0 {
		config.BufferSize = 1000
	}
	if config.BatchSize == 0 {
		config.BatchSize = 100
	}
	if config.FlushPeriod == 0 {
		config.FlushPeriod = 5 * time.Second
	}

	return &auditLogger{
		db:          config.DB,
		eventChan:   make(chan *AuditEvent, config.BufferSize),
		batchSize:   config.BatchSize,
		flushPeriod: config.FlushPeriod,
		stopChan:    make(chan struct{}),
	}
}

// Start begins the audit logger background worker
func (a *auditLogger) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stopped {
		return fmt.Errorf("audit logger already stopped")
	}

	a.wg.Add(1)
	go a.worker(ctx)

	return nil
}

// Stop gracefully stops the audit logger
func (a *auditLogger) Stop() error {
	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		return nil
	}
	a.stopped = true
	a.mu.Unlock()

	close(a.stopChan)
	close(a.eventChan)
	a.wg.Wait()

	return nil
}

// LogEvent logs a security event asynchronously
func (a *auditLogger) LogEvent(ctx context.Context, event *AuditEvent) error {
	a.mu.RLock()
	stopped := a.stopped
	a.mu.RUnlock()

	if stopped {
		return fmt.Errorf("audit logger is stopped")
	}

	// Generate ID if not provided
	if event.ID == "" {
		event.ID = uuid.New().String()
	}

	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Try to send event to channel (non-blocking)
	select {
	case a.eventChan <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Channel is full, log synchronously as fallback
		return a.writeEvent(event)
	}
}

// QueryEvents retrieves audit events based on filters
func (a *auditLogger) QueryEvents(ctx context.Context, filters AuditFilters) ([]*AuditEvent, error) {
	query := `
		SELECT id, tenant_id, event_type, actor, resource, action, result, details, ip_address, timestamp
		FROM audit_logs
		WHERE 1=1
	`
	args := []interface{}{}
	argPos := 1

	if filters.TenantID != "" {
		query += fmt.Sprintf(" AND tenant_id = $%d", argPos)
		args = append(args, filters.TenantID)
		argPos++
	}

	if filters.EventType != "" {
		query += fmt.Sprintf(" AND event_type = $%d", argPos)
		args = append(args, filters.EventType)
		argPos++
	}

	if filters.Actor != "" {
		query += fmt.Sprintf(" AND actor = $%d", argPos)
		args = append(args, filters.Actor)
		argPos++
	}

	if filters.Result != "" {
		query += fmt.Sprintf(" AND result = $%d", argPos)
		args = append(args, filters.Result)
		argPos++
	}

	if !filters.StartTime.IsZero() {
		query += fmt.Sprintf(" AND timestamp >= $%d", argPos)
		args = append(args, filters.StartTime)
		argPos++
	}

	if !filters.EndTime.IsZero() {
		query += fmt.Sprintf(" AND timestamp <= $%d", argPos)
		args = append(args, filters.EndTime)
		argPos++
	}

	query += " ORDER BY timestamp DESC"

	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argPos)
		args = append(args, filters.Limit)
		argPos++
	}

	if filters.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argPos)
		args = append(args, filters.Offset)
		argPos++
	}

	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit events: %w", err)
	}
	defer rows.Close()

	var events []*AuditEvent
	for rows.Next() {
		event := &AuditEvent{}
		err := rows.Scan(
			&event.ID,
			&event.TenantID,
			&event.EventType,
			&event.Actor,
			&event.Resource,
			&event.Action,
			&event.Result,
			&event.Details,
			&event.IPAddress,
			&event.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit event: %w", err)
		}
		events = append(events, event)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit events: %w", err)
	}

	return events, nil
}

// PurgeOldEvents deletes audit events older than the specified time
func (a *auditLogger) PurgeOldEvents(ctx context.Context, before time.Time) error {
	query := `DELETE FROM audit_logs WHERE timestamp < $1`
	result, err := a.db.ExecContext(ctx, query, before)
	if err != nil {
		return fmt.Errorf("failed to purge old audit events: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	// Log the purge operation
	purgeEvent := &AuditEvent{
		ID:        uuid.New().String(),
		TenantID:  "system",
		EventType: "audit_purge",
		Actor:     "system",
		Resource:  "audit_logs",
		Action:    "purge",
		Result:    "success",
		Details:   fmt.Sprintf(`{"rows_deleted": %d, "before": "%s"}`, rowsAffected, before.Format(time.RFC3339)),
		Timestamp: time.Now(),
	}

	// Write purge event synchronously to avoid recursion
	return a.writeEvent(purgeEvent)
}

// worker processes events from the channel in batches
func (a *auditLogger) worker(ctx context.Context) {
	defer a.wg.Done()

	batch := make([]*AuditEvent, 0, a.batchSize)
	ticker := time.NewTicker(a.flushPeriod)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}

		if err := a.writeBatch(batch); err != nil {
			// Log error but continue processing
			fmt.Printf("Error writing audit log batch: %v\n", err)
		}

		// Clear batch
		batch = batch[:0]
	}

	for {
		select {
		case event, ok := <-a.eventChan:
			if !ok {
				// Channel closed, flush remaining events and exit
				flush()
				return
			}

			batch = append(batch, event)

			// Flush if batch is full
			if len(batch) >= a.batchSize {
				flush()
			}

		case <-ticker.C:
			// Periodic flush
			flush()

		case <-a.stopChan:
			// Stop signal received, flush and exit
			flush()
			return

		case <-ctx.Done():
			// Context cancelled, flush and exit
			flush()
			return
		}
	}
}

// writeBatch writes a batch of events to the database
func (a *auditLogger) writeBatch(events []*AuditEvent) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := a.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO audit_logs (id, tenant_id, event_type, actor, resource, action, result, details, ip_address, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, event := range events {
		_, err := stmt.Exec(
			event.ID,
			event.TenantID,
			event.EventType,
			event.Actor,
			event.Resource,
			event.Action,
			event.Result,
			event.Details,
			event.IPAddress,
			event.Timestamp,
		)
		if err != nil {
			return fmt.Errorf("failed to insert audit event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// writeEvent writes a single event synchronously (fallback)
func (a *auditLogger) writeEvent(event *AuditEvent) error {
	query := `
		INSERT INTO audit_logs (id, tenant_id, event_type, actor, resource, action, result, details, ip_address, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := a.db.Exec(
		query,
		event.ID,
		event.TenantID,
		event.EventType,
		event.Actor,
		event.Resource,
		event.Action,
		event.Result,
		event.Details,
		event.IPAddress,
		event.Timestamp,
	)

	if err != nil {
		return fmt.Errorf("failed to write audit event: %w", err)
	}

	return nil
}

// Helper function to create audit event with details
func NewAuditEvent(tenantID, eventType, actor, resource, action, result string, details interface{}, ipAddress string) *AuditEvent {
	var detailsJSON string
	if details != nil {
		if detailsBytes, err := json.Marshal(details); err == nil {
			detailsJSON = string(detailsBytes)
		}
	}

	return &AuditEvent{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		EventType: eventType,
		Actor:     actor,
		Resource:  resource,
		Action:    action,
		Result:    result,
		Details:   detailsJSON,
		IPAddress: ipAddress,
		Timestamp: time.Now(),
	}
}
