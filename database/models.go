package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
)

// CreateAPIConfig creates a new API configuration
func CreateAPIConfig(config *APIConfig) error {
	if config.ID == "" {
		config.ID = uuid.New().String()
	}

	// Set Anthropic API Key if not provided
	if config.AnthropicAPIKey == "" {
		config.AnthropicAPIKey = uuid.New().String()
	} else {
		// Validate custom token
		if len(config.AnthropicAPIKey) > 100 {
			return fmt.Errorf("custom API token must be 100 characters or less")
		}

		for _, ch := range config.AnthropicAPIKey {
			if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
				(ch >= '0' && ch <= '9') || ch == '_') {
				return fmt.Errorf("custom API token can only contain letters, numbers, and underscores")
			}
		}

		// Check uniqueness
		var count int
		checkQuery := `SELECT COUNT(*) FROM api_configs WHERE anthropic_api_key = ?`
		err := DB.QueryRow(checkQuery, config.AnthropicAPIKey).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check token uniqueness: %w", err)
		}
		if count > 0 {
			return fmt.Errorf("this API token is already in use")
		}
	}

	// Encrypt API key
	encrypted, err := EncryptAPIKey(config.OpenAIAPIKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt API key: %w", err)
	}

	// Serialize supported_models to JSON
	var supportedModelsJSON []byte
	if len(config.SupportedModels) > 0 {
		supportedModelsJSON, err = json.Marshal(config.SupportedModels)
		if err != nil {
			return fmt.Errorf("failed to marshal supported models: %w", err)
		}
	}

	// Set default retry count if not provided
	if config.RetryCount <= 0 {
		config.RetryCount = 3
	}
	if config.RetryCount > 100 {
		config.RetryCount = 100
	}

	query := `
		INSERT INTO api_configs (
			id, name, description, openai_api_key_encrypted, openai_base_url,
			big_model, middle_model, small_model, supported_models, max_tokens_limit, request_timeout, retry_count,
			anthropic_api_key, enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'))
	`

	_, err = DB.Exec(query,
		config.ID, config.Name, config.Description, encrypted, config.OpenAIBaseURL,
		config.BigModel, config.MiddleModel, config.SmallModel, string(supportedModelsJSON), config.MaxTokensLimit,
		config.RequestTimeout, config.RetryCount, config.AnthropicAPIKey, config.Enabled,
	)

	if err != nil {
		return fmt.Errorf("failed to insert config: %w", err)
	}

	return nil
}

// GetAPIConfig retrieves an API configuration by ID
func GetAPIConfig(id string) (*APIConfig, error) {
	query := `
		SELECT id, name, description, openai_api_key_encrypted, openai_base_url,
			big_model, middle_model, small_model, supported_models, max_tokens_limit, request_timeout, retry_count,
			anthropic_api_key, enabled, created_at, updated_at
		FROM api_configs WHERE id = ?
	`

	config := &APIConfig{}
	var supportedModelsJSON sql.NullString
	err := DB.QueryRow(query, id).Scan(
		&config.ID, &config.Name, &config.Description, &config.OpenAIAPIKeyEncrypted,
		&config.OpenAIBaseURL, &config.BigModel, &config.MiddleModel, &config.SmallModel,
		&supportedModelsJSON, &config.MaxTokensLimit, &config.RequestTimeout, &config.RetryCount, &config.AnthropicAPIKey,
		&config.Enabled, &config.CreatedAt, &config.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("config not found")
		}
		return nil, fmt.Errorf("failed to query config: %w", err)
	}

	// Deserialize supported_models from JSON
	if supportedModelsJSON.Valid && supportedModelsJSON.String != "" {
		err = json.Unmarshal([]byte(supportedModelsJSON.String), &config.SupportedModels)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal supported models: %w", err)
		}
	}

	// Decrypt API key
	decrypted, err := DecryptAPIKey(config.OpenAIAPIKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt API key: %w", err)
	}
	config.OpenAIAPIKey = decrypted
	config.OpenAIAPIKeyMasked = MaskAPIKey(decrypted)

	return config, nil
}

// GetConfigByAnthropicAPIKey retrieves an API configuration by Anthropic API key
func GetConfigByAnthropicAPIKey(apiKey string) (*APIConfig, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("empty API key")
	}

	query := `
		SELECT id, name, description, openai_api_key_encrypted, openai_base_url,
			big_model, middle_model, small_model, supported_models, max_tokens_limit, request_timeout, retry_count,
			anthropic_api_key, enabled, created_at, updated_at
		FROM api_configs 
		WHERE anthropic_api_key = ? AND enabled = 1
		LIMIT 1
	`

	config := &APIConfig{}
	var supportedModelsJSON sql.NullString
	err := DB.QueryRow(query, apiKey).Scan(
		&config.ID, &config.Name, &config.Description, &config.OpenAIAPIKeyEncrypted,
		&config.OpenAIBaseURL, &config.BigModel, &config.MiddleModel, &config.SmallModel,
		&supportedModelsJSON, &config.MaxTokensLimit, &config.RequestTimeout, &config.RetryCount, &config.AnthropicAPIKey,
		&config.Enabled, &config.CreatedAt, &config.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("config not found for API key")
		}
		return nil, fmt.Errorf("failed to query config: %w", err)
	}

	// Deserialize supported_models from JSON
	if supportedModelsJSON.Valid && supportedModelsJSON.String != "" {
		err = json.Unmarshal([]byte(supportedModelsJSON.String), &config.SupportedModels)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal supported models: %w", err)
		}
	}

	// Decrypt API key
	decrypted, err := DecryptAPIKey(config.OpenAIAPIKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt API key: %w", err)
	}
	config.OpenAIAPIKey = decrypted
	config.OpenAIAPIKeyMasked = MaskAPIKey(decrypted)

	return config, nil
}

// RenewAnthropicAPIKey generates a new Anthropic API key for a config
// customToken: optional custom token (max 100 chars, alphanumeric + underscore only)
func RenewAnthropicAPIKey(configID string, customToken string) (string, error) {
	var newAPIKey string

	if customToken != "" {
		// Validate custom token
		if len(customToken) > 100 {
			return "", fmt.Errorf("custom token must be 100 characters or less")
		}

		// Validate characters (alphanumeric + underscore only)
		for _, ch := range customToken {
			if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
				(ch >= '0' && ch <= '9') || ch == '_') {
				return "", fmt.Errorf("custom token can only contain letters, numbers, and underscores")
			}
		}

		// Check uniqueness (excluding current config)
		var count int
		checkQuery := `SELECT COUNT(*) FROM api_configs WHERE anthropic_api_key = ? AND id != ?`
		err := DB.QueryRow(checkQuery, customToken, configID).Scan(&count)
		if err != nil {
			return "", fmt.Errorf("failed to check token uniqueness: %w", err)
		}
		if count > 0 {
			return "", fmt.Errorf("this API token is already in use by another configuration")
		}

		newAPIKey = customToken
	} else {
		// Generate UUID
		newAPIKey = uuid.New().String()
	}

	query := `
		UPDATE api_configs 
		SET anthropic_api_key = ?, updated_at = datetime('now')
		WHERE id = ?
	`

	_, err := DB.Exec(query, newAPIKey, configID)
	if err != nil {
		return "", fmt.Errorf("failed to renew API key: %w", err)
	}

	return newAPIKey, nil
}

// GetAllAPIConfigs retrieves all API configurations
func GetAllAPIConfigs() ([]*APIConfig, error) {
	query := `
		SELECT id, name, description, openai_api_key_encrypted, openai_base_url,
			big_model, middle_model, small_model, supported_models, max_tokens_limit, request_timeout, retry_count,
			anthropic_api_key, enabled, created_at, updated_at
		FROM api_configs ORDER BY created_at DESC
	`

	rows, err := DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query configs: %w", err)
	}
	defer rows.Close()

	var configs []*APIConfig
	for rows.Next() {
		config := &APIConfig{}
		var supportedModelsJSON sql.NullString
		err := rows.Scan(
			&config.ID, &config.Name, &config.Description, &config.OpenAIAPIKeyEncrypted,
			&config.OpenAIBaseURL, &config.BigModel, &config.MiddleModel, &config.SmallModel,
			&supportedModelsJSON, &config.MaxTokensLimit, &config.RequestTimeout, &config.RetryCount, &config.AnthropicAPIKey,
			&config.Enabled, &config.CreatedAt, &config.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan config: %w", err)
		}

		// Deserialize supported_models from JSON
		if supportedModelsJSON.Valid && supportedModelsJSON.String != "" {
			err = json.Unmarshal([]byte(supportedModelsJSON.String), &config.SupportedModels)
			if err != nil {
				// 忽略反序列化错误，继续处理
				config.SupportedModels = nil
			}
		}

		// Decrypt and mask API key
		decrypted, err := DecryptAPIKey(config.OpenAIAPIKeyEncrypted)
		if err != nil {
			config.OpenAIAPIKeyMasked = "****"
		} else {
			config.OpenAIAPIKeyMasked = MaskAPIKey(decrypted)
		}

		// Don't include decrypted key in list view
		config.OpenAIAPIKey = ""

		configs = append(configs, config)
	}

	return configs, nil
}

// UpdateAPIConfig updates an existing API configuration
func UpdateAPIConfig(config *APIConfig) error {
	// Encrypt API key if provided
	var encrypted string
	if config.OpenAIAPIKey != "" {
		var err error
		encrypted, err = EncryptAPIKey(config.OpenAIAPIKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt API key: %w", err)
		}
	} else {
		// Keep existing encrypted key
		existing, err := GetAPIConfig(config.ID)
		if err != nil {
			return err
		}
		encrypted = existing.OpenAIAPIKeyEncrypted
	}

	// Serialize supported_models to JSON
	var supportedModelsJSON []byte
	var err error
	if len(config.SupportedModels) > 0 {
		supportedModelsJSON, err = json.Marshal(config.SupportedModels)
		if err != nil {
			return fmt.Errorf("failed to marshal supported models: %w", err)
		}
	}

	// Validate retry count
	if config.RetryCount <= 0 {
		config.RetryCount = 3
	}
	if config.RetryCount > 100 {
		config.RetryCount = 100
	}

	query := `
		UPDATE api_configs SET
			name = ?, description = ?, openai_api_key_encrypted = ?, openai_base_url = ?,
			big_model = ?, middle_model = ?, small_model = ?, supported_models = ?, max_tokens_limit = ?,
			request_timeout = ?, retry_count = ?, anthropic_api_key = ?, enabled = ?, updated_at = datetime('now')
		WHERE id = ?
	`

	_, err = DB.Exec(query,
		config.Name, config.Description, encrypted, config.OpenAIBaseURL,
		config.BigModel, config.MiddleModel, config.SmallModel, string(supportedModelsJSON), config.MaxTokensLimit,
		config.RequestTimeout, config.RetryCount, config.AnthropicAPIKey, config.Enabled, config.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	return nil
}

// DeleteAPIConfig deletes an API configuration
func DeleteAPIConfig(id string) error {
	_, err := DB.Exec("DELETE FROM api_configs WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete config: %w", err)
	}
	return nil
}

// LogRequest logs an API request
func LogRequest(log *RequestLog) error {
	query := `
		INSERT INTO request_logs (
			config_id, model, input_tokens, output_tokens, total_tokens,
			duration_ms, status, error_message, request_body, response_body,
			request_summary, response_preview, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`

	_, err := DB.Exec(query,
		log.ConfigID, log.Model, log.InputTokens, log.OutputTokens, log.TotalTokens,
		log.DurationMs, log.Status, log.ErrorMessage, log.RequestBody, log.ResponseBody,
		log.RequestSummary, log.ResponsePreview,
	)

	if err != nil {
		return fmt.Errorf("failed to log request: %w", err)
	}

	// Update aggregated statistics
	return updateTokenStats(log)
}

// updateTokenStats updates the aggregated token statistics
func updateTokenStats(log *RequestLog) error {
	// Check if a stats record exists for today
	query := `
		SELECT id FROM token_stats
		WHERE config_id = ? AND model = ? AND DATE(created_at) = DATE('now')
	`

	var id int64
	err := DB.QueryRow(query, log.ConfigID, log.Model).Scan(&id)

	errorCount := 0
	if log.Status == "error" {
		errorCount = 1
	}

	if err == sql.ErrNoRows {
		// Create new stats record
		insertQuery := `
			INSERT INTO token_stats (
				config_id, model, input_tokens, output_tokens, total_tokens,
				request_count, error_count, created_at
			) VALUES (?, ?, ?, ?, ?, 1, ?, datetime('now'))
		`
		_, err = DB.Exec(insertQuery,
			log.ConfigID, log.Model, log.InputTokens, log.OutputTokens, log.TotalTokens, errorCount,
		)
	} else if err == nil {
		// Update existing stats record
		updateQuery := `
			UPDATE token_stats SET
				input_tokens = input_tokens + ?,
				output_tokens = output_tokens + ?,
				total_tokens = total_tokens + ?,
				request_count = request_count + 1,
				error_count = error_count + ?
			WHERE id = ?
		`
		_, err = DB.Exec(updateQuery,
			log.InputTokens, log.OutputTokens, log.TotalTokens, errorCount, id,
		)
	}

	return err
}

// GetConfigStats retrieves aggregated statistics for a config
func GetConfigStats(configID string, days int) (*ConfigStats, error) {
	query := `
		SELECT
			COUNT(*) as total_requests,
			SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success_requests,
			SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) as error_requests,
			COALESCE(SUM(input_tokens), 0) as total_input_tokens,
			COALESCE(SUM(output_tokens), 0) as total_output_tokens,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(AVG(duration_ms), 0) as avg_duration_ms
		FROM request_logs
		WHERE config_id = ? AND created_at >= datetime('now', '-' || ? || ' days')
	`

	stats := &ConfigStats{ConfigID: configID}
	err := DB.QueryRow(query, configID, days).Scan(
		&stats.TotalRequests, &stats.SuccessRequests, &stats.ErrorRequests,
		&stats.TotalInputTokens, &stats.TotalOutputTokens, &stats.TotalTokens,
		&stats.AvgDurationMs,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	return stats, nil
}

// GetRecentLogs retrieves recent request logs for a config
func GetRecentLogs(configID string, limit int) ([]*RequestLog, error) {
	query := `
		SELECT id, config_id, model, input_tokens, output_tokens, total_tokens,
			duration_ms, status, error_message, request_body, response_body,
			request_summary, response_preview, created_at
		FROM request_logs
		WHERE config_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := DB.Query(query, configID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query logs: %w", err)
	}
	defer rows.Close()

	var logs []*RequestLog
	for rows.Next() {
		log := &RequestLog{}
		var requestBody, responseBody, requestSummary, responsePreview sql.NullString
		err := rows.Scan(
			&log.ID, &log.ConfigID, &log.Model, &log.InputTokens, &log.OutputTokens,
			&log.TotalTokens, &log.DurationMs, &log.Status, &log.ErrorMessage,
			&requestBody, &responseBody, &requestSummary, &responsePreview, &log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan log: %w", err)
		}

		// 处理可空字段
		if requestBody.Valid {
			log.RequestBody = requestBody.String
		}
		if responseBody.Valid {
			log.ResponseBody = responseBody.String
		}
		if requestSummary.Valid {
			log.RequestSummary = requestSummary.String
		}
		if responsePreview.Valid {
			log.ResponsePreview = responsePreview.String
		}

		logs = append(logs, log)
	}

	return logs, nil
}

// ToConfig converts database APIConfig to config.Config
func (a *APIConfig) ToConfig() *config.Config {
	return &config.Config{
		OpenAIAPIKey:    a.OpenAIAPIKey,
		OpenAIBaseURL:   a.OpenAIBaseURL,
		BigModel:        a.BigModel,
		MiddleModel:     a.MiddleModel,
		SmallModel:      a.SmallModel,
		MaxTokensLimit:  a.MaxTokensLimit,
		RequestTimeout:  a.RequestTimeout,
		RetryCount:      a.RetryCount,
		AnthropicAPIKey: a.AnthropicAPIKey,
	}
}

// GetAllHistoricalModels retrieves all unique model names from configs and logs
// This includes models from:
// 1. Config model mappings (big_model, middle_model, small_model)
// 2. Config supported_models lists
// 3. Request logs
func GetAllHistoricalModels() ([]string, error) {
	modelSet := make(map[string]bool)
	var models []string

	// 1. Get models from config mappings
	configQuery := `
		SELECT DISTINCT model FROM (
			SELECT big_model as model FROM api_configs WHERE big_model IS NOT NULL AND big_model != ''
			UNION
			SELECT middle_model as model FROM api_configs WHERE middle_model IS NOT NULL AND middle_model != ''
			UNION
			SELECT small_model as model FROM api_configs WHERE small_model IS NOT NULL AND small_model != ''
		)
	`
	rows, err := DB.Query(configQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query config models: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var model string
		if err := rows.Scan(&model); err != nil {
			continue
		}
		if model != "" {
			modelSet[model] = true
		}
	}

	// 2. Get models from supported_models JSON field
	supportedQuery := `SELECT supported_models FROM api_configs WHERE supported_models IS NOT NULL AND supported_models != ''`
	rows, err = DB.Query(supportedQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var supportedModelsJSON string
			if err := rows.Scan(&supportedModelsJSON); err != nil {
				continue
			}
			// Parse JSON array
			if supportedModelsJSON != "" {
				// Simple JSON parsing for array of strings
				// Remove brackets and quotes, split by comma
				supportedModelsJSON = strings.Trim(supportedModelsJSON, "[]")
				supportedModelsJSON = strings.ReplaceAll(supportedModelsJSON, "\"", "")
				if supportedModelsJSON != "" {
					modelList := strings.Split(supportedModelsJSON, ",")
					for _, m := range modelList {
						m = strings.TrimSpace(m)
						if m != "" {
							modelSet[m] = true
						}
					}
				}
			}
		}
	}

	// 3. Get models from request logs
	logQuery := `SELECT DISTINCT model FROM request_logs WHERE model IS NOT NULL AND model != ''`
	rows, err = DB.Query(logQuery)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var model string
			if err := rows.Scan(&model); err != nil {
				continue
			}
			if model != "" {
				modelSet[model] = true
			}
		}
	}

	// Convert map to sorted slice
	for model := range modelSet {
		models = append(models, model)
	}

	// Sort models alphabetically
	sort.Strings(models)

	return models, nil
}
