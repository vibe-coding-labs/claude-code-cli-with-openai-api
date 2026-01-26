package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

// CheckInitialized checks if the system is initialized
func (h *Handler) CheckInitialized(c *gin.Context) {
	hasUser, err := database.HasUser()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check initialization"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"initialized": hasUser,
	})
}

// InitializeSystem creates the first user
func (h *Handler) InitializeSystem(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required,min=3,max=50"`
		Password string `json:"password" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	// Check if already initialized
	hasUser, err := database.HasUser()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check initialization"})
		return
	}

	if hasUser {
		c.JSON(http.StatusBadRequest, gin.H{"error": "System already initialized"})
		return
	}

	// Create user
	if err := database.CreateUser(req.Username, req.Password); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user: " + err.Error()})
		return
	}

	// Get user for token generation
	user, err := database.GetUser(req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user"})
		return
	}

	// Generate JWT token
	token, err := utils.GenerateToken(user.Username, user.ID, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "System initialized successfully",
		"token":   token,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
			"status":   user.Status,
		},
	})
}

// Login authenticates user and returns JWT token
func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	// Validate credentials
	valid, err := database.ValidatePassword(req.Username, req.Password)
	if err != nil || !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid username or password"})
		return
	}

	// Get user
	user, err := database.GetUser(req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user"})
		return
	}
	if strings.ToLower(strings.TrimSpace(user.Status)) != "active" {
		c.JSON(http.StatusForbidden, gin.H{"error": "User is disabled"})
		return
	}

	// Generate JWT token
	token, err := utils.GenerateToken(user.Username, user.ID, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Login successful",
		"token":   token,
		"user": gin.H{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
			"status":   user.Status,
		},
	})
}

// AuthMiddleware validates JWT token
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Skip auth for public paths
		publicPaths := []string{
			"/api/auth/initialized",
			"/api/auth/initialize",
			"/api/auth/login",
			"/v1/",
			"/proxy/",
			"/health",
			"/test-connection",
			"/",
		}

		for _, publicPath := range publicPaths {
			if path == publicPath || strings.HasPrefix(path, publicPath) {
				c.Next()
				return
			}
		}

		// Allow UI access for all paths (handled by frontend routing)
		if strings.HasPrefix(path, "/ui/") || path == "/ui" {
			c.Next()
			return
		}

		// Check if system is initialized
		hasUser, err := database.HasUser()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check initialization status"})
			c.Abort()
			return
		}

		// If no users exist, allow access for initialization
		if !hasUser {
			c.Next()
			return
		}

		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing authorization header"})
			c.Abort()
			return
		}

		// Check Bearer prefix
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization header format"})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// Validate token
		claims, err := utils.ValidateToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		user, err := database.GetUserByID(claims.UserID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
			c.Abort()
			return
		}
		if strings.ToLower(strings.TrimSpace(user.Status)) != "active" {
			c.JSON(http.StatusForbidden, gin.H{"error": "User is disabled"})
			c.Abort()
			return
		}

		// Set user info in context
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)

		c.Next()
	}
}
