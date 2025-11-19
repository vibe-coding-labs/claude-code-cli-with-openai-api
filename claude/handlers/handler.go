package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// Handler Claude API主处理器
type Handler struct {
	messagesHandler *MessagesHandler
	batchHandler    *BatchHandler
	filesHandler    *FilesHandler
	skillsHandler   *SkillsHandler
	modelsHandler   *ModelsHandler
	adminHandler    *AdminHandler
	config          *config.Config
}

// NewHandler 创建新的Claude API处理器
func NewHandler(cfg *config.Config) ClaudeHandler {
	return &Handler{
		messagesHandler: NewMessagesHandler(cfg),
		batchHandler:    NewBatchHandler(cfg),
		filesHandler:    NewFilesHandler(cfg),
		skillsHandler:   NewSkillsHandler(cfg),
		modelsHandler:   NewModelsHandler(cfg),
		adminHandler:    NewAdminHandler(cfg),
		config:          cfg,
	}
}

// Messages API 方法委托
func (h *Handler) CreateMessage(c *gin.Context) {
	h.messagesHandler.CreateMessage(c)
}

func (h *Handler) CountTokens(c *gin.Context) {
	h.messagesHandler.CountTokens(c)
}

// Batch API 方法委托
func (h *Handler) CreateBatch(c *gin.Context) {
	h.batchHandler.CreateBatch(c)
}

func (h *Handler) GetBatch(c *gin.Context) {
	h.batchHandler.GetBatch(c)
}

func (h *Handler) ListBatches(c *gin.Context) {
	h.batchHandler.ListBatches(c)
}

func (h *Handler) GetBatchResults(c *gin.Context) {
	h.batchHandler.GetBatchResults(c)
}

func (h *Handler) CancelBatch(c *gin.Context) {
	h.batchHandler.CancelBatch(c)
}

func (h *Handler) DeleteBatch(c *gin.Context) {
	h.batchHandler.DeleteBatch(c)
}

// Files API 方法委托
func (h *Handler) CreateFile(c *gin.Context) {
	h.filesHandler.CreateFile(c)
}

func (h *Handler) ListFiles(c *gin.Context) {
	h.filesHandler.ListFiles(c)
}

func (h *Handler) GetFileMetadata(c *gin.Context) {
	h.filesHandler.GetFileMetadata(c)
}

func (h *Handler) GetFileContent(c *gin.Context) {
	h.filesHandler.GetFileContent(c)
}

func (h *Handler) DeleteFile(c *gin.Context) {
	h.filesHandler.DeleteFile(c)
}

// Skills API 方法委托
func (h *Handler) CreateSkill(c *gin.Context) {
	h.skillsHandler.CreateSkill(c)
}

func (h *Handler) ListSkills(c *gin.Context) {
	h.skillsHandler.ListSkills(c)
}

func (h *Handler) GetSkill(c *gin.Context) {
	h.skillsHandler.GetSkill(c)
}

func (h *Handler) DeleteSkill(c *gin.Context) {
	h.skillsHandler.DeleteSkill(c)
}

func (h *Handler) CreateSkillVersion(c *gin.Context) {
	h.skillsHandler.CreateSkillVersion(c)
}

func (h *Handler) ListSkillVersions(c *gin.Context) {
	h.skillsHandler.ListSkillVersions(c)
}

func (h *Handler) GetSkillVersion(c *gin.Context) {
	h.skillsHandler.GetSkillVersion(c)
}

func (h *Handler) DeleteSkillVersion(c *gin.Context) {
	h.skillsHandler.DeleteSkillVersion(c)
}

// Models API 方法委托
func (h *Handler) ListModels(c *gin.Context) {
	h.modelsHandler.ListModels(c)
}

func (h *Handler) GetModel(c *gin.Context) {
	h.modelsHandler.GetModel(c)
}

// Admin API 方法委托
func (h *Handler) GetMe(c *gin.Context) {
	h.adminHandler.GetMe(c)
}

func (h *Handler) GetOrganizationUsage(c *gin.Context) {
	h.adminHandler.GetOrganizationUsage(c)
}

// NewHandlerWithConfig 使用特定配置创建处理器
func NewHandlerWithConfig(cfg *config.Config, dbConfig *database.APIConfig) ClaudeHandler {
	// 使用数据库配置覆盖默认配置
	if dbConfig != nil {
		customConfig := &config.Config{
			OpenAIBaseURL:   dbConfig.OpenAIBaseURL,
			OpenAIAPIKey:    dbConfig.OpenAIAPIKey,
			BigModel:        dbConfig.BigModel,
			MiddleModel:     dbConfig.MiddleModel,
			SmallModel:      dbConfig.SmallModel,
			MaxTokensLimit:  dbConfig.MaxTokensLimit,
			RequestTimeout:  cfg.RequestTimeout,
			AnthropicAPIKey: dbConfig.AnthropicAPIKey,
		}
		return NewHandler(customConfig)
	}
	return NewHandler(cfg)
}
