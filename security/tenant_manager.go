package security

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// TenantManager defines the interface for tenant management operations
type TenantManager interface {
	CreateTenant(ctx context.Context, tenant *database.Tenant) error
	GetTenant(ctx context.Context, tenantID string) (*database.Tenant, error)
	UpdateTenant(ctx context.Context, tenant *database.Tenant) error
	DeleteTenant(ctx context.Context, tenantID string) error
	ListTenants(ctx context.Context, filters database.TenantFilters) ([]*database.Tenant, error)
	GetTenantConfig(ctx context.Context, tenantID string) (*database.TenantConfig, error)
	UpdateTenantConfig(ctx context.Context, config *database.TenantConfig) error
}

// tenantManager implements the TenantManager interface
type tenantManager struct {
	db *sql.DB
}

// NewTenantManager creates a new TenantManager instance
func NewTenantManager(db *sql.DB) TenantManager {
	return &tenantManager{db: db}
}

// CreateTenant creates a new tenant
func (tm *tenantManager) CreateTenant(ctx context.Context, tenant *database.Tenant) error {
	// Generate ID if not provided
	if tenant.ID == "" {
		tenant.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now

	// Set default status if not provided
	if tenant.Status == "" {
		tenant.Status = "active"
	}

	// Validate tenant data
	if err := tenant.Validate(); err != nil {
		return fmt.Errorf("invalid tenant data: %w", err)
	}

	// Check if tenant name already exists
	var count int
	err := tm.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tenants WHERE name = ?", tenant.Name).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check tenant name uniqueness: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("tenant with name '%s' already exists", tenant.Name)
	}

	// Insert tenant
	query := `
		INSERT INTO tenants (id, name, description, status, created_at, updated_at, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err = tm.db.ExecContext(ctx, query,
		tenant.ID, tenant.Name, tenant.Description, tenant.Status,
		tenant.CreatedAt, tenant.UpdatedAt, tenant.Metadata)
	if err != nil {
		return fmt.Errorf("failed to create tenant: %w", err)
	}

	// Create default tenant config
	config := &database.TenantConfig{
		TenantID:         tenant.ID,
		CustomRateLimits: false,
		RequireHMAC:      false,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := tm.createTenantConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to create tenant config: %w", err)
	}

	return nil
}

// GetTenant retrieves a tenant by ID
func (tm *tenantManager) GetTenant(ctx context.Context, tenantID string) (*database.Tenant, error) {
	query := `
		SELECT id, name, description, status, created_at, updated_at, metadata
		FROM tenants
		WHERE id = ?
	`
	tenant := &database.Tenant{}
	err := tm.db.QueryRowContext(ctx, query, tenantID).Scan(
		&tenant.ID, &tenant.Name, &tenant.Description, &tenant.Status,
		&tenant.CreatedAt, &tenant.UpdatedAt, &tenant.Metadata)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("tenant not found: %s", tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}
	return tenant, nil
}

// UpdateTenant updates an existing tenant
func (tm *tenantManager) UpdateTenant(ctx context.Context, tenant *database.Tenant) error {
	// Validate tenant data
	if err := tenant.Validate(); err != nil {
		return fmt.Errorf("invalid tenant data: %w", err)
	}

	// Check if tenant exists
	existing, err := tm.GetTenant(ctx, tenant.ID)
	if err != nil {
		return err
	}

	// Check if name is being changed and if new name already exists
	if existing.Name != tenant.Name {
		var count int
		err := tm.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tenants WHERE name = ? AND id != ?",
			tenant.Name, tenant.ID).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check tenant name uniqueness: %w", err)
		}
		if count > 0 {
			return fmt.Errorf("tenant with name '%s' already exists", tenant.Name)
		}
	}

	// Update timestamp
	tenant.UpdatedAt = time.Now()

	// Update tenant
	query := `
		UPDATE tenants
		SET name = ?, description = ?, status = ?, updated_at = ?, metadata = ?
		WHERE id = ?
	`
	result, err := tm.db.ExecContext(ctx, query,
		tenant.Name, tenant.Description, tenant.Status, tenant.UpdatedAt, tenant.Metadata, tenant.ID)
	if err != nil {
		return fmt.Errorf("failed to update tenant: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("tenant not found: %s", tenant.ID)
	}

	return nil
}

// DeleteTenant deletes a tenant and all associated data
func (tm *tenantManager) DeleteTenant(ctx context.Context, tenantID string) error {
	// Check if tenant exists
	_, err := tm.GetTenant(ctx, tenantID)
	if err != nil {
		return err
	}

	// Delete tenant (cascade will handle related records)
	query := `DELETE FROM tenants WHERE id = ?`
	result, err := tm.db.ExecContext(ctx, query, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("tenant not found: %s", tenantID)
	}

	return nil
}

// ListTenants retrieves a list of tenants with optional filters
func (tm *tenantManager) ListTenants(ctx context.Context, filters database.TenantFilters) ([]*database.Tenant, error) {
	query := `
		SELECT id, name, description, status, created_at, updated_at, metadata
		FROM tenants
		WHERE 1=1
	`
	args := []interface{}{}

	// Apply filters
	if filters.Status != "" {
		query += " AND status = ?"
		args = append(args, filters.Status)
	}

	// Apply sorting
	sortBy := "created_at"
	if filters.SortBy != "" {
		sortBy = filters.SortBy
	}
	sortOrder := "DESC"
	if filters.SortOrder != "" {
		sortOrder = filters.SortOrder
	}
	query += fmt.Sprintf(" ORDER BY %s %s", sortBy, sortOrder)

	// Apply pagination
	if filters.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filters.Limit)
	}
	if filters.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filters.Offset)
	}

	rows, err := tm.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []*database.Tenant
	for rows.Next() {
		tenant := &database.Tenant{}
		err := rows.Scan(
			&tenant.ID, &tenant.Name, &tenant.Description, &tenant.Status,
			&tenant.CreatedAt, &tenant.UpdatedAt, &tenant.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tenant: %w", err)
		}
		tenants = append(tenants, tenant)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tenants: %w", err)
	}

	return tenants, nil
}

// GetTenantConfig retrieves the configuration for a tenant
func (tm *tenantManager) GetTenantConfig(ctx context.Context, tenantID string) (*database.TenantConfig, error) {
	query := `
		SELECT tenant_id, allowed_models, default_model, custom_rate_limits,
		       require_hmac, webhook_url, alert_email, created_at, updated_at
		FROM tenant_configs
		WHERE tenant_id = ?
	`
	config := &database.TenantConfig{}
	var allowedModelsJSON sql.NullString

	err := tm.db.QueryRowContext(ctx, query, tenantID).Scan(
		&config.TenantID, &allowedModelsJSON, &config.DefaultModel,
		&config.CustomRateLimits, &config.RequireHMAC, &config.WebhookURL,
		&config.AlertEmail, &config.CreatedAt, &config.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("tenant config not found: %s", tenantID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant config: %w", err)
	}

	// Parse allowed models JSON
	if allowedModelsJSON.Valid && allowedModelsJSON.String != "" {
		// Simple JSON array parsing (could use json.Unmarshal for more complex cases)
		config.AllowedModels = []string{} // TODO: Parse JSON array
	}

	return config, nil
}

// UpdateTenantConfig updates the configuration for a tenant
func (tm *tenantManager) UpdateTenantConfig(ctx context.Context, config *database.TenantConfig) error {
	// Validate config data
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid tenant config data: %w", err)
	}

	// Update timestamp
	config.UpdatedAt = time.Now()

	// Convert allowed models to JSON
	allowedModelsJSON := "" // TODO: Convert to JSON array

	// Update config
	query := `
		UPDATE tenant_configs
		SET allowed_models = ?, default_model = ?, custom_rate_limits = ?,
		    require_hmac = ?, webhook_url = ?, alert_email = ?, updated_at = ?
		WHERE tenant_id = ?
	`
	result, err := tm.db.ExecContext(ctx, query,
		allowedModelsJSON, config.DefaultModel, config.CustomRateLimits,
		config.RequireHMAC, config.WebhookURL, config.AlertEmail,
		config.UpdatedAt, config.TenantID)
	if err != nil {
		return fmt.Errorf("failed to update tenant config: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("tenant config not found: %s", config.TenantID)
	}

	return nil
}

// createTenantConfig creates a new tenant configuration
func (tm *tenantManager) createTenantConfig(ctx context.Context, config *database.TenantConfig) error {
	// Validate config data
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid tenant config data: %w", err)
	}

	// Convert allowed models to JSON
	allowedModelsJSON := "" // TODO: Convert to JSON array

	// Insert config
	query := `
		INSERT INTO tenant_configs (tenant_id, allowed_models, default_model,
		                           custom_rate_limits, require_hmac, webhook_url,
		                           alert_email, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := tm.db.ExecContext(ctx, query,
		config.TenantID, allowedModelsJSON, config.DefaultModel,
		config.CustomRateLimits, config.RequireHMAC, config.WebhookURL,
		config.AlertEmail, config.CreatedAt, config.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create tenant config: %w", err)
	}

	return nil
}
