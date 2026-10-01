package security

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

func TestIPFilter_DefaultAllowWhenNoRules(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	filter, err := NewIPFilter(db)
	require.NoError(t, err)

	allowed, err := filter.CheckIP(context.Background(), "1.2.3.4", "tenant-1")
	require.NoError(t, err)
	assert.True(t, allowed, "with no rules configured, traffic should be allowed by default")
}

func TestIPFilter_InvalidIPRejected(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	filter, err := NewIPFilter(db)
	require.NoError(t, err)

	_, err = filter.CheckIP(context.Background(), "not-an-ip", "tenant-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid IP address")
}

func TestIPFilter_GlobalBlacklist(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	filter, err := NewIPFilter(db)
	require.NoError(t, err)
	ctx := context.Background()

	err = filter.AddBlacklist(ctx, database.IPRule{IPAddress: "10.0.0.0/8", Description: "internal range"})
	require.NoError(t, err)

	allowed, err := filter.CheckIP(ctx, "10.1.2.3", "")
	require.NoError(t, err)
	assert.False(t, allowed)

	// Outside the blacklisted range and no whitelist configured -> allowed
	allowed, err = filter.CheckIP(ctx, "8.8.8.8", "")
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestIPFilter_TenantBlacklist(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	filter, err := NewIPFilter(db)
	require.NoError(t, err)
	ctx := context.Background()

	err = filter.AddBlacklist(ctx, database.IPRule{TenantID: "tenant-1", IPAddress: "203.0.113.5"})
	require.NoError(t, err)

	allowed, err := filter.CheckIP(ctx, "203.0.113.5", "tenant-1")
	require.NoError(t, err)
	assert.False(t, allowed)

	// Same IP under a different tenant is unaffected
	allowed, err = filter.CheckIP(ctx, "203.0.113.5", "tenant-2")
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestIPFilter_WhitelistEnforced(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	filter, err := NewIPFilter(db)
	require.NoError(t, err)
	ctx := context.Background()

	err = filter.AddWhitelist(ctx, database.IPRule{TenantID: "tenant-1", IPAddress: "198.51.100.0/24"})
	require.NoError(t, err)

	// In whitelist range
	allowed, err := filter.CheckIP(ctx, "198.51.100.42", "tenant-1")
	require.NoError(t, err)
	assert.True(t, allowed)

	// Not in whitelist range -> denied (whitelist mode active for this tenant)
	allowed, err = filter.CheckIP(ctx, "1.2.3.4", "tenant-1")
	require.NoError(t, err)
	assert.False(t, allowed)

	// A different tenant with no whitelist rules of its own still defaults to allow
	allowed, err = filter.CheckIP(ctx, "1.2.3.4", "tenant-2")
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestIPFilter_GlobalWhitelist(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	filter, err := NewIPFilter(db)
	require.NoError(t, err)
	ctx := context.Background()

	err = filter.AddWhitelist(ctx, database.IPRule{IPAddress: "192.0.2.10"})
	require.NoError(t, err)

	allowed, err := filter.CheckIP(ctx, "192.0.2.10", "tenant-1")
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, err = filter.CheckIP(ctx, "192.0.2.11", "tenant-1")
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestIPFilter_BlacklistTakesPrecedenceOverWhitelist(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	filter, err := NewIPFilter(db)
	require.NoError(t, err)
	ctx := context.Background()

	err = filter.AddWhitelist(ctx, database.IPRule{TenantID: "tenant-1", IPAddress: "10.0.0.0/8"})
	require.NoError(t, err)
	err = filter.AddBlacklist(ctx, database.IPRule{TenantID: "tenant-1", IPAddress: "10.0.0.5"})
	require.NoError(t, err)

	// Explicitly blacklisted IP within an otherwise-whitelisted range must be denied
	allowed, err := filter.CheckIP(ctx, "10.0.0.5", "tenant-1")
	require.NoError(t, err)
	assert.False(t, allowed)

	// Other IPs within the whitelisted range remain allowed
	allowed, err = filter.CheckIP(ctx, "10.0.0.6", "tenant-1")
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestIPFilter_AddRuleSingleIPConvertedToCIDR(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	filter, err := NewIPFilter(db)
	require.NoError(t, err)
	ctx := context.Background()

	err = filter.AddBlacklist(ctx, database.IPRule{IPAddress: "1.1.1.1"})
	require.NoError(t, err)

	rules, err := filter.ListRules(ctx, "")
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "1.1.1.1/32", rules[0].IPAddress)
}

func TestIPFilter_AddRuleIPv6SingleAddress(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	filter, err := NewIPFilter(db)
	require.NoError(t, err)
	ctx := context.Background()

	err = filter.AddBlacklist(ctx, database.IPRule{IPAddress: "2001:db8::1"})
	require.NoError(t, err)

	allowed, err := filter.CheckIP(ctx, "2001:db8::1", "")
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestIPFilter_AddRuleInvalidAddress(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	filter, err := NewIPFilter(db)
	require.NoError(t, err)

	err = filter.AddBlacklist(context.Background(), database.IPRule{IPAddress: "not-an-ip-at-all"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid IP address or CIDR")
}

func TestIPFilter_AddRuleValidationError(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	filter, err := NewIPFilter(db)
	require.NoError(t, err)

	// Empty IP address fails IPRule.Validate() before CIDR parsing is attempted
	err = filter.AddBlacklist(context.Background(), database.IPRule{IPAddress: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid IP rule")
}

func TestIPFilter_RemoveRule(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	filter, err := NewIPFilter(db)
	require.NoError(t, err)
	ctx := context.Background()

	err = filter.AddBlacklist(ctx, database.IPRule{ID: "rule-1", IPAddress: "5.5.5.5"})
	require.NoError(t, err)

	allowed, err := filter.CheckIP(ctx, "5.5.5.5", "")
	require.NoError(t, err)
	assert.False(t, allowed)

	err = filter.RemoveRule(ctx, "rule-1")
	require.NoError(t, err)

	allowed, err = filter.CheckIP(ctx, "5.5.5.5", "")
	require.NoError(t, err)
	assert.True(t, allowed, "rule should no longer apply after removal")
}

func TestIPFilter_RemoveRuleNotFound(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	filter, err := NewIPFilter(db)
	require.NoError(t, err)

	err = filter.RemoveRule(context.Background(), "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IP rule not found")
}

func TestIPFilter_ListRules(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	filter, err := NewIPFilter(db)
	require.NoError(t, err)
	ctx := context.Background()

	require.NoError(t, filter.AddWhitelist(ctx, database.IPRule{TenantID: "tenant-1", IPAddress: "1.1.1.1"}))
	require.NoError(t, filter.AddBlacklist(ctx, database.IPRule{IPAddress: "2.2.2.2"})) // global rule
	require.NoError(t, filter.AddWhitelist(ctx, database.IPRule{TenantID: "tenant-2", IPAddress: "3.3.3.3"}))

	rules, err := filter.ListRules(ctx, "tenant-1")
	require.NoError(t, err)
	// tenant-1 specific rule + the global rule should both be visible
	assert.Len(t, rules, 2)
}

func TestIPFilter_ReloadRulesSkipsInvalidCIDR(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	ctx := context.Background()

	// Insert a corrupt row directly (bypassing addRule's validation) to make sure
	// ReloadRules tolerates bad data already present in the database instead of failing outright.
	_, err := db.Exec(`INSERT INTO ip_rules (id, tenant_id, rule_type, ip_address, description) VALUES (?, ?, ?, ?, ?)`,
		"corrupt-1", nil, "blacklist", "this-is-not-valid", "corrupt row")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO ip_rules (id, tenant_id, rule_type, ip_address, description) VALUES (?, ?, ?, ?, ?)`,
		"valid-1", nil, "blacklist", "9.9.9.9/32", "valid row")
	require.NoError(t, err)

	filter, err := NewIPFilter(db)
	require.NoError(t, err)

	allowed, err := filter.CheckIP(ctx, "9.9.9.9", "")
	require.NoError(t, err)
	assert.False(t, allowed, "valid rule alongside a corrupt one should still be loaded and enforced")

	rules, err := filter.ListRules(ctx, "")
	require.NoError(t, err)
	// ListRules reads directly from the DB, so the corrupt row is still visible there
	assert.Len(t, rules, 2)
}

func TestIPFilter_ReloadRulesQueryError(t *testing.T) {
	db := setupFullSecurityTestDB(t)
	filter, err := NewIPFilter(db)
	require.NoError(t, err)

	_, err = db.Exec(`DROP TABLE ip_rules`)
	require.NoError(t, err)

	err = filter.ReloadRules(context.Background())
	require.Error(t, err)
}
