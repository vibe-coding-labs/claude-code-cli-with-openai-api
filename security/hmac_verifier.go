package security

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// HMACVerifier defines the interface for HMAC signature verification
type HMACVerifier interface {
	VerifySignature(ctx context.Context, request *http.Request, signature string, tenantID string) (valid bool, err error)
	GenerateSignature(method, path, body string, timestamp int64, secret string) (string, error)
	GetTenantSecret(ctx context.Context, tenantID string) (string, error)
}

// hmacVerifier implements the HMACVerifier interface
type hmacVerifier struct {
	db                *sql.DB
	timestampWindow   time.Duration // Default: 5 minutes
}

// NewHMACVerifier creates a new HMACVerifier instance
func NewHMACVerifier(db *sql.DB) HMACVerifier {
	return &hmacVerifier{
		db:              db,
		timestampWindow: 5 * time.Minute,
	}
}

// VerifySignature verifies the HMAC signature of a request
func (hv *hmacVerifier) VerifySignature(ctx context.Context, request *http.Request, signature string, tenantID string) (valid bool, err error) {
	// Get tenant's HMAC secret
	secret, err := hv.GetTenantSecret(ctx, tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to get tenant secret: %w", err)
	}

	// Extract timestamp from request header
	timestampStr := request.Header.Get("X-Timestamp")
	if timestampStr == "" {
		return false, fmt.Errorf("missing X-Timestamp header")
	}

	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return false, fmt.Errorf("invalid timestamp: %w", err)
	}

	// Validate timestamp (prevent replay attacks)
	requestTime := time.Unix(timestamp, 0)
	now := time.Now()
	timeDiff := now.Sub(requestTime)

	if timeDiff < 0 {
		timeDiff = -timeDiff
	}

	if timeDiff > hv.timestampWindow {
		return false, fmt.Errorf("timestamp outside valid window (max %v)", hv.timestampWindow)
	}

	// Read request body
	var body string
	if request.Body != nil {
		bodyBytes, err := io.ReadAll(request.Body)
		if err != nil {
			return false, fmt.Errorf("failed to read request body: %w", err)
		}
		body = string(bodyBytes)
		// Note: In production, you'd want to restore the body for downstream handlers
	}

	// Generate expected signature
	expectedSignature, err := hv.GenerateSignature(
		request.Method,
		request.URL.Path,
		body,
		timestamp,
		secret,
	)
	if err != nil {
		return false, fmt.Errorf("failed to generate signature: %w", err)
	}

	// Compare signatures (constant time comparison to prevent timing attacks)
	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return false, nil
	}

	return true, nil
}

// GenerateSignature generates an HMAC-SHA256 signature for a request
func (hv *hmacVerifier) GenerateSignature(method, path, body string, timestamp int64, secret string) (string, error) {
	// Create signature payload: method + path + body + timestamp
	payload := fmt.Sprintf("%s%s%s%d", method, path, body, timestamp)

	// Create HMAC-SHA256 hash
	h := hmac.New(sha256.New, []byte(secret))
	_, err := h.Write([]byte(payload))
	if err != nil {
		return "", fmt.Errorf("failed to write to HMAC: %w", err)
	}

	// Get signature as hex string
	signature := hex.EncodeToString(h.Sum(nil))

	return signature, nil
}

// GetTenantSecret retrieves the HMAC secret for a tenant
func (hv *hmacVerifier) GetTenantSecret(ctx context.Context, tenantID string) (string, error) {
	// Query the first active API key for the tenant to get HMAC secret
	query := `
		SELECT hmac_secret
		FROM api_keys
		WHERE tenant_id = ? AND status = 'active'
		LIMIT 1
	`
	var secret string
	err := hv.db.QueryRowContext(ctx, query, tenantID).Scan(&secret)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("no active API key found for tenant: %s", tenantID)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get tenant secret: %w", err)
	}

	if secret == "" {
		return "", fmt.Errorf("HMAC secret not configured for tenant: %s", tenantID)
	}

	return secret, nil
}

// SetTimestampWindow sets the timestamp validation window
func (hv *hmacVerifier) SetTimestampWindow(window time.Duration) {
	hv.timestampWindow = window
}
