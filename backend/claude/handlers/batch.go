package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/claude/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
)

// BatchHandler Batch API处理器
type BatchHandler struct {
	config  *config.Config
	batches map[string]*models.BatchResponse // 简单的内存存储
}

// NewBatchHandler 创建新的Batch处理器
func NewBatchHandler(cfg *config.Config) *BatchHandler {
	return &BatchHandler{
		config:  cfg,
		batches: make(map[string]*models.BatchResponse),
	}
}

// CreateBatch 创建批处理
func (h *BatchHandler) CreateBatch(c *gin.Context) {
	var req models.CreateBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "invalid_request_error",
				Message: fmt.Sprintf("Invalid request: %v", err),
			},
		})
		return
	}

	fmt.Printf("📦 [Batch API] Creating batch with %d requests\n", len(req.Requests))

	// 创建批处理响应
	batchID := "batch_" + uuid.New().String()
	batch := &models.BatchResponse{
		ID:               batchID,
		Type:             "message_batch",
		ProcessingStatus: "in_progress",
		RequestCounts: models.BatchRequestCounts{
			Processing: len(req.Requests),
			Total:      len(req.Requests),
		},
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	// 存储批处理
	h.batches[batchID] = batch

	c.JSON(http.StatusOK, batch)
	fmt.Printf("✅ [Batch API] Created batch: %s\n", batchID)
}

// GetBatch 获取批处理信息
func (h *BatchHandler) GetBatch(c *gin.Context) {
	batchID := c.Param("batch_id")
	fmt.Printf("🔍 [Batch API] Getting batch: %s\n", batchID)

	batch, exists := h.batches[batchID]
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "not_found",
				Message: fmt.Sprintf("Batch not found: %s", batchID),
			},
		})
		return
	}

	c.JSON(http.StatusOK, batch)
}

// ListBatches 列出批处理
func (h *BatchHandler) ListBatches(c *gin.Context) {
	fmt.Printf("📋 [Batch API] Listing batches\n")

	var params models.ListBatchesParams
	c.ShouldBindQuery(&params)

	// 简单实现：返回所有批处理
	batches := make([]models.BatchResponse, 0, len(h.batches))
	for _, batch := range h.batches {
		batches = append(batches, *batch)
	}

	response := models.ListBatchesResponse{
		Data:    batches,
		HasMore: false,
	}

	c.JSON(http.StatusOK, response)
	fmt.Printf("✅ [Batch API] Listed %d batches\n", len(batches))
}

// GetBatchResults 获取批处理结果
func (h *BatchHandler) GetBatchResults(c *gin.Context) {
	batchID := c.Param("batch_id")
	fmt.Printf("📊 [Batch API] Getting results for batch: %s\n", batchID)

	batch, exists := h.batches[batchID]
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "not_found",
				Message: fmt.Sprintf("Batch not found: %s", batchID),
			},
		})
		return
	}

	// 模拟结果
	results := models.BatchResultsResponse{
		Results: []models.BatchResult{
			{
				CustomID: "example_1",
				Type:     "success",
				Result: &models.MessagesResponse{
					ID:         "msg_" + uuid.New().String(),
					Type:       "message",
					Role:       "assistant",
					Model:      batch.ID,
					Content:    []models.ContentBlock{{Type: "text", Text: "This is a batch result"}},
					StopReason: "end_turn",
					Usage: models.Usage{
						InputTokens:  10,
						OutputTokens: 5,
					},
				},
			},
		},
	}

	c.JSON(http.StatusOK, results)
}

// CancelBatch 取消批处理
func (h *BatchHandler) CancelBatch(c *gin.Context) {
	batchID := c.Param("batch_id")
	fmt.Printf("❌ [Batch API] Canceling batch: %s\n", batchID)

	batch, exists := h.batches[batchID]
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "not_found",
				Message: fmt.Sprintf("Batch not found: %s", batchID),
			},
		})
		return
	}

	// 更新状态
	batch.ProcessingStatus = "canceled"
	now := time.Now()
	batch.CancelInitiatedAt = &now
	batch.EndedAt = &now

	c.JSON(http.StatusOK, batch)
	fmt.Printf("✅ [Batch API] Canceled batch: %s\n", batchID)
}

// DeleteBatch 删除批处理
func (h *BatchHandler) DeleteBatch(c *gin.Context) {
	batchID := c.Param("batch_id")
	fmt.Printf("🗑️ [Batch API] Deleting batch: %s\n", batchID)

	_, exists := h.batches[batchID]
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "not_found",
				Message: fmt.Sprintf("Batch not found: %s", batchID),
			},
		})
		return
	}

	// 删除批处理
	delete(h.batches, batchID)

	c.JSON(http.StatusOK, gin.H{
		"deleted": true,
		"id":      batchID,
	})
	fmt.Printf("✅ [Batch API] Deleted batch: %s\n", batchID)
}
