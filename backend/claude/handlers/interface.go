package handlers

import "github.com/gin-gonic/gin"

// ClaudeHandler 定义所有Claude API处理器接口
type ClaudeHandler interface {
	// Messages API
	CreateMessage(c *gin.Context)
	CountTokens(c *gin.Context)

	// Batch API
	CreateBatch(c *gin.Context)
	GetBatch(c *gin.Context)
	ListBatches(c *gin.Context)
	GetBatchResults(c *gin.Context)
	CancelBatch(c *gin.Context)
	DeleteBatch(c *gin.Context)

	// Files API
	CreateFile(c *gin.Context)
	ListFiles(c *gin.Context)
	GetFileMetadata(c *gin.Context)
	GetFileContent(c *gin.Context)
	DeleteFile(c *gin.Context)

	// Skills API
	CreateSkill(c *gin.Context)
	ListSkills(c *gin.Context)
	GetSkill(c *gin.Context)
	DeleteSkill(c *gin.Context)
	CreateSkillVersion(c *gin.Context)
	ListSkillVersions(c *gin.Context)
	GetSkillVersion(c *gin.Context)
	DeleteSkillVersion(c *gin.Context)

	// Models API
	ListModels(c *gin.Context)
	GetModel(c *gin.Context)

	// Admin API
	GetMe(c *gin.Context)
	GetOrganizationUsage(c *gin.Context)
}
