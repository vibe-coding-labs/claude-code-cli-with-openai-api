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

// SkillsHandler Skills API处理器
type SkillsHandler struct {
	config   *config.Config
	skills   map[string]*models.SkillResponse        // 技能存储
	versions map[string]*models.SkillVersionResponse // 版本存储
}

// NewSkillsHandler 创建新的Skills处理器
func NewSkillsHandler(cfg *config.Config) *SkillsHandler {
	return &SkillsHandler{
		config:   cfg,
		skills:   make(map[string]*models.SkillResponse),
		versions: make(map[string]*models.SkillVersionResponse),
	}
}

// CreateSkill 创建技能
func (h *SkillsHandler) CreateSkill(c *gin.Context) {
	var req models.CreateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "invalid_request_error",
				Message: fmt.Sprintf("Invalid request: %v", err),
			},
		})
		return
	}

	fmt.Printf("🎯 [Skills API] Creating skill: %s\n", req.Name)

	// 创建技能响应
	skillID := "skill_" + uuid.New().String()
	skill := &models.SkillResponse{
		ID:           skillID,
		Type:         "skill",
		Name:         req.Name,
		Description:  req.Description,
		Instructions: req.Instructions,
		Parameters:   req.Parameters,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// 存储技能
	h.skills[skillID] = skill

	c.JSON(http.StatusOK, skill)
	fmt.Printf("✅ [Skills API] Created skill: %s\n", skillID)
}

// ListSkills 列出技能
func (h *SkillsHandler) ListSkills(c *gin.Context) {
	fmt.Printf("📋 [Skills API] Listing skills\n")

	var params models.ListSkillsParams
	c.ShouldBindQuery(&params)

	// 简单实现：返回所有技能
	skills := make([]models.SkillResponse, 0, len(h.skills))
	for _, skill := range h.skills {
		skills = append(skills, *skill)
	}

	response := models.ListSkillsResponse{
		Data:    skills,
		HasMore: false,
	}

	c.JSON(http.StatusOK, response)
	fmt.Printf("✅ [Skills API] Listed %d skills\n", len(skills))
}

// GetSkill 获取技能
func (h *SkillsHandler) GetSkill(c *gin.Context) {
	skillID := c.Param("skill_id")
	fmt.Printf("🔍 [Skills API] Getting skill: %s\n", skillID)

	skill, exists := h.skills[skillID]
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "not_found",
				Message: fmt.Sprintf("Skill not found: %s", skillID),
			},
		})
		return
	}

	c.JSON(http.StatusOK, skill)
}

// DeleteSkill 删除技能
func (h *SkillsHandler) DeleteSkill(c *gin.Context) {
	skillID := c.Param("skill_id")
	fmt.Printf("🗑️ [Skills API] Deleting skill: %s\n", skillID)

	_, exists := h.skills[skillID]
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "not_found",
				Message: fmt.Sprintf("Skill not found: %s", skillID),
			},
		})
		return
	}

	// 删除技能
	delete(h.skills, skillID)

	c.JSON(http.StatusOK, gin.H{
		"deleted": true,
		"id":      skillID,
	})
	fmt.Printf("✅ [Skills API] Deleted skill: %s\n", skillID)
}

// CreateSkillVersion 创建技能版本
func (h *SkillsHandler) CreateSkillVersion(c *gin.Context) {
	skillID := c.Param("skill_id")

	var req models.CreateSkillVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "invalid_request_error",
				Message: fmt.Sprintf("Invalid request: %v", err),
			},
		})
		return
	}

	fmt.Printf("📌 [Skills API] Creating version for skill: %s\n", skillID)

	// 检查技能是否存在
	_, exists := h.skills[skillID]
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "not_found",
				Message: fmt.Sprintf("Skill not found: %s", skillID),
			},
		})
		return
	}

	// 创建版本响应
	versionID := "skver_" + uuid.New().String()
	version := &models.SkillVersionResponse{
		ID:           versionID,
		Type:         "skill_version",
		SkillID:      skillID,
		Version:      1,
		Instructions: req.Instructions,
		Parameters:   req.Parameters,
		CreatedAt:    time.Now(),
		IsActive:     true,
	}

	// 存储版本
	h.versions[versionID] = version

	c.JSON(http.StatusOK, version)
	fmt.Printf("✅ [Skills API] Created version: %s\n", versionID)
}

// ListSkillVersions 列出技能版本
func (h *SkillsHandler) ListSkillVersions(c *gin.Context) {
	skillID := c.Param("skill_id")
	fmt.Printf("📋 [Skills API] Listing versions for skill: %s\n", skillID)

	var params models.ListSkillVersionsParams
	c.ShouldBindQuery(&params)

	// 过滤该技能的版本
	versions := make([]models.SkillVersionResponse, 0)
	for _, version := range h.versions {
		if version.SkillID == skillID {
			versions = append(versions, *version)
		}
	}

	response := models.ListSkillVersionsResponse{
		Data:    versions,
		HasMore: false,
	}

	c.JSON(http.StatusOK, response)
	fmt.Printf("✅ [Skills API] Listed %d versions\n", len(versions))
}

// GetSkillVersion 获取技能版本
func (h *SkillsHandler) GetSkillVersion(c *gin.Context) {
	versionID := c.Param("version_id")
	fmt.Printf("🔍 [Skills API] Getting version: %s\n", versionID)

	version, exists := h.versions[versionID]
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "not_found",
				Message: fmt.Sprintf("Version not found: %s", versionID),
			},
		})
		return
	}

	c.JSON(http.StatusOK, version)
}

// DeleteSkillVersion 删除技能版本
func (h *SkillsHandler) DeleteSkillVersion(c *gin.Context) {
	versionID := c.Param("version_id")
	fmt.Printf("🗑️ [Skills API] Deleting version: %s\n", versionID)

	_, exists := h.versions[versionID]
	if !exists {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error: models.ErrorDetail{
				Type:    "not_found",
				Message: fmt.Sprintf("Version not found: %s", versionID),
			},
		})
		return
	}

	// 删除版本
	delete(h.versions, versionID)

	c.JSON(http.StatusOK, gin.H{
		"deleted": true,
		"id":      versionID,
	})
	fmt.Printf("✅ [Skills API] Deleted version: %s\n", versionID)
}
