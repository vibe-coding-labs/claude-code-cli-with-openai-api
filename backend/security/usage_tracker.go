package security

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// UsageTracker tracks API usage for tenants
type UsageTracker struct {
	db            *sql.DB
	recordChan    chan *database.UsageRecord
	batchSize     int
	flushInterval time.Duration
	wg            sync.WaitGroup
	stopChan      chan struct{}
}

// NewUsageTracker creates a new UsageTracker instance
func NewUsageTracker(db *sql.DB) *UsageTracker {
	ut := &UsageTracker{
		db:            db,
		recordChan:    make(chan *database.UsageRecord, 1000),
		batchSize:     100,
		flushInterval: 5 * time.Second,
		stopChan:      make(chan struct{}),
	}

	// Start background worker for batch processing
	ut.wg.Add(1)
	go ut.batchWorker()

	return ut
}

// RecordUsage records a usage event asynchronously
func (ut *UsageTracker) RecordUsage(ctx context.Context, record *database.UsageRecord) error {
	// Set ID and timestamp if not already set
	if record.ID == "" {
		record.ID = uuid.New().String()
	}
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now()
	}

	if err := record.Validate(); err != nil {
		return fmt.Errorf("invalid usage record: %w", err)
	}

	// Send to channel for async processing
	select {
	case ut.recordChan <- record:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Channel is full, log warning but don't block
		return fmt.Errorf("usage record channel is full")
	}
}

// GetUsage retrieves usage statistics for a tenant
func (ut *UsageTracker) GetUsage(ctx context.Context, tenantID string, period database.TimePeriod) (*database.UsageStats, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}

	query := `
		SELECT 
			COUNT(*) as total_requests,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(cost), 0) as total_cost,
			COALESCE(AVG(response_time), 0) as avg_response_time,
			COALESCE(SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 0) as error_rate
		FROM usage_records
		WHERE tenant_id = ? AND timestamp >= ? AND timestamp <= ?
	`

	stats := &database.UsageStats{
		TenantID:  tenantID,
		StartTime: period.Start,
		EndTime:   period.End,
	}

	err := ut.db.QueryRowContext(ctx, query, tenantID, period.Start, period.End).Scan(
		&stats.TotalRequests,
		&stats.TotalTokens,
		&stats.TotalCost,
		&stats.AvgResponseTime,
		&stats.ErrorRate,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get usage stats: %w", err)
	}

	// Determine period type
	duration := period.End.Sub(period.Start)
	if duration <= 24*time.Hour {
		stats.Period = "daily"
	} else if duration <= 31*24*time.Hour {
		stats.Period = "monthly"
	} else {
		stats.Period = "custom"
	}

	return stats, nil
}

// GetUsageHistory retrieves historical usage data for a tenant
func (ut *UsageTracker) GetUsageHistory(ctx context.Context, tenantID string, period database.TimePeriod, groupBy string) ([]database.UsageStats, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}

	// Validate groupBy parameter
	var timeFormat string
	switch groupBy {
	case "hour":
		timeFormat = "%Y-%m-%d %H:00:00"
	case "day":
		timeFormat = "%Y-%m-%d"
	case "month":
		timeFormat = "%Y-%m"
	default:
		return nil, fmt.Errorf("invalid groupBy parameter: %s (must be hour, day, or month)", groupBy)
	}

	query := fmt.Sprintf(`
		SELECT 
			strftime('%s', timestamp) as period,
			COUNT(*) as total_requests,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(cost), 0) as total_cost,
			COALESCE(AVG(response_time), 0) as avg_response_time,
			COALESCE(SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 0) as error_rate,
			MIN(timestamp) as start_time,
			MAX(timestamp) as end_time
		FROM usage_records
		WHERE tenant_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY strftime('%s', timestamp)
		ORDER BY period ASC
	`, timeFormat, timeFormat)

	rows, err := ut.db.QueryContext(ctx, query, tenantID, period.Start, period.End)
	if err != nil {
		return nil, fmt.Errorf("failed to query usage history: %w", err)
	}
	defer rows.Close()

	var history []database.UsageStats
	for rows.Next() {
		var stats database.UsageStats
		var periodStr string
		err := rows.Scan(
			&periodStr,
			&stats.TotalRequests,
			&stats.TotalTokens,
			&stats.TotalCost,
			&stats.AvgResponseTime,
			&stats.ErrorRate,
			&stats.StartTime,
			&stats.EndTime,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan usage stats: %w", err)
		}

		stats.TenantID = tenantID
		stats.Period = periodStr
		history = append(history, stats)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating usage history: %w", err)
	}

	return history, nil
}

// batchWorker processes usage records in batches
func (ut *UsageTracker) batchWorker() {
	defer ut.wg.Done()

	batch := make([]*database.UsageRecord, 0, ut.batchSize)
	ticker := time.NewTicker(ut.flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}

		ctx := context.Background()
		if err := ut.insertBatch(ctx, batch); err != nil {
			// Log error but continue processing
			fmt.Printf("Error inserting usage batch: %v\n", err)
		}

		// Clear batch
		batch = batch[:0]
	}

	for {
		select {
		case record := <-ut.recordChan:
			batch = append(batch, record)
			if len(batch) >= ut.batchSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-ut.stopChan:
			// Flush remaining records before stopping
			flush()
			return
		}
	}
}

// insertBatch inserts a batch of usage records into the database
func (ut *UsageTracker) insertBatch(ctx context.Context, records []*database.UsageRecord) error {
	if len(records) == 0 {
		return nil
	}

	tx, err := ut.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO usage_records (
			id, tenant_id, api_key_id, model, 
			prompt_tokens, completion_tokens, total_tokens, 
			cost, response_time, status_code, timestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, record := range records {
		_, err := stmt.ExecContext(ctx,
			record.ID,
			record.TenantID,
			record.APIKeyID,
			record.Model,
			record.PromptTokens,
			record.CompletionTokens,
			record.TotalTokens,
			record.Cost,
			record.ResponseTime,
			record.StatusCode,
			record.Timestamp,
		)
		if err != nil {
			return fmt.Errorf("failed to insert usage record: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetUsageByAPIKey retrieves usage statistics grouped by API key
func (ut *UsageTracker) GetUsageByAPIKey(ctx context.Context, tenantID string, period database.TimePeriod) (map[string]*database.UsageStats, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}

	query := `
		SELECT 
			api_key_id,
			COUNT(*) as total_requests,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(cost), 0) as total_cost,
			COALESCE(AVG(response_time), 0) as avg_response_time,
			COALESCE(SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 0) as error_rate
		FROM usage_records
		WHERE tenant_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY api_key_id
	`

	rows, err := ut.db.QueryContext(ctx, query, tenantID, period.Start, period.End)
	if err != nil {
		return nil, fmt.Errorf("failed to query usage by API key: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*database.UsageStats)
	for rows.Next() {
		var apiKeyID string
		stats := &database.UsageStats{
			TenantID:  tenantID,
			StartTime: period.Start,
			EndTime:   period.End,
		}

		err := rows.Scan(
			&apiKeyID,
			&stats.TotalRequests,
			&stats.TotalTokens,
			&stats.TotalCost,
			&stats.AvgResponseTime,
			&stats.ErrorRate,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan usage stats: %w", err)
		}

		result[apiKeyID] = stats
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating usage by API key: %w", err)
	}

	return result, nil
}

// GetUsageByModel retrieves usage statistics grouped by model
func (ut *UsageTracker) GetUsageByModel(ctx context.Context, tenantID string, period database.TimePeriod) (map[string]*database.UsageStats, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}

	query := `
		SELECT 
			model,
			COUNT(*) as total_requests,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(cost), 0) as total_cost,
			COALESCE(AVG(response_time), 0) as avg_response_time,
			COALESCE(SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 0) as error_rate
		FROM usage_records
		WHERE tenant_id = ? AND timestamp >= ? AND timestamp <= ?
		GROUP BY model
	`

	rows, err := ut.db.QueryContext(ctx, query, tenantID, period.Start, period.End)
	if err != nil {
		return nil, fmt.Errorf("failed to query usage by model: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*database.UsageStats)
	for rows.Next() {
		var model string
		stats := &database.UsageStats{
			TenantID:  tenantID,
			StartTime: period.Start,
			EndTime:   period.End,
		}

		err := rows.Scan(
			&model,
			&stats.TotalRequests,
			&stats.TotalTokens,
			&stats.TotalCost,
			&stats.AvgResponseTime,
			&stats.ErrorRate,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan usage stats: %w", err)
		}

		result[model] = stats
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating usage by model: %w", err)
	}

	return result, nil
}

// Close gracefully shuts down the usage tracker
func (ut *UsageTracker) Close() error {
	close(ut.stopChan)
	ut.wg.Wait()
	return nil
}
