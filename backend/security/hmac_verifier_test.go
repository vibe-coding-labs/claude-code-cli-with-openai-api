package security

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHMACVerifier_GenerateSignature(t *testing.T) {
	hv := NewHMACVerifier(nil)

	sig1, err := hv.GenerateSignature("POST", "/v1/messages", `{"a":1}`, 1700000000, "test-secret-123")
	require.NoError(t, err)
	assert.NotEmpty(t, sig1)

	// Deterministic: same inputs produce the same signature
	sig2, err := hv.GenerateSignature("POST", "/v1/messages", `{"a":1}`, 1700000000, "test-secret-123")
	require.NoError(t, err)
	assert.Equal(t, sig1, sig2)

	// Changing any component changes the signature
	sig3, err := hv.GenerateSignature("GET", "/v1/messages", `{"a":1}`, 1700000000, "test-secret-123")
	require.NoError(t, err)
	assert.NotEqual(t, sig1, sig3)

	sig4, err := hv.GenerateSignature("POST", "/v1/messages", `{"a":1}`, 1700000000, "different-secret")
	require.NoError(t, err)
	assert.NotEqual(t, sig1, sig4)
}

func TestHMACVerifier_GetTenantSecret(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	hv := NewHMACVerifier(db)
	ctx := context.Background()

	// No tenant / no key at all
	_, err := hv.GetTenantSecret(ctx, "missing-tenant")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no active API key found")

	_, err = db.Exec(`INSERT INTO tenants (id, name, status) VALUES (?, ?, ?)`, "tenant-1", "Tenant One", "active")
	require.NoError(t, err)

	// Key exists but revoked -> still "no active API key"
	_, err = db.Exec(`INSERT INTO api_keys (id, key_hash, tenant_id, name, status, hmac_secret) VALUES (?, ?, ?, ?, ?, ?)`,
		"key-revoked", "hash-1", "tenant-1", "revoked key", "revoked", "secret-abc")
	require.NoError(t, err)
	_, err = hv.GetTenantSecret(ctx, "tenant-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no active API key found")

	// Active key but empty hmac secret
	_, err = db.Exec(`INSERT INTO api_keys (id, key_hash, tenant_id, name, status, hmac_secret) VALUES (?, ?, ?, ?, ?, ?)`,
		"key-nosecret", "hash-2", "tenant-1", "no secret key", "active", "")
	require.NoError(t, err)
	_, err = hv.GetTenantSecret(ctx, "tenant-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HMAC secret not configured")

	// Remove the empty-secret key and add a proper active key
	_, err = db.Exec(`DELETE FROM api_keys WHERE id = ?`, "key-nosecret")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO api_keys (id, key_hash, tenant_id, name, status, hmac_secret) VALUES (?, ?, ?, ?, ?, ?)`,
		"key-active", "hash-3", "tenant-1", "active key", "active", "real-secret-xyz")
	require.NoError(t, err)

	secret, err := hv.GetTenantSecret(ctx, "tenant-1")
	require.NoError(t, err)
	assert.Equal(t, "real-secret-xyz", secret)
}

func TestHMACVerifier_VerifySignature(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	hv := NewHMACVerifier(db)
	ctx := context.Background()

	_, err := db.Exec(`INSERT INTO tenants (id, name, status) VALUES (?, ?, ?)`, "tenant-1", "Tenant One", "active")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO api_keys (id, key_hash, tenant_id, name, status, hmac_secret) VALUES (?, ?, ?, ?, ?, ?)`,
		"key-active", "hash-1", "tenant-1", "active key", "active", "shared-secret-123")
	require.NoError(t, err)

	newSignedRequest := func(method, path, body string, ts int64) (*http.Request, string) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("X-Timestamp", strconv.FormatInt(ts, 10))
		sig, sigErr := hv.GenerateSignature(method, path, body, ts, "shared-secret-123")
		require.NoError(t, sigErr)
		return req, sig
	}

	t.Run("valid signature", func(t *testing.T) {
		req, sig := newSignedRequest("POST", "/v1/messages", `{"hello":"world"}`, time.Now().Unix())
		valid, err := hv.VerifySignature(ctx, req, sig, "tenant-1")
		require.NoError(t, err)
		assert.True(t, valid)
	})

	t.Run("tampered signature rejected", func(t *testing.T) {
		req, _ := newSignedRequest("POST", "/v1/messages", `{"hello":"world"}`, time.Now().Unix())
		valid, err := hv.VerifySignature(ctx, req, "0000000000000000000000000000000000000000000000000000000000000000", "tenant-1")
		require.NoError(t, err)
		assert.False(t, valid)
	})

	t.Run("tampered body invalidates signature", func(t *testing.T) {
		ts := time.Now().Unix()
		_, sig := newSignedRequest("POST", "/v1/messages", `{"hello":"world"}`, ts)
		// Attacker re-sends with a different body but reuses the old signature
		req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"hello":"INJECTED"}`))
		req.Header.Set("X-Timestamp", strconv.FormatInt(ts, 10))
		valid, err := hv.VerifySignature(ctx, req, sig, "tenant-1")
		require.NoError(t, err)
		assert.False(t, valid)
	})

	t.Run("missing timestamp header", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{}`))
		_, err := hv.VerifySignature(ctx, req, "whatever", "tenant-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing X-Timestamp")
	})

	t.Run("invalid timestamp format", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{}`))
		req.Header.Set("X-Timestamp", "not-a-number")
		_, err := hv.VerifySignature(ctx, req, "whatever", "tenant-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid timestamp")
	})

	t.Run("timestamp too old is rejected (replay protection)", func(t *testing.T) {
		oldTs := time.Now().Add(-1 * time.Hour).Unix()
		req, sig := newSignedRequest("POST", "/v1/messages", `{}`, oldTs)
		_, err := hv.VerifySignature(ctx, req, sig, "tenant-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timestamp outside valid window")
	})

	t.Run("timestamp in the future is rejected", func(t *testing.T) {
		futureTs := time.Now().Add(1 * time.Hour).Unix()
		req, sig := newSignedRequest("POST", "/v1/messages", `{}`, futureTs)
		_, err := hv.VerifySignature(ctx, req, sig, "tenant-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timestamp outside valid window")
	})

	t.Run("unknown tenant fails to resolve secret", func(t *testing.T) {
		req, sig := newSignedRequest("POST", "/v1/messages", `{}`, time.Now().Unix())
		_, err := hv.VerifySignature(ctx, req, sig, "no-such-tenant")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get tenant secret")
	})

	t.Run("nil body is handled", func(t *testing.T) {
		ts := time.Now().Unix()
		sig, sigErr := hv.GenerateSignature("GET", "/v1/status", "", ts, "shared-secret-123")
		require.NoError(t, sigErr)
		req := httptest.NewRequest("GET", "/v1/status", nil)
		req.Body = nil
		req.Header.Set("X-Timestamp", strconv.FormatInt(ts, 10))
		valid, err := hv.VerifySignature(ctx, req, sig, "tenant-1")
		require.NoError(t, err)
		assert.True(t, valid)
	})

	t.Run("custom timestamp window is honored", func(t *testing.T) {
		hvCustom := NewHMACVerifier(db).(*hmacVerifier)
		hvCustom.SetTimestampWindow(1 * time.Second)

		ts := time.Now().Add(-2 * time.Second).Unix()
		req, sig := newSignedRequest("POST", "/v1/messages", `{}`, ts)
		_, err := hvCustom.VerifySignature(ctx, req, sig, "tenant-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "timestamp outside valid window")
	})
}
