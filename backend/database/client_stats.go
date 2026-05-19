package database

import (
	"database/sql"
	"fmt"
	"time"
)

// GetClientStats returns statistics about active clients for a specific config
// Note: 通过并发请求检测来估算实际客户端数量
func GetClientStats(configID string) (*ClientStats, error) {
	stats := &ClientStats{
		ConfigID: configID,
		Clients:  []ActiveClient{},
	}

	// 获取最近24小时内的所有唯一客户端及其最后请求时间
	query := `
		SELECT 
			COALESCE(client_ip, 'unknown') as client_ip,
			COALESCE(user_agent, 'unknown') as user_agent,
			MAX(created_at) as last_request_at,
			COUNT(*) as request_count
		FROM request_logs
		WHERE config_id = ? 
			AND created_at >= datetime('now', '-24 hours')
			AND (client_ip IS NOT NULL OR user_agent IS NOT NULL)
		GROUP BY client_ip, user_agent
		ORDER BY last_request_at DESC
	`

	rows, err := DB.Query(query, configID)
	if err != nil {
		return nil, fmt.Errorf("failed to query client stats: %w", err)
	}
	defer rows.Close()

	now := time.Now()
	fiveMinutesAgo := now.Add(-5 * time.Minute)

	for rows.Next() {
		var client ActiveClient
		var lastRequestAtStr string

		err := rows.Scan(&client.ClientIP, &client.UserAgent, &lastRequestAtStr, &client.RequestCount)
		if err != nil {
			continue
		}

		// 解析时间
		client.LastRequestAt, err = time.Parse("2006-01-02 15:04:05", lastRequestAtStr)
		if err != nil {
			continue
		}

		// 判断是否活跃（最近5分钟内有请求）
		client.IsActive = client.LastRequestAt.After(fiveMinutesAgo)
		if client.IsActive {
			stats.ActiveClients++
		}

		stats.Clients = append(stats.Clients, client)
		stats.TotalClients++

		// 记录最新的请求时间
		if stats.LastRequestAt == nil || client.LastRequestAt.After(*stats.LastRequestAt) {
			stats.LastRequestAt = &client.LastRequestAt
		}
	}

	// 估算实际客户端数量（考虑并发情况）
	baseCount, estimatedCount, err := EstimateActualClients(configID)
	if err == nil {
		stats.EstimatedClients = estimatedCount
		stats.HasConcurrent = estimatedCount > baseCount

		// 如果估算值大于基础统计，使用估算值
		if estimatedCount > stats.ActiveClients {
			stats.ActiveClients = estimatedCount
		}
	} else {
		// 如果估算失败，使用基础统计
		stats.EstimatedClients = stats.ActiveClients
		stats.HasConcurrent = false
	}

	return stats, nil
}

// GetRecentClientActivity returns recent client activity for a config (last 5 minutes)
func GetRecentClientActivity(configID string) (bool, int, error) {
	query := `
		SELECT COUNT(DISTINCT client_ip || '_' || user_agent) as client_count
		FROM request_logs
		WHERE config_id = ?
			AND created_at >= datetime('now', '-5 minutes')
			AND (client_ip IS NOT NULL OR user_agent IS NOT NULL)
	`

	var clientCount int
	err := DB.QueryRow(query, configID).Scan(&clientCount)
	if err != nil && err != sql.ErrNoRows {
		return false, 0, fmt.Errorf("failed to get recent client activity: %w", err)
	}

	hasActivity := clientCount > 0
	return hasActivity, clientCount, nil
}

// GetAllActiveConfigs returns a map of config IDs to their active client counts
func GetAllActiveConfigs() (map[string]int, error) {
	query := `
		SELECT 
			config_id,
			COUNT(DISTINCT client_ip || '_' || user_agent) as client_count
		FROM request_logs
		WHERE created_at >= datetime('now', '-5 minutes')
			AND (client_ip IS NOT NULL OR user_agent IS NOT NULL)
		GROUP BY config_id
	`

	rows, err := DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query active configs: %w", err)
	}
	defer rows.Close()

	activeConfigs := make(map[string]int)
	for rows.Next() {
		var configID string
		var clientCount int
		if err := rows.Scan(&configID, &clientCount); err != nil {
			continue
		}
		activeConfigs[configID] = clientCount
	}

	return activeConfigs, nil
}

// GetConcurrentConnections 检测并发连接数来估算实际客户端数量
// 原理：如果同一IP在同一时间窗口内有多个并发请求，说明有多个客户端
func GetConcurrentConnections(configID string) (int, error) {
	// 查找最近5分钟内，每秒的最大并发请求数
	// 如果同一IP在同一秒内有N个请求，说明至少有N个客户端
	query := `
		WITH time_groups AS (
			SELECT 
				client_ip,
				user_agent,
				strftime('%Y-%m-%d %H:%M:%S', created_at) as time_bucket,
				COUNT(*) as concurrent_count
			FROM request_logs
			WHERE config_id = ?
				AND created_at >= datetime('now', '-5 minutes')
				AND (client_ip IS NOT NULL OR user_agent IS NOT NULL)
			GROUP BY client_ip, user_agent, time_bucket
		)
		SELECT COALESCE(MAX(concurrent_count), 0) as max_concurrent
		FROM time_groups
	`

	var maxConcurrent int
	err := DB.QueryRow(query, configID).Scan(&maxConcurrent)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to get concurrent connections: %w", err)
	}

	return maxConcurrent, nil
}

// EstimateActualClients 估算实际客户端数量
// 返回：基础计数（IP+UA组合）和估算的实际数量（考虑并发）
func EstimateActualClients(configID string) (baseCount int, estimatedCount int, err error) {
	// 1. 基础计数：通过 IP + User-Agent 组合
	baseQuery := `
		SELECT COUNT(DISTINCT client_ip || '_' || user_agent)
		FROM request_logs
		WHERE config_id = ?
			AND created_at >= datetime('now', '-5 minutes')
			AND (client_ip IS NOT NULL OR user_agent IS NOT NULL)
	`

	err = DB.QueryRow(baseQuery, configID).Scan(&baseCount)
	if err != nil && err != sql.ErrNoRows {
		return 0, 0, fmt.Errorf("failed to get base count: %w", err)
	}

	// 2. 并发检测：检查是否有并发请求
	concurrent, err := GetConcurrentConnections(configID)
	if err != nil {
		return baseCount, baseCount, err
	}

	// 3. 估算实际数量
	// 如果检测到并发请求，说明同一IP有多个客户端
	estimatedCount = baseCount
	if concurrent > 1 {
		// 如果有并发，实际客户端数可能是基础计数 * 并发倍数
		// 这里采用保守估计：至少是并发数
		if concurrent > estimatedCount {
			estimatedCount = concurrent
		}
	}

	return baseCount, estimatedCount, nil
}
