package security

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// IPFilter defines the interface for IP filtering operations
type IPFilter interface {
	CheckIP(ctx context.Context, ip string, tenantID string) (allowed bool, err error)
	AddWhitelist(ctx context.Context, rule database.IPRule) error
	AddBlacklist(ctx context.Context, rule database.IPRule) error
	RemoveRule(ctx context.Context, ruleID string) error
	ListRules(ctx context.Context, tenantID string) ([]database.IPRule, error)
	ReloadRules(ctx context.Context) error
}

// ipFilter implements the IPFilter interface
type ipFilter struct {
	db             *sql.DB
	whitelistRules map[string][]*ipRule // tenantID -> rules
	blacklistRules map[string][]*ipRule // tenantID -> rules
	globalWhitelist []*ipRule
	globalBlacklist []*ipRule
	mu             sync.RWMutex
}

// ipRule represents a parsed IP rule with CIDR support
type ipRule struct {
	id          string
	tenantID    string
	ruleType    string
	ipNet       *net.IPNet
	description string
}

// NewIPFilter creates a new IPFilter instance
func NewIPFilter(db *sql.DB) (IPFilter, error) {
	filter := &ipFilter{
		db:             db,
		whitelistRules: make(map[string][]*ipRule),
		blacklistRules: make(map[string][]*ipRule),
		globalWhitelist: []*ipRule{},
		globalBlacklist: []*ipRule{},
	}

	// Load rules from database
	if err := filter.ReloadRules(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to load IP rules: %w", err)
	}

	return filter, nil
}

// CheckIP checks if an IP address is allowed based on whitelist/blacklist rules
func (ipf *ipFilter) CheckIP(ctx context.Context, ip string, tenantID string) (allowed bool, err error) {
	// Parse IP address
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false, fmt.Errorf("invalid IP address: %s", ip)
	}

	ipf.mu.RLock()
	defer ipf.mu.RUnlock()

	// Check blacklist first (blacklist takes precedence)
	// Check global blacklist
	for _, rule := range ipf.globalBlacklist {
		if rule.ipNet.Contains(parsedIP) {
			return false, nil
		}
	}

	// Check tenant-specific blacklist
	if tenantID != "" {
		if rules, exists := ipf.blacklistRules[tenantID]; exists {
			for _, rule := range rules {
				if rule.ipNet.Contains(parsedIP) {
					return false, nil
				}
			}
		}
	}

	// Check whitelist
	// If there are any whitelist rules, IP must be in whitelist
	hasWhitelistRules := len(ipf.globalWhitelist) > 0
	if tenantID != "" {
		if rules, exists := ipf.whitelistRules[tenantID]; exists && len(rules) > 0 {
			hasWhitelistRules = true
		}
	}

	if !hasWhitelistRules {
		// No whitelist rules, allow by default
		return true, nil
	}

	// Check global whitelist
	for _, rule := range ipf.globalWhitelist {
		if rule.ipNet.Contains(parsedIP) {
			return true, nil
		}
	}

	// Check tenant-specific whitelist
	if tenantID != "" {
		if rules, exists := ipf.whitelistRules[tenantID]; exists {
			for _, rule := range rules {
				if rule.ipNet.Contains(parsedIP) {
					return true, nil
				}
			}
		}
	}

	// Not in whitelist, deny
	return false, nil
}

// AddWhitelist adds an IP address or CIDR range to the whitelist
func (ipf *ipFilter) AddWhitelist(ctx context.Context, rule database.IPRule) error {
	rule.RuleType = "whitelist"
	return ipf.addRule(ctx, rule)
}

// AddBlacklist adds an IP address or CIDR range to the blacklist
func (ipf *ipFilter) AddBlacklist(ctx context.Context, rule database.IPRule) error {
	rule.RuleType = "blacklist"
	return ipf.addRule(ctx, rule)
}

// addRule adds an IP rule to the database and cache
func (ipf *ipFilter) addRule(ctx context.Context, rule database.IPRule) error {
	// Generate ID if not provided
	if rule.ID == "" {
		rule.ID = uuid.New().String()
	}

	// Set timestamp
	rule.CreatedAt = time.Now()

	// Validate rule
	if err := rule.Validate(); err != nil {
		return fmt.Errorf("invalid IP rule: %w", err)
	}

	// Parse and validate IP/CIDR
	_, ipNet, err := net.ParseCIDR(rule.IPAddress)
	if err != nil {
		// Try parsing as single IP
		ip := net.ParseIP(rule.IPAddress)
		if ip == nil {
			return fmt.Errorf("invalid IP address or CIDR: %s", rule.IPAddress)
		}
		// Convert single IP to CIDR
		if ip.To4() != nil {
			rule.IPAddress = rule.IPAddress + "/32"
		} else {
			rule.IPAddress = rule.IPAddress + "/128"
		}
		_, ipNet, _ = net.ParseCIDR(rule.IPAddress)
	}

	// Insert rule into database
	query := `
		INSERT INTO ip_rules (id, tenant_id, rule_type, ip_address, description, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`
	_, err = ipf.db.ExecContext(ctx, query,
		rule.ID, rule.TenantID, rule.RuleType, rule.IPAddress, rule.Description, rule.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to add IP rule: %w", err)
	}

	// Add to cache
	ipf.mu.Lock()
	defer ipf.mu.Unlock()

	parsedRule := &ipRule{
		id:          rule.ID,
		tenantID:    rule.TenantID,
		ruleType:    rule.RuleType,
		ipNet:       ipNet,
		description: rule.Description,
	}

	if rule.RuleType == "whitelist" {
		if rule.TenantID == "" {
			ipf.globalWhitelist = append(ipf.globalWhitelist, parsedRule)
		} else {
			ipf.whitelistRules[rule.TenantID] = append(ipf.whitelistRules[rule.TenantID], parsedRule)
		}
	} else {
		if rule.TenantID == "" {
			ipf.globalBlacklist = append(ipf.globalBlacklist, parsedRule)
		} else {
			ipf.blacklistRules[rule.TenantID] = append(ipf.blacklistRules[rule.TenantID], parsedRule)
		}
	}

	return nil
}

// RemoveRule removes an IP rule
func (ipf *ipFilter) RemoveRule(ctx context.Context, ruleID string) error {
	// Delete from database
	query := `DELETE FROM ip_rules WHERE id = ?`
	result, err := ipf.db.ExecContext(ctx, query, ruleID)
	if err != nil {
		return fmt.Errorf("failed to remove IP rule: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("IP rule not found: %s", ruleID)
	}

	// Reload rules to update cache
	return ipf.ReloadRules(ctx)
}

// ListRules lists all IP rules for a tenant
func (ipf *ipFilter) ListRules(ctx context.Context, tenantID string) ([]database.IPRule, error) {
	query := `
		SELECT id, tenant_id, rule_type, ip_address, description, created_at
		FROM ip_rules
		WHERE tenant_id = ? OR tenant_id IS NULL
		ORDER BY created_at DESC
	`
	rows, err := ipf.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list IP rules: %w", err)
	}
	defer rows.Close()

	var rules []database.IPRule
	for rows.Next() {
		rule := database.IPRule{}
		var tenantIDNull sql.NullString

		err := rows.Scan(
			&rule.ID, &tenantIDNull, &rule.RuleType, &rule.IPAddress,
			&rule.Description, &rule.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan IP rule: %w", err)
		}

		if tenantIDNull.Valid {
			rule.TenantID = tenantIDNull.String
		}

		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating IP rules: %w", err)
	}

	return rules, nil
}

// ReloadRules reloads all IP rules from the database
func (ipf *ipFilter) ReloadRules(ctx context.Context) error {
	query := `
		SELECT id, tenant_id, rule_type, ip_address, description, created_at
		FROM ip_rules
		ORDER BY created_at
	`
	rows, err := ipf.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to load IP rules: %w", err)
	}
	defer rows.Close()

	// Create new rule maps
	whitelistRules := make(map[string][]*ipRule)
	blacklistRules := make(map[string][]*ipRule)
	globalWhitelist := []*ipRule{}
	globalBlacklist := []*ipRule{}

	for rows.Next() {
		var id, ruleType, ipAddress, description string
		var tenantIDNull sql.NullString
		var createdAt time.Time

		err := rows.Scan(&id, &tenantIDNull, &ruleType, &ipAddress, &description, &createdAt)
		if err != nil {
			return fmt.Errorf("failed to scan IP rule: %w", err)
		}

		tenantID := ""
		if tenantIDNull.Valid {
			tenantID = tenantIDNull.String
		}

		// Parse CIDR
		_, ipNet, err := net.ParseCIDR(ipAddress)
		if err != nil {
			// Skip invalid rules
			continue
		}

		parsedRule := &ipRule{
			id:          id,
			tenantID:    tenantID,
			ruleType:    ruleType,
			ipNet:       ipNet,
			description: description,
		}

		if ruleType == "whitelist" {
			if tenantID == "" {
				globalWhitelist = append(globalWhitelist, parsedRule)
			} else {
				whitelistRules[tenantID] = append(whitelistRules[tenantID], parsedRule)
			}
		} else {
			if tenantID == "" {
				globalBlacklist = append(globalBlacklist, parsedRule)
			} else {
				blacklistRules[tenantID] = append(blacklistRules[tenantID], parsedRule)
			}
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating IP rules: %w", err)
	}

	// Update cache
	ipf.mu.Lock()
	defer ipf.mu.Unlock()

	ipf.whitelistRules = whitelistRules
	ipf.blacklistRules = blacklistRules
	ipf.globalWhitelist = globalWhitelist
	ipf.globalBlacklist = globalBlacklist

	return nil
}
