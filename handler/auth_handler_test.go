package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/database"
	"github.com/vibe-coding-labs/claude-code-cli-with-openai-api/utils"
)

func setupAuthTestDB(t *testing.T) func() {
	tdb, err := database.InitTestDB()
	require.NoError(t, err)
	return func() {
		tdb.Close()
	}
}

func TestLoginDisabledUser(t *testing.T) {
	cleanup := setupAuthTestDB(t)
	defer cleanup()

	err := database.CreateUserWithRoleStatus("disabled_user", "password123", "user", "disabled")
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := &Handler{}
	router.POST("/api/auth/login", h.Login)

	payload := map[string]string{"username": "disabled_user", "password": "password123"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusForbidden, resp.Code)
}

func TestAuthMiddlewareRejectsDisabledUser(t *testing.T) {
	cleanup := setupAuthTestDB(t)
	defer cleanup()

	err := database.CreateUserWithRoleStatus("blocked_user", "password123", "user", "disabled")
	require.NoError(t, err)
	user, err := database.GetUser("blocked_user")
	require.NoError(t, err)

	token, err := utils.GenerateToken(user.Username, user.ID, user.Role)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AuthMiddleware())
	router.GET("/api/secure", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/secure", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusForbidden, resp.Code)
}

func TestAuthMiddlewareAllowsActiveUser(t *testing.T) {
	cleanup := setupAuthTestDB(t)
	defer cleanup()

	err := database.CreateUserWithRoleStatus("active_user", "password123", "user", "active")
	require.NoError(t, err)
	user, err := database.GetUser("active_user")
	require.NoError(t, err)

	token, err := utils.GenerateToken(user.Username, user.ID, user.Role)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AuthMiddleware())
	router.GET("/api/secure", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/secure", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
}
