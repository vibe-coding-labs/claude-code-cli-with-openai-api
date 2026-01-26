package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// LogsQueryParams represents query parameters for logs
type LogsQueryParams struct {
	ConfigID  string
	UserID    int64
	Status    string // "success", "error", or empty for all
	Model     string // filter by model
	SortBy    string // "created_at", "duration_ms", "total_tokens"
	SortOrder string // "asc" or "desc"
	Page      int    // page number (1-indexed)
	PageSize  int    // items per page
	Search    string // search in request_summary or response_preview
}

// GetUserLogsWithFilters retrieves logs for a user with filtering, sorting, and pagination
func GetUserLogsWithFilters(params UserLogsQueryParams) (*LogsResult, error) {
	whereClauses := []string{"user_id = ?"}
	args := []interface{}{params.UserID}

	if params.ConfigID != "" {
		whereClauses = append(whereClauses, "config_id = ?")
		args = append(args, params.ConfigID)
	}

	if params.Status != "" {
		whereClauses = append(whereClauses, "status = ?")
		args = append(args, params.Status)
	}

	if params.Model != "" {
		whereClauses = append(whereClauses, "model = ?")
		args = append(args, params.Model)
	}

	if params.Search != "" {
		whereClauses = append(whereClauses, "(request_summary LIKE ? OR response_preview LIKE ?)")
		searchPattern := "%" + params.Search + "%"
		args = append(args, searchPattern, searchPattern)
	}

	if params.StartTime != nil {
		whereClauses = append(whereClauses, "created_at >= ?")
		args = append(args, params.StartTime.Format("2006-01-02 15:04:05"))
	}
	if params.EndTime != nil {
		whereClauses = append(whereClauses, "created_at <= ?")
		args = append(args, params.EndTime.Format("2006-01-02 15:04:05"))
	}

	whereClause := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM request_logs WHERE %s", whereClause)
	var total int64
	err := DB.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("failed to count logs: %w", err)
	}

	sortBy := "created_at"
	if params.SortBy != "" {
		switch params.SortBy {
		case "created_at", "duration_ms", "total_tokens", "input_tokens", "output_tokens":
			sortBy = params.SortBy
		}
	}

	sortOrder := "DESC"
	if params.SortOrder == "asc" {
		sortOrder = "ASC"
	}

	page := params.Page
	if page < 1 {
		page = 1
	}

	pageSize := params.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	query := fmt.Sprintf(`
		SELECT id, config_id, user_id, model, input_tokens, output_tokens, total_tokens,
			duration_ms, status, error_message, request_body, response_body,
			request_summary, response_preview, created_at
		FROM request_logs
		WHERE %s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortOrder)

	args = append(args, pageSize, offset)

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query logs: %w", err)
	}
	defer rows.Close()

	var logs []*RequestLog
	for rows.Next() {
		log := &RequestLog{}
		var requestBody, responseBody, requestSummary, responsePreview, errorMessage sql.NullString
		err := rows.Scan(
			&log.ID, &log.ConfigID, &log.UserID, &log.Model, &log.InputTokens, &log.OutputTokens,
			&log.TotalTokens, &log.DurationMs, &log.Status, &errorMessage,
			&requestBody, &responseBody, &requestSummary, &responsePreview, &log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan log: %w", err)
		}

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
		if errorMessage.Valid {
			log.ErrorMessage = errorMessage.String
		}

		logs = append(logs, log)
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))

	return &LogsResult{
		Logs:       logs,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// LogsResult represents paginated logs result
type LogsResult struct {
	Logs       []*RequestLog `json:"logs"`
	Total      int64         `json:"total"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
	TotalPages int           `json:"total_pages"`
}

// UserLogsQueryParams represents query parameters for user logs
type UserLogsQueryParams struct {
	UserID    int64
	ConfigID  string
	Status    string // "success", "error", or empty for all
	Model     string // filter by model
	SortBy    string // "created_at", "duration_ms", "total_tokens"
	SortOrder string // "asc" or "desc"
	Page      int    // page number (1-indexed)
	PageSize  int    // items per page
	Search    string // search in request_summary or response_preview
	StartTime *time.Time
	EndTime   *time.Time
}

// GetLogsWithFilters retrieves logs with filtering, sorting, and pagination
func GetLogsWithFilters(params LogsQueryParams) (*LogsResult, error) {
	// Build WHERE clause
	whereClauses := []string{"config_id = ?"}
	args := []interface{}{params.ConfigID}

	if params.Status != "" {
		whereClauses = append(whereClauses, "status = ?")
		args = append(args, params.Status)
	}

	if params.UserID > 0 {
		whereClauses = append(whereClauses, "user_id = ?")
		args = append(args, params.UserID)
	}

	if params.Model != "" {
		whereClauses = append(whereClauses, "model = ?")
		args = append(args, params.Model)
	}

	if params.Search != "" {
		whereClauses = append(whereClauses, "(request_summary LIKE ? OR response_preview LIKE ?)")
		searchPattern := "%" + params.Search + "%"
		args = append(args, searchPattern, searchPattern)
	}

	whereClause := strings.Join(whereClauses, " AND ")

	// Get total count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM request_logs WHERE %s", whereClause)
	var total int64
	err := DB.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("failed to count logs: %w", err)
	}

	// Build ORDER BY clause
	sortBy := "created_at"
	if params.SortBy != "" {
		switch params.SortBy {
		case "created_at", "duration_ms", "total_tokens", "input_tokens", "output_tokens":
			sortBy = params.SortBy
		}
	}

	sortOrder := "DESC"
	if params.SortOrder == "asc" {
		sortOrder = "ASC"
	}

	// Calculate pagination
	page := params.Page
	if page < 1 {
		page = 1
	}

	pageSize := params.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := (page - 1) * pageSize

	// Build final query
	query := fmt.Sprintf(`
		SELECT id, config_id, user_id, model, input_tokens, output_tokens, total_tokens,
			duration_ms, status, error_message, request_body, response_body,
			request_summary, response_preview, created_at
		FROM request_logs
		WHERE %s
		ORDER BY %s %s
		LIMIT ? OFFSET ?
	`, whereClause, sortBy, sortOrder)

	args = append(args, pageSize, offset)

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query logs: %w", err)
	}
	defer rows.Close()

	var logs []*RequestLog
	for rows.Next() {
		log := &RequestLog{}
		var requestBody, responseBody, requestSummary, responsePreview, errorMessage sql.NullString
		err := rows.Scan(
			&log.ID, &log.ConfigID, &log.UserID, &log.Model, &log.InputTokens, &log.OutputTokens,
			&log.TotalTokens, &log.DurationMs, &log.Status, &errorMessage,
			&requestBody, &responseBody, &requestSummary, &responsePreview, &log.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan log: %w", err)
		}

		// Handle nullable fields
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
		if errorMessage.Valid {
			log.ErrorMessage = errorMessage.String
		}

		logs = append(logs, log)
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))

	return &LogsResult{
		Logs:       logs,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

// DeleteConfigLogs deletes all logs for a specific config
func DeleteConfigLogs(configID string) error {
	_, err := DB.Exec("DELETE FROM request_logs WHERE config_id = ?", configID)
	if err != nil {
		return fmt.Errorf("failed to delete logs: %w", err)
	}
	return nil
}

// GetLogByID retrieves a single log by ID
func GetLogByID(id int64) (*RequestLog, error) {
	log := &RequestLog{}
	var requestBody, responseBody, requestSummary, responsePreview, errorMessage sql.NullString

	query := `
		SELECT id, config_id, user_id, model, input_tokens, output_tokens, total_tokens,
			duration_ms, status, error_message, request_body, response_body,
			request_summary, response_preview, created_at
		FROM request_logs
		WHERE id = ?
	`

	err := DB.QueryRow(query, id).Scan(
		&log.ID, &log.ConfigID, &log.UserID, &log.Model, &log.InputTokens, &log.OutputTokens,
		&log.TotalTokens, &log.DurationMs, &log.Status, &errorMessage,
		&requestBody, &responseBody, &requestSummary, &responsePreview, &log.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("log not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get log: %w", err)
	}

	// Handle nullable fields
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
	if errorMessage.Valid {
		log.ErrorMessage = errorMessage.String
	}

	return log, nil
}

// GetAvailableModels returns list of unique models used in logs for a config
func GetAvailableModels(configID string) ([]string, error) {
	query := `
		SELECT DISTINCT model 
		FROM request_logs 
		WHERE config_id = ? 
		ORDER BY model
	`

	rows, err := DB.Query(query, configID)
	if err != nil {
		return nil, fmt.Errorf("failed to query models: %w", err)
	}
	defer rows.Close()

	var models []string
	for rows.Next() {
		var model string
		if err := rows.Scan(&model); err != nil {
			return nil, fmt.Errorf("failed to scan model: %w", err)
		}
		models = append(models, model)
	}

	return models, nil
}
