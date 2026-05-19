package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/claude/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
)

// ModelsHandler Models API处理器
type ModelsHandler struct {
	config *config.Config
}

// NewModelsHandler 创建新的Models处理器
func NewModelsHandler(cfg *config.Config) *ModelsHandler {
	return &ModelsHandler{
		config: cfg,
	}
}

// ListModels 列出可用模型
func (h *ModelsHandler) ListModels(c *gin.Context) {
	fmt.Printf("📋 [Models API] Listing available models\n")

	// Claude支持的模型列表
	modelsList := []models.ModelResponse{
		{
			ID:          "claude-3-opus-20240229",
			Object:      "model",
			Created:     1709164800,
			OwnedBy:     "anthropic",
			DisplayName: "Claude 3 Opus",
			Description: "Most capable model for highly complex tasks",
			ContextSize: 200000,
		},
		{
			ID:          "claude-3-5-sonnet-20241022",
			Object:      "model",
			Created:     1729555200,
			OwnedBy:     "anthropic",
			DisplayName: "Claude 3.5 Sonnet",
			Description: "Balanced performance and speed",
			ContextSize: 200000,
		},
		{
			ID:          "claude-3-5-haiku-20241022",
			Object:      "model",
			Created:     1729555200,
			OwnedBy:     "anthropic",
			DisplayName: "Claude 3.5 Haiku",
			Description: "Fastest and most compact model",
			ContextSize: 200000,
		},
		{
			ID:          "claude-3-haiku-20240307",
			Object:      "model",
			Created:     1709769600,
			OwnedBy:     "anthropic",
			DisplayName: "Claude 3 Haiku",
			Description: "Fast and cost-effective",
			ContextSize: 200000,
		},
	}

	response := models.ListModelsResponse{
		Object: "list",
		Data:   modelsList,
	}

	c.JSON(http.StatusOK, response)
	fmt.Printf("✅ [Models API] Listed %d models\n", len(modelsList))
}

// GetModel 获取模型信息
func (h *ModelsHandler) GetModel(c *gin.Context) {
	modelID := c.Param("model_id")
	fmt.Printf("🔍 [Models API] Getting model: %s\n", modelID)

	// 模型映射
	modelsMap := map[string]models.ModelResponse{
		"claude-3-opus-20240229": {
			ID:          "claude-3-opus-20240229",
			Object:      "model",
			Created:     1709164800,
			OwnedBy:     "anthropic",
			DisplayName: "Claude 3 Opus",
			Description: "Most capable model for highly complex tasks",
			ContextSize: 200000,
		},
		"claude-3-5-sonnet-20241022": {
			ID:          "claude-3-5-sonnet-20241022",
			Object:      "model",
			Created:     1729555200,
			OwnedBy:     "anthropic",
			DisplayName: "Claude 3.5 Sonnet",
			Description: "Balanced performance and speed",
			ContextSize: 200000,
		},
		"claude-3-5-haiku-20241022": {
			ID:          "claude-3-5-haiku-20241022",
			Object:      "model",
			Created:     1729555200,
			OwnedBy:     "anthropic",
			DisplayName: "Claude 3.5 Haiku",
			Description: "Fastest and most compact model",
			ContextSize: 200000,
		},
		"claude-3-haiku-20240307": {
			ID:          "claude-3-haiku-20240307",
			Object:      "model",
			Created:     1709769600,
			OwnedBy:     "anthropic",
			DisplayName: "Claude 3 Haiku",
			Description: "Fast and cost-effective",
			ContextSize: 200000,
		},
	}

	model, exists := modelsMap[modelID]
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "not_found",
				Message: fmt.Sprintf("Model not found: %s", modelID),
			},
		})
		return
	}

	c.JSON(http.StatusOK, model)
	fmt.Printf("✅ [Models API] Found model: %s\n", modelID)
}
