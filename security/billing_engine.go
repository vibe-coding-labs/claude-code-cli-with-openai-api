package security

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Usage represents resource usage for billing
type Usage struct {
	TenantID         string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	RequestCount     int
	Model            string
	Timestamp        time.Time
}

// PricingTier defines pricing for a tenant
type PricingTier struct {
	ID                    string
	Name                  string
	PromptTokenPrice      float64 // Price per 1000 prompt tokens
	CompletionTokenPrice  float64 // Price per 1000 completion tokens
	RequestPrice          float64 // Price per request
	VolumeDiscountEnabled bool
	VolumeDiscountTiers   []VolumeDiscountTier
}

// VolumeDiscountTier defines volume-based discounts
type VolumeDiscountTier struct {
	MinTokens int
	Discount  float64 // Percentage discount (0.0 - 1.0)
}

// TimePeriod defines a time range for billing
type TimePeriod struct {
	Start time.Time
	End   time.Time
}

// BillingReport contains billing information for a tenant
type BillingReport struct {
	TenantID          string
	TenantName        string
	Period            TimePeriod
	TotalRequests     int
	TotalPromptTokens int
	TotalCompTokens   int
	TotalTokens       int
	PromptTokenCost   float64
	CompTokenCost     float64
	RequestCost       float64
	TotalCost         float64
	PricingTier       string
	GeneratedAt       time.Time
	LineItems         []BillingLineItem
}

// BillingLineItem represents a single billing entry
type BillingLineItem struct {
	Date              time.Time
	Model             string
	Requests          int
	PromptTokens      int
	CompletionTokens  int
	Cost              float64
}

// ExportFormat defines the export format for billing reports
type ExportFormat string

const (
	ExportFormatJSON ExportFormat = "json"
	ExportFormatCSV  ExportFormat = "csv"
)

// BillingEngine interface for cost calculation and reporting
type BillingEngine interface {
	CalculateCost(usage Usage, pricingTier PricingTier) (cost float64, err error)
	GenerateReport(ctx context.Context, tenantID string, period TimePeriod) (*BillingReport, error)
	ExportReport(ctx context.Context, report *BillingReport, format ExportFormat) ([]byte, error)
	SetPricingTier(ctx context.Context, tenantID string, tier PricingTier) error
	GetPricingTier(ctx context.Context, tenantID string) (*PricingTier, error)
}

// billingEngine implements BillingEngine interface
type billingEngine struct {
	db *sql.DB
}

// BillingEngineConfig holds configuration for billing engine
type BillingEngineConfig struct {
	DB *sql.DB
}

// NewBillingEngine creates a new billing engine instance
func NewBillingEngine(config BillingEngineConfig) BillingEngine {
	return &billingEngine{
		db: config.DB,
	}
}

// CalculateCost calculates the cost for given usage and pricing tier
func (b *billingEngine) CalculateCost(usage Usage, pricingTier PricingTier) (float64, error) {
	// Calculate base costs
	promptCost := float64(usage.PromptTokens) / 1000.0 * pricingTier.PromptTokenPrice
	completionCost := float64(usage.CompletionTokens) / 1000.0 * pricingTier.CompletionTokenPrice
	requestCost := float64(usage.RequestCount) * pricingTier.RequestPrice

	totalCost := promptCost + completionCost + requestCost

	// Apply volume discount if enabled
	if pricingTier.VolumeDiscountEnabled && len(pricingTier.VolumeDiscountTiers) > 0 {
		discount := b.calculateVolumeDiscount(usage.TotalTokens, pricingTier.VolumeDiscountTiers)
		totalCost = totalCost * (1.0 - discount)
	}

	// Round to 4 decimal places
	totalCost = roundToDecimal(totalCost, 4)

	return totalCost, nil
}

// GenerateReport generates a billing report for a tenant and time period
func (b *billingEngine) GenerateReport(ctx context.Context, tenantID string, period TimePeriod) (*BillingReport, error) {
	// Get tenant information
	var tenantName string
	err := b.db.QueryRowContext(ctx, "SELECT name FROM tenants WHERE id = $1", tenantID).Scan(&tenantName)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	// Get pricing tier
	pricingTier, err := b.GetPricingTier(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pricing tier: %w", err)
	}

	// Query usage records
	query := `
		SELECT 
			DATE(timestamp) as date,
			model,
			COUNT(*) as requests,
			SUM(prompt_tokens) as prompt_tokens,
			SUM(completion_tokens) as completion_tokens
		FROM usage_records
		WHERE tenant_id = $1 AND timestamp >= $2 AND timestamp < $3
		GROUP BY DATE(timestamp), model
		ORDER BY date, model
	`

	rows, err := b.db.QueryContext(ctx, query, tenantID, period.Start, period.End)
	if err != nil {
		return nil, fmt.Errorf("failed to query usage records: %w", err)
	}
	defer rows.Close()

	report := &BillingReport{
		TenantID:    tenantID,
		TenantName:  tenantName,
		Period:      period,
		PricingTier: pricingTier.Name,
		GeneratedAt: time.Now(),
		LineItems:   []BillingLineItem{},
	}

	// Process each line item
	for rows.Next() {
		var lineItem BillingLineItem
		var dateStr string

		err := rows.Scan(
			&dateStr,
			&lineItem.Model,
			&lineItem.Requests,
			&lineItem.PromptTokens,
			&lineItem.CompletionTokens,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan usage record: %w", err)
		}

		// Parse date
		lineItem.Date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse date: %w", err)
		}

		// Calculate cost for this line item
		usage := Usage{
			PromptTokens:     lineItem.PromptTokens,
			CompletionTokens: lineItem.CompletionTokens,
			TotalTokens:      lineItem.PromptTokens + lineItem.CompletionTokens,
			RequestCount:     lineItem.Requests,
		}

		lineItem.Cost, err = b.CalculateCost(usage, *pricingTier)
		if err != nil {
			return nil, fmt.Errorf("failed to calculate cost: %w", err)
		}

		report.LineItems = append(report.LineItems, lineItem)

		// Accumulate totals
		report.TotalRequests += lineItem.Requests
		report.TotalPromptTokens += lineItem.PromptTokens
		report.TotalCompTokens += lineItem.CompletionTokens
		report.TotalCost += lineItem.Cost
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating usage records: %w", err)
	}

	// Calculate total tokens
	report.TotalTokens = report.TotalPromptTokens + report.TotalCompTokens

	// Calculate component costs
	report.PromptTokenCost = float64(report.TotalPromptTokens) / 1000.0 * pricingTier.PromptTokenPrice
	report.CompTokenCost = float64(report.TotalCompTokens) / 1000.0 * pricingTier.CompletionTokenPrice
	report.RequestCost = float64(report.TotalRequests) * pricingTier.RequestPrice

	// Round all costs
	report.PromptTokenCost = roundToDecimal(report.PromptTokenCost, 4)
	report.CompTokenCost = roundToDecimal(report.CompTokenCost, 4)
	report.RequestCost = roundToDecimal(report.RequestCost, 4)
	report.TotalCost = roundToDecimal(report.TotalCost, 4)

	return report, nil
}

// ExportReport exports a billing report in the specified format
func (b *billingEngine) ExportReport(ctx context.Context, report *BillingReport, format ExportFormat) ([]byte, error) {
	switch format {
	case ExportFormatJSON:
		return b.exportJSON(report)
	case ExportFormatCSV:
		return b.exportCSV(report)
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}

// SetPricingTier sets the pricing tier for a tenant
func (b *billingEngine) SetPricingTier(ctx context.Context, tenantID string, tier PricingTier) error {
	// Serialize volume discount tiers
	discountTiersJSON, err := json.Marshal(tier.VolumeDiscountTiers)
	if err != nil {
		return fmt.Errorf("failed to marshal volume discount tiers: %w", err)
	}

	query := `
		INSERT INTO pricing_tiers (tenant_id, name, prompt_token_price, completion_token_price, request_price, volume_discount_enabled, volume_discount_tiers)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (tenant_id) DO UPDATE SET
			name = EXCLUDED.name,
			prompt_token_price = EXCLUDED.prompt_token_price,
			completion_token_price = EXCLUDED.completion_token_price,
			request_price = EXCLUDED.request_price,
			volume_discount_enabled = EXCLUDED.volume_discount_enabled,
			volume_discount_tiers = EXCLUDED.volume_discount_tiers,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err = b.db.ExecContext(ctx, query,
		tenantID,
		tier.Name,
		tier.PromptTokenPrice,
		tier.CompletionTokenPrice,
		tier.RequestPrice,
		tier.VolumeDiscountEnabled,
		string(discountTiersJSON),
	)

	if err != nil {
		return fmt.Errorf("failed to set pricing tier: %w", err)
	}

	return nil
}

// GetPricingTier retrieves the pricing tier for a tenant
func (b *billingEngine) GetPricingTier(ctx context.Context, tenantID string) (*PricingTier, error) {
	query := `
		SELECT name, prompt_token_price, completion_token_price, request_price, volume_discount_enabled, volume_discount_tiers
		FROM pricing_tiers
		WHERE tenant_id = $1
	`

	tier := &PricingTier{ID: tenantID}
	var discountTiersJSON string

	err := b.db.QueryRowContext(ctx, query, tenantID).Scan(
		&tier.Name,
		&tier.PromptTokenPrice,
		&tier.CompletionTokenPrice,
		&tier.RequestPrice,
		&tier.VolumeDiscountEnabled,
		&discountTiersJSON,
	)

	if err == sql.ErrNoRows {
		// Return default pricing tier
		return b.getDefaultPricingTier(), nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get pricing tier: %w", err)
	}

	// Deserialize volume discount tiers
	if discountTiersJSON != "" {
		err = json.Unmarshal([]byte(discountTiersJSON), &tier.VolumeDiscountTiers)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal volume discount tiers: %w", err)
		}
	}

	return tier, nil
}

// calculateVolumeDiscount calculates the discount based on volume
func (b *billingEngine) calculateVolumeDiscount(totalTokens int, tiers []VolumeDiscountTier) float64 {
	var discount float64

	for _, tier := range tiers {
		if totalTokens >= tier.MinTokens {
			discount = tier.Discount
		}
	}

	return discount
}

// getDefaultPricingTier returns the default pricing tier
func (b *billingEngine) getDefaultPricingTier() *PricingTier {
	return &PricingTier{
		ID:                   "default",
		Name:                 "Standard",
		PromptTokenPrice:     0.01,   // $0.01 per 1K prompt tokens
		CompletionTokenPrice: 0.03,   // $0.03 per 1K completion tokens
		RequestPrice:         0.0001, // $0.0001 per request
		VolumeDiscountEnabled: false,
		VolumeDiscountTiers:   []VolumeDiscountTier{},
	}
}

// exportJSON exports the report as JSON
func (b *billingEngine) exportJSON(report *BillingReport) ([]byte, error) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal report to JSON: %w", err)
	}
	return data, nil
}

// exportCSV exports the report as CSV
func (b *billingEngine) exportCSV(report *BillingReport) ([]byte, error) {
	var buf strings.Builder
	writer := csv.NewWriter(&buf)

	// Write header
	header := []string{
		"Tenant ID",
		"Tenant Name",
		"Period Start",
		"Period End",
		"Date",
		"Model",
		"Requests",
		"Prompt Tokens",
		"Completion Tokens",
		"Cost",
	}
	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write line items
	for _, item := range report.LineItems {
		row := []string{
			report.TenantID,
			report.TenantName,
			report.Period.Start.Format("2006-01-02"),
			report.Period.End.Format("2006-01-02"),
			item.Date.Format("2006-01-02"),
			item.Model,
			fmt.Sprintf("%d", item.Requests),
			fmt.Sprintf("%d", item.PromptTokens),
			fmt.Sprintf("%d", item.CompletionTokens),
			fmt.Sprintf("%.4f", item.Cost),
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	// Write summary row
	summaryRow := []string{
		"",
		"",
		"",
		"",
		"TOTAL",
		"",
		fmt.Sprintf("%d", report.TotalRequests),
		fmt.Sprintf("%d", report.TotalPromptTokens),
		fmt.Sprintf("%d", report.TotalCompTokens),
		fmt.Sprintf("%.4f", report.TotalCost),
	}
	if err := writer.Write(summaryRow); err != nil {
		return nil, fmt.Errorf("failed to write CSV summary: %w", err)
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("CSV writer error: %w", err)
	}

	return []byte(buf.String()), nil
}

// roundToDecimal rounds a float to the specified number of decimal places
func roundToDecimal(value float64, decimals int) float64 {
	multiplier := 1.0
	for i := 0; i < decimals; i++ {
		multiplier *= 10
	}
	return float64(int(value*multiplier+0.5)) / multiplier
}
