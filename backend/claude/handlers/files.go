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

// FilesHandler Files API处理器
type FilesHandler struct {
	config *config.Config
	files  map[string]*models.FileResponse // 简单的内存存储
}

// NewFilesHandler 创建新的Files处理器
func NewFilesHandler(cfg *config.Config) *FilesHandler {
	return &FilesHandler{
		config: cfg,
		files:  make(map[string]*models.FileResponse),
	}
}

// CreateFile 创建文件
func (h *FilesHandler) CreateFile(c *gin.Context) {
	// 处理multipart form
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "invalid_request_error",
				Message: fmt.Sprintf("Failed to get file: %v", err),
			},
		})
		return
	}
	defer file.Close()

	purpose := c.PostForm("purpose")
	if purpose == "" {
		purpose = "assistants"
	}

	fmt.Printf("📁 [Files API] Creating file: %s (purpose: %s)\n", header.Filename, purpose)

	// 创建文件响应
	fileID := "file_" + uuid.New().String()
	fileResp := &models.FileResponse{
		ID:        fileID,
		Type:      "file",
		FileName:  header.Filename,
		Purpose:   purpose,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		FileSize:  header.Size,
		Status:    "processed",
	}

	// 存储文件信息
	h.files[fileID] = fileResp

	c.JSON(http.StatusOK, fileResp)
	fmt.Printf("✅ [Files API] Created file: %s\n", fileID)
}

// ListFiles 列出文件
func (h *FilesHandler) ListFiles(c *gin.Context) {
	fmt.Printf("📋 [Files API] Listing files\n")

	var params models.ListFilesParams
	c.ShouldBindQuery(&params)

	// 简单实现：返回所有文件
	files := make([]models.FileResponse, 0, len(h.files))
	for _, file := range h.files {
		if params.Purpose == "" || file.Purpose == params.Purpose {
			files = append(files, *file)
		}
	}

	response := models.ListFilesResponse{
		Data:    files,
		HasMore: false,
	}

	c.JSON(http.StatusOK, response)
	fmt.Printf("✅ [Files API] Listed %d files\n", len(files))
}

// GetFileMetadata 获取文件元数据
func (h *FilesHandler) GetFileMetadata(c *gin.Context) {
	fileID := c.Param("file_id")
	fmt.Printf("🔍 [Files API] Getting metadata for file: %s\n", fileID)

	file, exists := h.files[fileID]
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "not_found",
				Message: fmt.Sprintf("File not found: %s", fileID),
			},
		})
		return
	}

	c.JSON(http.StatusOK, file)
}

// GetFileContent 获取文件内容
func (h *FilesHandler) GetFileContent(c *gin.Context) {
	fileID := c.Param("file_id")
	fmt.Printf("📄 [Files API] Getting content for file: %s\n", fileID)

	file, exists := h.files[fileID]
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "not_found",
				Message: fmt.Sprintf("File not found: %s", fileID),
			},
		})
		return
	}

	// 模拟文件内容
	c.Data(http.StatusOK, "text/plain", []byte("This is the file content for "+file.FileName))
	fmt.Printf("✅ [Files API] Sent content for file: %s\n", fileID)
}

// DeleteFile 删除文件
func (h *FilesHandler) DeleteFile(c *gin.Context) {
	fileID := c.Param("file_id")
	fmt.Printf("🗑️ [Files API] Deleting file: %s\n", fileID)

	_, exists := h.files[fileID]
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "not_found",
				Message: fmt.Sprintf("File not found: %s", fileID),
			},
		})
		return
	}

	// 删除文件
	delete(h.files, fileID)

	c.JSON(http.StatusOK, gin.H{
		"deleted": true,
		"id":      fileID,
	})
	fmt.Printf("✅ [Files API] Deleted file: %s\n", fileID)
}
