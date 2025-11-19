package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/claude/models"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/config"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
)

// AdminHandler Admin API处理器
type AdminHandler struct {
	config *config.Config
}

// NewAdminHandler 创建新的Admin处理器
func NewAdminHandler(cfg *config.Config) *AdminHandler {
	return &AdminHandler{
		config: cfg,
	}
}

// GetMe 获取当前组织信息
// 这是Claude CLI启动时需要调用的关键端点
func (h *AdminHandler) GetMe(c *gin.Context) {
	fmt.Printf("🔵 [Admin API] GetMe - 获取组织信息\n")

	// 从路径获取配置ID（如果有）
	configID := c.Param("id")
	if configID == "" {
		configID = "default"
	}

	// 检查是否使用特定配置
	var orgName string
	var orgID string

	if configID != "default" {
		// 尝试从数据库获取配置（如果数据库已初始化）
		if database.IsInitialized() {
			dbConfig, err := database.GetAPIConfig(configID)
			if err == nil && dbConfig != nil {
				orgName = dbConfig.Name
				orgID = fmt.Sprintf("org_%s", configID)
			} else {
				orgName = "Proxy Organization"
				orgID = fmt.Sprintf("org_%s", configID)
			}
		} else {
			// 数据库未初始化时的默认行为
			orgName = "Proxy Organization"
			orgID = fmt.Sprintf("org_%s", configID)
		}
	} else {
		orgName = "Default Organization"
		orgID = "org_default"
	}

	// 返回组织信息
	response := models.OrganizationResponse{
		ID:   orgID,
		Name: orgName,
		Type: "organization",
	}

	c.JSON(http.StatusOK, response)
	fmt.Printf("✅ [Admin API] 返回组织信息: %s (%s)\n", orgName, orgID)
}

// GetOrganizationUsage 获取组织使用情况
func (h *AdminHandler) GetOrganizationUsage(c *gin.Context) {
	orgID := c.Param("org_id")
	fmt.Printf("📊 [Admin API] Getting usage for organization: %s\n", orgID)

	// 模拟使用情况数据
	usage := models.OrganizationUsageResponse{
		Object: "usage",
		Data: []models.UsageEntry{
			{
				Type:          "credit_balance",
				CreditBalance: 1000000.0, // 返回一个较大的余额
			},
			{
				Type:        "credits_used",
				CreditsUsed: 12345.67,
				Period:      "2024-11",
			},
			{
				Type:       "tokens_used",
				TokensUsed: 5000000,
				Period:     "2024-11",
			},
		},
	}

	c.JSON(http.StatusOK, usage)
	fmt.Printf("✅ [Admin API] Returned usage for organization: %s\n", orgID)
}
