package security

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"golang.org/x/crypto/bcrypt"
)

// APIKeyManager defines the interface for API key management operations
type APIKeyManager interface {
	CreateKey(ctx context.Context, tenantID string, name string, expiration *time.Time) (*APIKey, error)
	ValidateKey(ctx context.Context, key string) (*database.APIKey, error)
	RevokeKey(ctx context.Context, keyID string) error
	RotateKey(ctx context.Context, keyID string, gracePeriod time.Duration) (*APIKey, error)
	ListKeys(ctx context.Context, tenantID string) ([]*database.APIKey, error)
	UpdateLastUsed(ctx context.Context, keyID string) error
}

// APIKey represents an API key with the plain text key (only returned on creation)
type APIKey struct {
	*database.APIKey
	PlainKey string `json:"key"` // Only populated on creation
}

// apiKeyManager implements the APIKeyManager interface
type apiKeyManager struct {
	db *sql.DB
}

// NewAPIKeyManager creates a new APIKeyManager instance
func NewAPIKeyManager(db *sql.DB) APIKeyManager {
	return &apiKeyManager{db: db}
}

// generateSecureKey generates a cryptographically secure random API key
func generateSecureKey() (string, error) {
	// Generate 32 bytes of random data
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode to base64 for a readable key
	key := base64.URLEncoding.EncodeToString(bytes)
	return key, nil
}

// generateHMACSecret generates a cryptographically secure HMAC secret
func generateHMACSecret() (string, error) {
	// Generate 64 bytes of random data for HMAC secret
	bytes := make([]byte, 64)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode to base64
	secret := base64.URLEncoding.EncodeToString(bytes)
	return secret, nil
}

// hashKey hashes an API key using bcrypt
func hashKey(key string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(key), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash key: %w", err)
	}
	return string(hash), nil
}

// verifyKey verifies an API key against its hash
func verifyKey(key, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(key))
	return err == nil
}

// CreateKey creates a new API key for a tenant
func (akm *apiKeyManager) CreateKey(ctx context.Context, tenantID string, name string, expiration *time.Time) (*APIKey, error) {
	// Generate secure key
	plainKey, err := generateSecureKey()
	if err != nil {
		return nil, err
	}

	// Hash the key
	keyHash, err := hashKey(plainKey)
	if err != nil {
		return nil, err
	}

	// Generate HMAC secret
	hmacSecret, err := generateHMACSecret()
	if err != nil {
		return nil, err
	}

	// Create API key record
	apiKey := &database.APIKey{
		ID:         uuid.New().String(),
		KeyHash:    keyHash,
		TenantID:   tenantID,
		Name:       name,
		Status:     "active",
		CreatedAt:  time.Now(),
		ExpiresAt:  expiration,
		HMACSecret: hmacSecret,
	}

	// Validate API key data
	if err := apiKey.Validate(); err != nil {
		return nil, fmt.Errorf("invalid API key data: %w", err)
	}

	// Insert API key
	query := `
		INSERT INTO api_keys (id, key_hash, tenant_id, name, status, created_at, expires_at, hmac_secret)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = akm.db.ExecContext(ctx, query,
		apiKey.ID, apiKey.KeyHash, apiKey.TenantID, apiKey.Name,
		apiKey.Status, apiKey.CreatedAt, apiKey.ExpiresAt, apiKey.HMACSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	// Return API key with plain text key
	return &APIKey{
		APIKey:   apiKey,
		PlainKey: plainKey,
	}, nil
}

// ValidateKey validates an API key and returns the associated key record
func (akm *apiKeyManager) ValidateKey(ctx context.Context, key string) (*database.APIKey, error) {
	// Query all active keys (we need to check hash for each)
	query := `
		SELECT id, key_hash, tenant_id, name, status, created_at, expires_at, last_used_at, hmac_secret
		FROM api_keys
		WHERE status = 'active'
	`
	rows, err := akm.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query API keys: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		apiKey := &database.APIKey{}
		var expiresAt, lastUsedAt sql.NullTime

		err := rows.Scan(
			&apiKey.ID, &apiKey.KeyHash, &apiKey.TenantID, &apiKey.Name,
			&apiKey.Status, &apiKey.CreatedAt, &expiresAt, &lastUsedAt, &apiKey.HMACSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to scan API key: %w", err)
		}

		apiKey.ExpiresAt = database.NullTimeToPtr(expiresAt)
		apiKey.LastUsedAt = database.NullTimeToPtr(lastUsedAt)

		// Check if key matches hash
		if verifyKey(key, apiKey.KeyHash) {
			// Check if key is expired
			if apiKey.IsExpired() {
				return nil, fmt.Errorf("API key has expired")
			}

			// Update last used timestamp
			if err := akm.UpdateLastUsed(ctx, apiKey.ID); err != nil {
				// Log error but don't fail validation
				fmt.Printf("Warning: failed to update last used timestamp: %v\n", err)
			}

			return apiKey, nil
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating API keys: %w", err)
	}

	return nil, fmt.Errorf("invalid API key")
}

// RevokeKey revokes an API key immediately
func (akm *apiKeyManager) RevokeKey(ctx context.Context, keyID string) error {
	query := `
		UPDATE api_keys
		SET status = 'revoked'
		WHERE id = ?
	`
	result, err := akm.db.ExecContext(ctx, query, keyID)
	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("API key not found: %s", keyID)
	}

	return nil
}

// RotateKey rotates an API key, creating a new key while keeping the old one valid for a grace period
func (akm *apiKeyManager) RotateKey(ctx context.Context, keyID string, gracePeriod time.Duration) (*APIKey, error) {
	// Get existing key
	query := `
		SELECT id, key_hash, tenant_id, name, status, created_at, expires_at, last_used_at, hmac_secret
		FROM api_keys
		WHERE id = ?
	`
	oldKey := &database.APIKey{}
	var expiresAt, lastUsedAt sql.NullTime

	err := akm.db.QueryRowContext(ctx, query, keyID).Scan(
		&oldKey.ID, &oldKey.KeyHash, &oldKey.TenantID, &oldKey.Name,
		&oldKey.Status, &oldKey.CreatedAt, &expiresAt, &lastUsedAt, &oldKey.HMACSecret)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("API key not found: %s", keyID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get API key: %w", err)
	}

	oldKey.ExpiresAt = database.NullTimeToPtr(expiresAt)
	oldKey.LastUsedAt = database.NullTimeToPtr(lastUsedAt)

	// Set expiration for old key (grace period)
	graceExpiration := time.Now().Add(gracePeriod)
	updateQuery := `
		UPDATE api_keys
		SET expires_at = ?
		WHERE id = ?
	`
	_, err = akm.db.ExecContext(ctx, updateQuery, graceExpiration, keyID)
	if err != nil {
		return nil, fmt.Errorf("failed to update old key expiration: %w", err)
	}

	// Create new key with same name
	newKeyName := oldKey.Name
	if newKeyName != "" {
		newKeyName = newKeyName + " (rotated)"
	}

	// Use the same expiration as the old key (if it had one)
	var newExpiration *time.Time
	if oldKey.ExpiresAt != nil {
		exp := *oldKey.ExpiresAt
		newExpiration = &exp
	}

	newKey, err := akm.CreateKey(ctx, oldKey.TenantID, newKeyName, newExpiration)
	if err != nil {
		return nil, fmt.Errorf("failed to create new key: %w", err)
	}

	return newKey, nil
}

// ListKeys lists all API keys for a tenant
func (akm *apiKeyManager) ListKeys(ctx context.Context, tenantID string) ([]*database.APIKey, error) {
	query := `
		SELECT id, key_hash, tenant_id, name, status, created_at, expires_at, last_used_at, hmac_secret
		FROM api_keys
		WHERE tenant_id = ?
		ORDER BY created_at DESC
	`
	rows, err := akm.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	defer rows.Close()

	var keys []*database.APIKey
	for rows.Next() {
		apiKey := &database.APIKey{}
		var expiresAt, lastUsedAt sql.NullTime

		err := rows.Scan(
			&apiKey.ID, &apiKey.KeyHash, &apiKey.TenantID, &apiKey.Name,
			&apiKey.Status, &apiKey.CreatedAt, &expiresAt, &lastUsedAt, &apiKey.HMACSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to scan API key: %w", err)
		}

		apiKey.ExpiresAt = database.NullTimeToPtr(expiresAt)
		apiKey.LastUsedAt = database.NullTimeToPtr(lastUsedAt)

		keys = append(keys, apiKey)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating API keys: %w", err)
	}

	return keys, nil
}

// UpdateLastUsed updates the last used timestamp for an API key
func (akm *apiKeyManager) UpdateLastUsed(ctx context.Context, keyID string) error {
	query := `
		UPDATE api_keys
		SET last_used_at = ?
		WHERE id = ?
	`
	_, err := akm.db.ExecContext(ctx, query, time.Now(), keyID)
	if err != nil {
		return fmt.Errorf("failed to update last used timestamp: %w", err)
	}
	return nil
}
