package claude

import (
	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/claude/handlers"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// RegisterRoutes 注册所有Claude API路由
func RegisterRoutes(router *gin.Engine, cfg *config.Config) {
	// 创建处理器
	handler := handlers.NewHandler(cfg)

	// 标准Claude API路由 (/v1/*)
	v1 := router.Group("/v1")
	{
		// Messages API
		v1.POST("/messages", handler.CreateMessage)
		v1.POST("/messages/count_tokens", handler.CountTokens)

		// Batch API
		v1.POST("/batches", handler.CreateBatch)
		v1.GET("/batches/:batch_id", handler.GetBatch)
		v1.GET("/batches", handler.ListBatches)
		v1.GET("/batches/:batch_id/results", handler.GetBatchResults)
		v1.POST("/batches/:batch_id/cancel", handler.CancelBatch)
		v1.DELETE("/batches/:batch_id", handler.DeleteBatch)

		// Files API
		v1.POST("/files", handler.CreateFile)
		v1.GET("/files", handler.ListFiles)
		v1.GET("/files/:file_id", handler.GetFileMetadata)
		v1.GET("/files/:file_id/content", handler.GetFileContent)
		v1.DELETE("/files/:file_id", handler.DeleteFile)

		// Skills API
		v1.POST("/skills", handler.CreateSkill)
		v1.GET("/skills", handler.ListSkills)
		v1.GET("/skills/:skill_id", handler.GetSkill)
		v1.DELETE("/skills/:skill_id", handler.DeleteSkill)
		v1.POST("/skills/:skill_id/versions", handler.CreateSkillVersion)
		v1.GET("/skills/:skill_id/versions", handler.ListSkillVersions)
		v1.GET("/skills/:skill_id/versions/:version_id", handler.GetSkillVersion)
		v1.DELETE("/skills/:skill_id/versions/:version_id", handler.DeleteSkillVersion)

		// Models API
		v1.GET("/models", handler.ListModels)
		v1.GET("/models/:model_id", handler.GetModel)

		// Admin API (组织相关)
		v1.GET("/me", handler.GetMe)
		v1.GET("/organizations/:org_id/usage", handler.GetOrganizationUsage)
	}

	// 每个配置独立的路由 (/proxy/:id/v1/*)
	proxyGroup := router.Group("/proxy/:id/v1")
	{
		// Messages API
		proxyGroup.POST("/messages", createProxyHandler(cfg, "messages"))
		proxyGroup.POST("/messages/count_tokens", createProxyHandler(cfg, "count_tokens"))

		// Batch API
		proxyGroup.POST("/batches", createProxyHandler(cfg, "create_batch"))
		proxyGroup.GET("/batches/:batch_id", createProxyHandler(cfg, "get_batch"))
		proxyGroup.GET("/batches", createProxyHandler(cfg, "list_batches"))
		proxyGroup.GET("/batches/:batch_id/results", createProxyHandler(cfg, "get_batch_results"))
		proxyGroup.POST("/batches/:batch_id/cancel", createProxyHandler(cfg, "cancel_batch"))
		proxyGroup.DELETE("/batches/:batch_id", createProxyHandler(cfg, "delete_batch"))

		// Files API
		proxyGroup.POST("/files", createProxyHandler(cfg, "create_file"))
		proxyGroup.GET("/files", createProxyHandler(cfg, "list_files"))
		proxyGroup.GET("/files/:file_id", createProxyHandler(cfg, "get_file_metadata"))
		proxyGroup.GET("/files/:file_id/content", createProxyHandler(cfg, "get_file_content"))
		proxyGroup.DELETE("/files/:file_id", createProxyHandler(cfg, "delete_file"))

		// Skills API
		proxyGroup.POST("/skills", createProxyHandler(cfg, "create_skill"))
		proxyGroup.GET("/skills", createProxyHandler(cfg, "list_skills"))
		proxyGroup.GET("/skills/:skill_id", createProxyHandler(cfg, "get_skill"))
		proxyGroup.DELETE("/skills/:skill_id", createProxyHandler(cfg, "delete_skill"))
		proxyGroup.POST("/skills/:skill_id/versions", createProxyHandler(cfg, "create_skill_version"))
		proxyGroup.GET("/skills/:skill_id/versions", createProxyHandler(cfg, "list_skill_versions"))
		proxyGroup.GET("/skills/:skill_id/versions/:version_id", createProxyHandler(cfg, "get_skill_version"))
		proxyGroup.DELETE("/skills/:skill_id/versions/:version_id", createProxyHandler(cfg, "delete_skill_version"))

		// Models API
		proxyGroup.GET("/models", createProxyHandler(cfg, "list_models"))
		proxyGroup.GET("/models/:model_id", createProxyHandler(cfg, "get_model"))

		// Admin API
		proxyGroup.GET("/me", createProxyHandler(cfg, "get_me"))
		proxyGroup.GET("/organizations/:org_id/usage", createProxyHandler(cfg, "get_organization_usage"))
	}
}

// createProxyHandler 为特定配置创建处理函数
func createProxyHandler(cfg *config.Config, endpoint string) gin.HandlerFunc {
	return func(c *gin.Context) {
		configID := c.Param("id")

		// 从数据库获取配置
		dbConfig, err := database.GetAPIConfig(configID)
		if err != nil {
			c.JSON(404, gin.H{
				"error": map[string]interface{}{
					"type":    "not_found",
					"message": "Config not found: " + configID,
				},
			})
			return
		}

		if !dbConfig.Enabled {
			c.JSON(400, gin.H{
				"error": map[string]interface{}{
					"type":    "invalid_request",
					"message": "Config is disabled: " + configID,
				},
			})
			return
		}

		// 使用特定配置创建处理器
		handler := handlers.NewHandlerWithConfig(cfg, dbConfig)

		// 根据端点调用相应方法
		switch endpoint {
		case "messages":
			handler.CreateMessage(c)
		case "count_tokens":
			handler.CountTokens(c)
		case "create_batch":
			handler.CreateBatch(c)
		case "get_batch":
			handler.GetBatch(c)
		case "list_batches":
			handler.ListBatches(c)
		case "get_batch_results":
			handler.GetBatchResults(c)
		case "cancel_batch":
			handler.CancelBatch(c)
		case "delete_batch":
			handler.DeleteBatch(c)
		case "create_file":
			handler.CreateFile(c)
		case "list_files":
			handler.ListFiles(c)
		case "get_file_metadata":
			handler.GetFileMetadata(c)
		case "get_file_content":
			handler.GetFileContent(c)
		case "delete_file":
			handler.DeleteFile(c)
		case "create_skill":
			handler.CreateSkill(c)
		case "list_skills":
			handler.ListSkills(c)
		case "get_skill":
			handler.GetSkill(c)
		case "delete_skill":
			handler.DeleteSkill(c)
		case "create_skill_version":
			handler.CreateSkillVersion(c)
		case "list_skill_versions":
			handler.ListSkillVersions(c)
		case "get_skill_version":
			handler.GetSkillVersion(c)
		case "delete_skill_version":
			handler.DeleteSkillVersion(c)
		case "list_models":
			handler.ListModels(c)
		case "get_model":
			handler.GetModel(c)
		case "get_me":
			handler.GetMe(c)
		case "get_organization_usage":
			handler.GetOrganizationUsage(c)
		default:
			c.JSON(404, gin.H{
				"error": map[string]interface{}{
					"type":    "not_found",
					"message": "Endpoint not found",
				},
			})
		}
	}
}
