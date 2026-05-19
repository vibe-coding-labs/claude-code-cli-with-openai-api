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

// QuotaManager manages tenant quotas and usage tracking
type QuotaManager struct {
	db    *sql.DB
	cache *quotaCache
	mu    sync.RWMutex
}

// quotaCache stores quota data in memory for fast access
type quotaCache struct {
	quotas map[string]*database.Quota // key: tenantID:quotaType:period
	mu     sync.RWMutex
}

// NewQuotaManager creates a new QuotaManager instance
func NewQuotaManager(db *sql.DB) *QuotaManager {
	qm := &QuotaManager{
		db: db,
		cache: &quotaCache{
			quotas: make(map[string]*database.Quota),
		},
	}
	
	// Start background job to reset quotas periodically
	go qm.quotaResetWorker()
	
	return qm
}

// CheckQuota checks if a tenant has sufficient quota for the requested usage
// Returns: allowed (bool), remaining quota, error
func (qm *QuotaManager) CheckQuota(ctx context.Context, tenantID string, usage database.Usage) (bool, *database.Quota, error) {
	if tenantID == "" {
		return false, nil, fmt.Errorf("tenant ID is required")
	}

	// Get all quotas for the tenant
	quotas, err := qm.getQuotasForTenant(ctx, tenantID)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get quotas: %w", err)
	}

	// If no quotas are set, allow the request
	if len(quotas) == 0 {
		return true, nil, nil
	}

	// Check each quota type
	for _, quota := range quotas {
		// Check if quota needs reset
		if time.Now().After(quota.ResetAt) {
			if err := qm.resetQuotaInternal(ctx, quota); err != nil {
				return false, nil, fmt.Errorf("failed to reset quota: %w", err)
			}
		}

		// Check if usage would exceed quota
		var wouldExceed bool
		switch database.QuotaType(quota.QuotaType) {
		case database.QuotaTypeRequests:
			wouldExceed = quota.CurrentUsage+usage.Requests > quota.Limit
		case database.QuotaTypeTokens:
			wouldExceed = quota.CurrentUsage+usage.Tokens > quota.Limit
		case database.QuotaTypeCost:
			// Convert cost to int64 (cents)
			costCents := int64(usage.Cost * 100)
			wouldExceed = quota.CurrentUsage+costCents > quota.Limit
		}

		if wouldExceed {
			return false, quota, nil
		}
	}

	return true, nil, nil
}

// IncrementUsage increments the usage counters for a tenant
func (qm *QuotaManager) IncrementUsage(ctx context.Context, tenantID string, usage database.Usage) error {
	if tenantID == "" {
		return fmt.Errorf("tenant ID is required")
	}

	// Get all quotas for the tenant
	quotas, err := qm.getQuotasForTenant(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to get quotas: %w", err)
	}

	// Increment each quota type
	for _, quota := range quotas {
		var increment int64
		switch database.QuotaType(quota.QuotaType) {
		case database.QuotaTypeRequests:
			increment = usage.Requests
		case database.QuotaTypeTokens:
			increment = usage.Tokens
		case database.QuotaTypeCost:
			// Convert cost to int64 (cents)
			increment = int64(usage.Cost * 100)
		}

		if increment > 0 {
			if err := qm.incrementQuotaUsage(ctx, quota.ID, increment); err != nil {
				return fmt.Errorf("failed to increment quota usage: %w", err)
			}
		}
	}

	return nil
}

// GetQuota retrieves quota information for a tenant
func (qm *QuotaManager) GetQuota(ctx context.Context, tenantID string) ([]*database.Quota, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}

	return qm.getQuotasForTenant(ctx, tenantID)
}

// SetQuota creates or updates a quota for a tenant
func (qm *QuotaManager) SetQuota(ctx context.Context, quota *database.Quota) error {
	// Set ID if not provided (for new quotas)
	if quota.ID == "" {
		quota.ID = uuid.New().String()
	}

	if err := quota.Validate(); err != nil {
		return fmt.Errorf("invalid quota: %w", err)
	}

	// Check if quota already exists
	existing, err := qm.getQuotaByTypeAndPeriod(ctx, quota.TenantID, quota.QuotaType, quota.Period)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check existing quota: %w", err)
	}

	if existing != nil {
		// Update existing quota
		quota.ID = existing.ID
		quota.CurrentUsage = existing.CurrentUsage
		quota.UpdatedAt = time.Now()
		return qm.updateQuota(ctx, quota)
	}

	// Create new quota
	quota.CurrentUsage = 0
	quota.ResetAt = calculateNextReset(quota.Period)
	quota.UpdatedAt = time.Now()

	return qm.createQuota(ctx, quota)
}

// ResetQuota resets the usage counter for a specific quota
func (qm *QuotaManager) ResetQuota(ctx context.Context, tenantID string, quotaType database.QuotaType) error {
	if tenantID == "" {
		return fmt.Errorf("tenant ID is required")
	}

	quotas, err := qm.getQuotasForTenant(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("failed to get quotas: %w", err)
	}

	for _, quota := range quotas {
		if database.QuotaType(quota.QuotaType) == quotaType {
			if err := qm.resetQuotaInternal(ctx, quota); err != nil {
				return fmt.Errorf("failed to reset quota: %w", err)
			}
		}
	}

	return nil
}

// getQuotasForTenant retrieves all quotas for a tenant (with caching)
func (qm *QuotaManager) getQuotasForTenant(ctx context.Context, tenantID string) ([]*database.Quota, error) {
	// Try cache first
	qm.cache.mu.RLock()
	var cachedQuotas []*database.Quota
	for _, quota := range qm.cache.quotas {
		if quota.TenantID == tenantID {
			cachedQuotas = append(cachedQuotas, quota)
		}
	}
	qm.cache.mu.RUnlock()

	if len(cachedQuotas) > 0 {
		return cachedQuotas, nil
	}

	// Query database
	query := `
		SELECT id, tenant_id, quota_type, period, [limit], current_usage, reset_at, updated_at
		FROM quotas
		WHERE tenant_id = ?
	`

	rows, err := qm.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to query quotas: %w", err)
	}
	defer rows.Close()

	var quotas []*database.Quota
	for rows.Next() {
		quota := &database.Quota{}
		err := rows.Scan(
			&quota.ID,
			&quota.TenantID,
			&quota.QuotaType,
			&quota.Period,
			&quota.Limit,
			&quota.CurrentUsage,
			&quota.ResetAt,
			&quota.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan quota: %w", err)
		}
		quotas = append(quotas, quota)

		// Update cache
		cacheKey := fmt.Sprintf("%s:%s:%s", quota.TenantID, quota.QuotaType, quota.Period)
		qm.cache.mu.Lock()
		qm.cache.quotas[cacheKey] = quota
		qm.cache.mu.Unlock()
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating quotas: %w", err)
	}

	return quotas, nil
}

// getQuotaByTypeAndPeriod retrieves a specific quota by type and period
func (qm *QuotaManager) getQuotaByTypeAndPeriod(ctx context.Context, tenantID, quotaType, period string) (*database.Quota, error) {
	// Try cache first
	cacheKey := fmt.Sprintf("%s:%s:%s", tenantID, quotaType, period)
	qm.cache.mu.RLock()
	if cached, ok := qm.cache.quotas[cacheKey]; ok {
		qm.cache.mu.RUnlock()
		return cached, nil
	}
	qm.cache.mu.RUnlock()

	// Query database
	query := `
		SELECT id, tenant_id, quota_type, period, [limit], current_usage, reset_at, updated_at
		FROM quotas
		WHERE tenant_id = ? AND quota_type = ? AND period = ?
	`

	quota := &database.Quota{}
	err := qm.db.QueryRowContext(ctx, query, tenantID, quotaType, period).Scan(
		&quota.ID,
		&quota.TenantID,
		&quota.QuotaType,
		&quota.Period,
		&quota.Limit,
		&quota.CurrentUsage,
		&quota.ResetAt,
		&quota.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	// Update cache
	qm.cache.mu.Lock()
	qm.cache.quotas[cacheKey] = quota
	qm.cache.mu.Unlock()

	return quota, nil
}

// createQuota creates a new quota in the database
func (qm *QuotaManager) createQuota(ctx context.Context, quota *database.Quota) error {
	query := `
		INSERT INTO quotas (id, tenant_id, quota_type, period, [limit], current_usage, reset_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := qm.db.ExecContext(ctx, query,
		quota.ID,
		quota.TenantID,
		quota.QuotaType,
		quota.Period,
		quota.Limit,
		quota.CurrentUsage,
		quota.ResetAt,
		quota.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create quota: %w", err)
	}

	// Update cache
	cacheKey := fmt.Sprintf("%s:%s:%s", quota.TenantID, quota.QuotaType, quota.Period)
	qm.cache.mu.Lock()
	qm.cache.quotas[cacheKey] = quota
	qm.cache.mu.Unlock()

	return nil
}

// updateQuota updates an existing quota in the database
func (qm *QuotaManager) updateQuota(ctx context.Context, quota *database.Quota) error {
	query := `
		UPDATE quotas
		SET [limit] = ?, reset_at = ?, updated_at = ?
		WHERE id = ?
	`

	_, err := qm.db.ExecContext(ctx, query,
		quota.Limit,
		quota.ResetAt,
		quota.UpdatedAt,
		quota.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update quota: %w", err)
	}

	// Update cache
	cacheKey := fmt.Sprintf("%s:%s:%s", quota.TenantID, quota.QuotaType, quota.Period)
	qm.cache.mu.Lock()
	qm.cache.quotas[cacheKey] = quota
	qm.cache.mu.Unlock()

	return nil
}

// incrementQuotaUsage atomically increments the usage counter
func (qm *QuotaManager) incrementQuotaUsage(ctx context.Context, quotaID string, increment int64) error {
	query := `
		UPDATE quotas
		SET current_usage = current_usage + ?, updated_at = ?
		WHERE id = ?
	`

	_, err := qm.db.ExecContext(ctx, query, increment, time.Now(), quotaID)
	if err != nil {
		return fmt.Errorf("failed to increment usage: %w", err)
	}

	// Invalidate cache for this quota
	qm.cache.mu.Lock()
	for key, quota := range qm.cache.quotas {
		if quota.ID == quotaID {
			delete(qm.cache.quotas, key)
			break
		}
	}
	qm.cache.mu.Unlock()

	return nil
}

// resetQuotaInternal resets a quota's usage counter
func (qm *QuotaManager) resetQuotaInternal(ctx context.Context, quota *database.Quota) error {
	query := `
		UPDATE quotas
		SET current_usage = 0, reset_at = ?, updated_at = ?
		WHERE id = ?
	`

	nextReset := calculateNextReset(quota.Period)
	_, err := qm.db.ExecContext(ctx, query, nextReset, time.Now(), quota.ID)
	if err != nil {
		return fmt.Errorf("failed to reset quota: %w", err)
	}

	// Update cache
	quota.CurrentUsage = 0
	quota.ResetAt = nextReset
	quota.UpdatedAt = time.Now()

	cacheKey := fmt.Sprintf("%s:%s:%s", quota.TenantID, quota.QuotaType, quota.Period)
	qm.cache.mu.Lock()
	qm.cache.quotas[cacheKey] = quota
	qm.cache.mu.Unlock()

	return nil
}

// quotaResetWorker runs periodically to reset expired quotas
func (qm *QuotaManager) quotaResetWorker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()
		
		// Find quotas that need reset
		query := `
			SELECT id, tenant_id, quota_type, period, [limit], current_usage, reset_at, updated_at
			FROM quotas
			WHERE reset_at <= ?
		`

		rows, err := qm.db.QueryContext(ctx, query, time.Now())
		if err != nil {
			continue
		}

		var quotasToReset []*database.Quota
		for rows.Next() {
			quota := &database.Quota{}
			err := rows.Scan(
				&quota.ID,
				&quota.TenantID,
				&quota.QuotaType,
				&quota.Period,
				&quota.Limit,
				&quota.CurrentUsage,
				&quota.ResetAt,
				&quota.UpdatedAt,
			)
			if err != nil {
				continue
			}
			quotasToReset = append(quotasToReset, quota)
		}
		rows.Close()

		// Reset each quota
		for _, quota := range quotasToReset {
			_ = qm.resetQuotaInternal(ctx, quota)
		}
	}
}

// calculateNextReset calculates the next reset time based on the period
func calculateNextReset(period string) time.Time {
	now := time.Now()
	
	switch period {
	case "daily":
		// Reset at midnight
		return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	case "monthly":
		// Reset at start of next month
		if now.Month() == time.December {
			return time.Date(now.Year()+1, time.January, 1, 0, 0, 0, 0, now.Location())
		}
		return time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
	default:
		// Default to daily
		return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	}
}
