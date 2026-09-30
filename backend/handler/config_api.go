package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// RenewConfigAPIKey renews the Anthropic API key for a config
func (h *Handler) RenewConfigAPIKey(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		CustomToken string `json:"custom_token"`
	}

	// Parse optional custom token
	_ = c.ShouldBindJSON(&req)

	// Check if config exists
	_, err := database.GetAPIConfig(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Config not found",
		})
		return
	}

	// Renew API key (with optional custom token)
	newAPIKey, err := database.RenewAnthropicAPIKey(id, req.CustomToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"new_api_key": newAPIKey,
		"message":     "API key renewed successfully",
	})
}

// GetConfigLogs retrieves logs for a config with filtering, sorting, and pagination
func (h *Handler) GetConfigLogs(c *gin.Context) {
	configID := c.Param("id")
	config, err := database.GetAPIConfig(configID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Config not found",
		})
		return
	}

	userID, role := getUserContext(c)
	if !isAdminRole(role) && config.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Parse query parameters
	params := database.LogsQueryParams{
		ConfigID:  configID,
		Status:    c.Query("status"),
		Model:     c.Query("model"),
		SortBy:    c.DefaultQuery("sort_by", "created_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
		Search:    c.Query("search"),
	}
	if !isAdminRole(role) {
		params.UserID = userID
	}

	// Parse page
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	params.Page = page

	// Parse page size
	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize < 1 {
		pageSize = 20
	}
	params.PageSize = pageSize

	// Get logs
	result, err := database.GetLogsWithFilters(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve logs: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// DeleteConfigLogs deletes all logs for a config
func (h *Handler) DeleteConfigLogs(c *gin.Context) {
	configID := c.Param("id")

	// Check if config exists
	config, err := database.GetAPIConfig(configID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Config not found",
		})
		return
	}

	userID, role := getUserContext(c)
	if !isAdminRole(role) && config.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Delete logs
	if err := database.DeleteConfigLogs(configID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to delete logs: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Logs deleted successfully",
	})
}

// GetLogDetail retrieves detailed information for a single log
func (h *Handler) GetLogDetail(c *gin.Context) {
	logIDStr := c.Param("log_id")
	logID, err := strconv.ParseInt(logIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid log ID",
		})
		return
	}

	log, err := database.GetLogByID(logID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	userID, role := getUserContext(c)
	if !isAdminRole(role) && log.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	c.JSON(http.StatusOK, log)
}

// GetSystemSettings retrieves all system settings.
// The stored proxy password column is ciphertext; it is decrypted back to the
// plaintext proxy_password key so the UI can echo it (mirrors the config-level
// round-trip, where the password is never kept as ciphertext in the browser).
func (h *Handler) GetSystemSettings(c *gin.Context) {
	settings, err := database.GetAllSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load settings"})
		return
	}

	if passEnc, ok := settings[settingProxyPasswordEncrypted]; ok && passEnc != "" {
		pass, decErr := database.DecryptAPIKey(passEnc)
		if decErr != nil {
			// 解不出来就回显空，避免把密文当密码写回库
			log.Printf("[system-proxy] failed to decrypt proxy password for display: %v", decErr)
			pass = ""
		}
		settings[settingProxyPassword] = pass
	} else {
		settings[settingProxyPassword] = ""
	}
	delete(settings, settingProxyPasswordEncrypted)

	c.JSON(http.StatusOK, settings)
}

// UpdateSystemSettings updates system settings.
// Proxy keys are special-cased: proxy_password is encrypted before storage
// (system_settings stores proxy_password_encrypted), and afterwards the
// in-memory h.config.SystemProxy* fields are refreshed so the new service-wide
// default proxy applies immediately, without a restart.
func (h *Handler) UpdateSystemSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	_, role := getUserContext(c)
	if !isAdminRole(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	username := c.GetString("username")

	// 待同步到 h.config.SystemProxy* 的新值（keys 缺失 = 不修改该项）。
	sysSync := map[string]*string{}
	for key := range systemProxyKeys() {
		sysSync[key] = nil // 存在即标记待同步
	}

	for key, value := range req {
		switch key {
		case settingProxyPassword:
			// 明文密码：非空则加密存储，空则保留旧密文不动。
			if value != "" {
				enc, encErr := database.EncryptAPIKey(value)
				if encErr != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to encrypt proxy password: %v", encErr)})
					return
				}
				if err := database.SetSetting(settingProxyPasswordEncrypted, enc, username); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save setting %s: %v", settingProxyPasswordEncrypted, err)})
					return
				}
				v := value
				sysSync[settingProxyPassword] = &v
			}
			continue
		case settingProxyURL, settingProxyType, settingProxyUsername:
			if err := database.SetSetting(key, value, username); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save setting %s: %v", key, err)})
				return
			}
			v := value
			sysSync[key] = &v
		default:
			if err := database.SetSetting(key, value, username); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to save setting %s: %v", key, err)})
				return
			}
		}
	}

	// 热生效：把新值同步进内存 SystemProxy*，config 级 proxy_url 为空的流量
	// 在下一个请求即走新代理，无需重启。
	h.syncSystemProxy(sysSync)

	c.JSON(http.StatusOK, gin.H{"message": "Settings updated successfully"})
}

// systemProxyKeys 返回全局默认代理相关的 system_settings 逻辑键。
func systemProxyKeys() map[string]struct{} {
	return map[string]struct{}{
		settingProxyURL:      {},
		settingProxyType:     {},
		settingProxyUsername: {},
		settingProxyPassword: {},
	}
}

// syncSystemProxy 把 PUT 中出现的代理键同步到 h.config.SystemProxy*。
// 未出现的键保持原值；密码在内存中始终为明文（与 config 级 ProxyPassword 一致）。
func (h *Handler) syncSystemProxy(updates map[string]*string) {
	if h.config == nil {
		return
	}
	if v, ok := updates[settingProxyURL]; ok {
		h.config.SystemProxyURL = derefProxyValue(v)
	}
	if v, ok := updates[settingProxyType]; ok {
		h.config.SystemProxyType = derefProxyValue(v)
	}
	if v, ok := updates[settingProxyUsername]; ok {
		h.config.SystemProxyUsername = derefProxyValue(v)
	}
	if v, ok := updates[settingProxyPassword]; ok {
		h.config.SystemProxyPassword = derefProxyValue(v)
	}
}

func derefProxyValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// GetLogStats returns log storage statistics
func (h *Handler) GetLogStats(c *gin.Context) {
	totalLogs, err := database.GetRequestLogCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get log stats"})
		return
	}

	// Get database size info
	sizeInfo, err := database.GetDatabaseSizeInfo()
	if err != nil {
		// Don't fail the whole request, just omit size info
		c.JSON(http.StatusOK, gin.H{
			"total_logs": totalLogs,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total_logs":       totalLogs,
		"db_size_bytes":    sizeInfo.DBSizeBytes,
		"wal_size_bytes":   sizeInfo.WALSizeBytes,
		"total_size_bytes": sizeInfo.TotalBytes,
		"free_pages":       sizeInfo.FreePages,
		"total_pages":      sizeInfo.TotalPages,
		"page_size":        sizeInfo.PageSize,
		"body_size_bytes":  sizeInfo.BodySizeBytes,
		"db_path":          sizeInfo.DBPath,
	})
}

// TriggerCleanup manually triggers the cleanup process
func (h *Handler) TriggerCleanup(c *gin.Context) {
	_, role := getUserContext(c)
	if !isAdminRole(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	go func() {
		if err := CleanupOldData(); err != nil {
			log.Printf("Manual cleanup failed: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "Cleanup triggered successfully"})
}

// TriggerVacuum manually triggers VACUUM INTO
func (h *Handler) TriggerVacuum(c *gin.Context) {
	_, role := getUserContext(c)
	if !isAdminRole(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	sizeInfo, err := database.GetDatabaseSizeInfo()
	if err != nil || sizeInfo.DBPath == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get database path"})
		return
	}

	newSize, err := database.VacuumInto(sizeInfo.DBPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("VACUUM failed: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "VACUUM completed successfully",
		"new_size_bytes": newSize,
	})
}

// MigrateBodies manually triggers inline body migration
func (h *Handler) MigrateBodies(c *gin.Context) {
	_, role := getUserContext(c)
	if !isAdminRole(role) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
		return
	}

	migrated, err := database.MigrateInlineBodiesToFiles(1000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Migration failed: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Migration batch completed",
		"migrated": migrated,
	})
}

// GetAvailableModels returns available models for filtering
func (h *Handler) GetAvailableModels(c *gin.Context) {
	configID := c.Param("id")
	config, err := database.GetAPIConfig(configID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Config not found"})
		return
	}
	userID, role := getUserContext(c)
	if !isAdminRole(role) && config.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	models, err := database.GetAvailableModels(configID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get models: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"models": models,
	})
}

// GetHistoricalModels returns all unique models from all configs and logs
// This is used by the frontend ModelSelector component to provide autocomplete suggestions
func (h *Handler) GetHistoricalModels(c *gin.Context) {
	models, err := database.GetAllHistoricalModels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get historical models: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"models": models,
	})
}
